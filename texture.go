package raytracing

import (
	"errors"
	"image"
	"image/color"
	_ "image/jpeg" // register the JPEG decoder
	_ "image/png"  // register the PNG decoder
	"io"
	"math"
	"os"
)

// Filter selects how an imageTexture reconstructs a color between texels.
type Filter int

const (
	// NearestFilter picks the texel whose center is closest to the sample point.
	NearestFilter Filter = iota
	// BilinearFilter blends the four texels surrounding the sample point.
	BilinearFilter
)

// MaxTextureDimension is the largest accepted width or height of an image
// texture. Larger images are rejected at construction time rather than
// exhausting memory.
const MaxTextureDimension = 16384

// missingColor is returned for every sampling failure (empty texture,
// non-finite UV, ...). Magenta makes a misconfigured texture conspicuous
// instead of silently blending into the scene.
var missingColor = NewColor(1, 0, 1)

var (
	// ErrBadImage is returned when an image cannot be decoded or has zero size.
	ErrBadImage = errors.New("raytracing: cannot decode texture image or empty image")
	// ErrTextureTooLarge is returned when an image exceeds MaxTextureDimension.
	ErrTextureTooLarge = errors.New("raytracing: texture image is too large")
	// ErrNilTexture is returned when a material is built from a nil texture.
	ErrNilTexture = errors.New("raytracing: material requires a non-nil texture")
)

// Texture is anything that can provide a surface color for a UV coordinate.
type Texture interface {
	// value looks up the color at (u, v). Implementations must never panic;
	// they return missingColor when the sample cannot be taken.
	value(u, v float64) Color
}

// solidTexture returns the same color at every UV. Plain-colored materials
// use it so that textured and untextured surfaces share one code path.
type solidTexture struct {
	c Color
}

// NewSolidTexture wraps a constant color in the Texture interface.
func NewSolidTexture(c Color) Texture {
	return solidTexture{c: c}
}

func (t solidTexture) value(_, _ float64) Color {
	return t.c
}

// checkerTexture is a procedural latitude/longitude checkerboard. It needs
// no image asset, so it is handy for verifying sphere UV mapping in tests
// and example scenes.
type checkerTexture struct {
	even Texture
	odd  Texture
	divU float64
	divV float64
}

// NewCheckerTexture builds a checkerboard alternating between two textures,
// with scale cells along the u and v axes. A non-positive scale is treated
// as one cell per axis.
func NewCheckerTexture(even, odd Texture, scaleU, scaleV int) Texture {
	if scaleU < 1 {
		scaleU = 1
	}
	if scaleV < 1 {
		scaleV = 1
	}
	return checkerTexture{
		even: even,
		odd:  odd,
		divU: float64(scaleU),
		divV: float64(scaleV),
	}
}

func (t checkerTexture) value(u, v float64) Color {
	if math.IsNaN(u) || math.IsInf(u, 0) || math.IsNaN(v) || math.IsInf(v, 0) {
		return missingColor
	}
	// wrapU/clampV keep the index math inside the checker grid.
	u = wrapU(u)
	v = clampV(v)

	iu := int(math.Floor(u * t.divU))
	iv := int(math.Floor(v * t.divV))
	// Guard the exact u == 1 / v == 1 boundary.
	if iu >= int(t.divU) {
		iu = int(t.divU) - 1
	}
	if iv >= int(t.divV) {
		iv = int(t.divV) - 1
	}

	if (iu+iv)%2 == 0 {
		return t.even.value(u, v)
	}
	return t.odd.value(u, v)
}

// imageTexture stores a decoded image in linear float64 texels so that the
// hot sampling path performs no color-model conversion.
//
// Addressing is fixed to repeat horizontally (longitude wraps around the
// sphere, keeping the seam seamless) and clamp vertically (latitude is
// pinned at the poles, so no out-of-range stripes appear there).
type imageTexture struct {
	texels []Color
	width  int
	height int
	filter Filter
}

// NewImageTexture decodes a PNG or JPEG stream into a texture. An undecodable
// stream or an empty image yields ErrBadImage; images wider or taller than
// MaxTextureDimension yield ErrTextureTooLarge.
func NewImageTexture(r io.Reader, filter Filter) (Texture, error) {
	if r == nil {
		return nil, ErrBadImage
	}
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, ErrBadImage
	}
	return NewImageTextureFromImage(img, filter)
}

// NewImageTextureFile opens, decodes and wraps the PNG/JPEG file at path.
func NewImageTextureFile(path string, filter Filter) (Texture, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return NewImageTexture(f, filter)
}

// NewImageTextureFromImage wraps an already decoded image.Image.
//
// Rows are stored flipped vertically: image formats are top-down while the
// sphere v coordinate grows from the south pole (v=0) to the north pole
// (v=1). Flipping at load time makes a standard north-up equirectangular
// map appear upright on the sphere.
//
// Colors are converted through color.NRGBAModel, which yields straight
// (non-premultiplied) components. The alpha channel is intentionally
// ignored when shading: transparent pixels keep their RGB color instead of
// becoming holes in the surface.
func NewImageTextureFromImage(img image.Image, filter Filter) (Texture, error) {
	if img == nil {
		return nil, ErrBadImage
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, ErrBadImage
	}
	if width > MaxTextureDimension || height > MaxTextureDimension {
		return nil, ErrTextureTooLarge
	}

	texels := make([]Color, width*height)
	for y := 0; y < height; y++ {
		// Flip rows so that v=0 addresses the image's bottom row.
		dstRow := height - 1 - y
		for x := 0; x < width; x++ {
			nrgba := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			texels[dstRow*width+x] = NewColor(
				float64(nrgba.R)/255,
				float64(nrgba.G)/255,
				float64(nrgba.B)/255,
			)
		}
	}

	return imageTexture{
		texels: texels,
		width:  width,
		height: height,
		filter: normalizeFilter(filter),
	}, nil
}

func (t imageTexture) value(u, v float64) Color {
	if len(t.texels) == 0 || t.width <= 0 || t.height <= 0 {
		return missingColor
	}
	// A non-finite UV has no meaningful location and must not reach the
	// index arithmetic below.
	if math.IsNaN(u) || math.IsInf(u, 0) || math.IsNaN(v) || math.IsInf(v, 0) {
		return missingColor
	}

	if t.filter == NearestFilter {
		// Texel i spans the interval [i, i+1) in continuous texture space,
		// matching the convention used by the bilinear path.
		x := int(math.Floor(wrapU(u) * float64(t.width)))
		y := int(math.Floor(clampV(v) * float64(t.height)))
		return t.at(x, y)
	}

	// Bilinear interpolation. The fractional coordinates are computed in
	// continuous space so horizontal wrapping never clamps against the seam.
	fx := wrapU(u) * float64(t.width)
	fy := clampV(v) * float64(t.height)

	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)

	c00 := t.at(x0, y0)
	c10 := t.at(x0+1, y0)
	c01 := t.at(x0, y0+1)
	c11 := t.at(x0+1, y0+1)

	return blend(blend(c00, c10, tx), blend(c01, c11, tx), ty)
}

// at fetches a texel with repeat addressing in x and clamp addressing in y.
func (t imageTexture) at(x, y int) Color {
	if len(t.texels) == 0 {
		return missingColor
	}

	// Horizontal repeat. math.Mod keeps the argument cheap even for UVs
	// far outside [0, 1].
	x = x % t.width
	if x < 0 {
		x += t.width
	}

	// Vertical clamp.
	if y < 0 {
		y = 0
	} else if y >= t.height {
		y = t.height - 1
	}

	return t.texels[y*t.width+x]
}

// wrapU maps any finite u onto [0, 1) with period 1 (repeat).
func wrapU(u float64) float64 {
	u = math.Mod(u, 1.0)
	if u < 0 {
		u += 1.0
	}
	return u
}

// clampV pins v to [0, 1] (clamp to edge).
func clampV(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func normalizeFilter(f Filter) Filter {
	if f == NearestFilter {
		return NearestFilter
	}
	return BilinearFilter
}

func blend(a, b Color, t float64) Color {
	return NewColor(
		a.x+(b.x-a.x)*t,
		a.y+(b.y-a.y)*t,
		a.z+(b.z-a.z)*t,
	)
}
