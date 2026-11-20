package raytracing

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strings"
	"testing"
)

const (
	r8 = uint8(255)
	g8 = uint8(200)
	b8 = uint8(100)
	k8 = uint8(0)
	w8 = uint8(255)
)

func colorApproxEqual(a, b Color) bool {
	const eps = 1e-9
	return math.Abs(a.x-b.x) < eps && math.Abs(a.y-b.y) < eps && math.Abs(a.z-b.z) < eps
}

// newTestImage builds a 4x2 RGBA image with known texel colors. Image rows
// are top-down; the texture flips them at load time so that v=0 addresses
// the image's bottom row and v=1 its top row:
//
//	image row 0 (top):    red    green  blue   white
//	image row 1 (bottom): black  red    green  blue
func newTestImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	texels := []color.RGBA{
		{r8, k8, k8, w8}, {k8, g8, k8, w8}, {k8, k8, b8, w8}, {w8, w8, w8, w8},
		{k8, k8, k8, w8}, {r8, k8, k8, w8}, {k8, g8, k8, w8}, {k8, k8, b8, w8},
	}
	for i, c := range texels {
		img.SetRGBA(i%4, i/4, c)
	}
	return img
}

func mustTexture(t *testing.T, tex Texture, err error) Texture {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected texture error: %v", err)
	}
	return tex
}

func mustImageTexture(t *testing.T, img image.Image, filter Filter) Texture {
	t.Helper()
	tex, err := NewImageTextureFromImage(img, filter)
	if err != nil {
		t.Fatalf("unexpected texture error: %v", err)
	}
	return tex
}

func mustDecodeTexture(t *testing.T, r io.Reader, filter Filter) Texture {
	t.Helper()
	tex, err := NewImageTexture(r, filter)
	if err != nil {
		t.Fatalf("unexpected texture decode error: %v", err)
	}
	return tex
}

func TestImageTextureNearest(t *testing.T) {
	tex := mustImageTexture(t, newTestImage(), NearestFilter)

	red := NewColor(1, 0, 0)
	green := NewColor(0, float64(g8)/255, 0)
	blue := NewColor(0, 0, float64(b8)/255)
	white := NewColor(1, 1, 1)
	black := NewColor(0, 0, 0)

	cases := []struct {
		name string
		u, v float64
		want Color
	}{
		// Storage row 0 is the image's bottom row: black, red, green, blue.
		{"bottom row black", 0.125, 0.25, black},
		{"bottom row red", 0.375, 0.25, red},
		{"bottom row green", 0.625, 0.25, green},
		{"bottom row blue", 0.875, 0.25, blue},
		// Storage row 1 is the image's top row: red, green, blue, white.
		{"top row red", 0.125, 0.75, red},
		{"top row green", 0.375, 0.75, green},
		{"top row blue", 0.625, 0.75, blue},
		{"top row white", 0.875, 0.75, white},
		// Horizontal repeat: the seam is continuous.
		{"repeat from the right", 1.125, 0.25, black},
		{"repeat from the left", -0.875, 0.25, black},
		{"far wrapped u", 7.125, 0.25, black},
		// Vertical clamp: outside the poles, the edge row is reused.
		{"clamp below south pole", 0.375, -5, red},
		{"clamp above north pole", 0.625, 5, blue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tex.value(tc.u, tc.v); !colorApproxEqual(got, tc.want) {
				t.Fatalf("value(%v,%v) = %v, want %v", tc.u, tc.v, got, tc.want)
			}
		})
	}
}

func TestImageTextureBilinear(t *testing.T) {
	tex := mustImageTexture(t, newTestImage(), BilinearFilter)

	black := NewColor(0, 0, 0)

	// Integer sample points land exactly on texel corners. v=0 addresses
	// the image's bottom-left texel, which is black.
	if got := tex.value(0, 0); !colorApproxEqual(got, black) {
		t.Fatalf("corner sample = %v, want black", got)
	}

	// Halfway between black (x=0) and red (x=1) on the bottom row.
	halfBlackRed := NewColor(0.5, 0, 0)
	if got := tex.value(0.125, 0); !colorApproxEqual(got, halfBlackRed) {
		t.Fatalf("midpoint sample = %v, want %v", got, halfBlackRed)
	}

	// Halfway between red (x=1) and green (x=2).
	halfRedGreen := NewColor(0.5, float64(g8)/255/2, 0)
	if got := tex.value(0.375, 0); !colorApproxEqual(got, halfRedGreen) {
		t.Fatalf("midpoint sample = %v, want %v", got, halfRedGreen)
	}

	// Seam: x=4 wraps to x=0. On the top row the last texel is white and
	// the first red; the halfway blend must match on both sides of the seam.
	seamBlend := NewColor(1, 0.5, 0.5)
	fromRight := tex.value(0.875, 1.0) // fx = 3.5 inside [0,1]
	fromLeft := tex.value(-0.125, 1.0) // wraps to fx = 3.5
	if !colorApproxEqual(fromRight, seamBlend) || !colorApproxEqual(fromLeft, seamBlend) {
		t.Fatalf("seam not continuous: right=%v left=%v want=%v", fromRight, fromLeft, seamBlend)
	}

	// Vertical clamp at the pole: fy beyond the last row still blends
	// against edge texels only, never indexing outside the image.
	pole := tex.value(0.5, 2.0)
	if math.IsNaN(pole.x) || math.IsNaN(pole.y) || math.IsNaN(pole.z) {
		t.Fatal("clamped pole sample is NaN")
	}
	if got := tex.value(0.5, 2.0); !colorApproxEqual(got, tex.value(0.5, 1.0)) {
		t.Fatalf("clamped v should pin to the edge row: %v vs %v", got, tex.value(0.5, 1.0))
	}
}

func TestImageTextureInvalidUV(t *testing.T) {
	tex := mustImageTexture(t, newTestImage(), BilinearFilter)
	for _, uv := range [][2]float64{
		{math.NaN(), 0.5},
		{0.5, math.NaN()},
		{math.Inf(1), 0.5},
		{math.Inf(-1), 0.5},
		{0.5, math.Inf(1)},
	} {
		if got := tex.value(uv[0], uv[1]); !colorApproxEqual(got, missingColor) {
			t.Fatalf("value(%v,%v) = %v, want missingColor", uv[0], uv[1], got)
		}
	}
}

func TestNewImageTextureBadInput(t *testing.T) {
	if _, err := NewImageTexture(strings.NewReader("not an image"), BilinearFilter); !errors.Is(err, ErrBadImage) {
		t.Fatalf("garbage stream: got %v, want ErrBadImage", err)
	}
	if _, err := NewImageTexture(nil, BilinearFilter); !errors.Is(err, ErrBadImage) {
		t.Fatalf("nil reader: got %v, want ErrBadImage", err)
	}
	if _, err := NewImageTextureFromImage(nil, BilinearFilter); !errors.Is(err, ErrBadImage) {
		t.Fatalf("nil image: got %v, want ErrBadImage", err)
	}
	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if _, err := NewImageTextureFromImage(empty, BilinearFilter); !errors.Is(err, ErrBadImage) {
		t.Fatalf("empty image: got %v, want ErrBadImage", err)
	}
}

func TestNewImageTextureTooLarge(t *testing.T) {
	// A struct with huge bounds exercises the size guard before any large
	// texel allocation happens.
	huge := image.NewRGBA(image.Rect(0, 0, 4, MaxTextureDimension+1))
	if _, err := NewImageTextureFromImage(huge, NearestFilter); !errors.Is(err, ErrTextureTooLarge) {
		t.Fatalf("got %v, want ErrTextureTooLarge", err)
	}
}

func TestImageTextureEmptyValue(t *testing.T) {
	// A zero-value texture must report the fallback color rather than panic.
	var empty imageTexture
	if got := empty.value(0.5, 0.5); !colorApproxEqual(got, missingColor) {
		t.Fatalf("empty texture value = %v, want missingColor", got)
	}
}

func TestImageTextureTransparentPNG(t *testing.T) {
	// Transparent pixels keep their straight RGB: alpha is ignored while
	// shading so a PNG with an alpha channel never opens holes in a sphere.
	// NRGBA stores non-premultiplied components, so the RGB survives A=0.
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: r8, G: g8, B: b8, A: 0})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	tex := mustDecodeTexture(t, &buf, NearestFilter)
	want := NewColor(1, float64(g8)/255, float64(b8)/255)
	if got := tex.value(0.5, 0.5); !colorApproxEqual(got, want) {
		t.Fatalf("transparent pixel = %v, want %v (alpha ignored)", got, want)
	}
}

func TestNewImageTextureJPEG(t *testing.T) {
	// Two large solid halves keep the sample points far from the boundary
	// so JPEG chroma subsampling cannot contaminate them.
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x < 4 {
				img.SetRGBA(x, y, color.RGBA{R: r8, A: w8})
			} else {
				img.SetRGBA(x, y, color.RGBA{B: b8, A: w8})
			}
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	tex := mustDecodeTexture(t, &buf, NearestFilter)
	// JPEG is lossy; require the decoded texels to stay close, not exact.
	red := tex.value(0.25, 0.5)
	if red.x < 0.9 || red.y > 0.1 || red.z > 0.1 {
		t.Fatalf("JPEG red texel decoded as %v", red)
	}
	blue := tex.value(0.75, 0.5)
	if blue.z < 0.35 || blue.x > 0.1 {
		t.Fatalf("JPEG blue texel decoded as %v", blue)
	}
}

func TestNormalizeFilter(t *testing.T) {
	// Unknown filter values fall back to bilinear, the safe smooth default.
	tex := mustImageTexture(t, newTestImage(), Filter(99))
	img := tex.(imageTexture)
	if img.filter != BilinearFilter {
		t.Fatalf("got filter %v, want BilinearFilter", img.filter)
	}
}

func TestSolidTexture(t *testing.T) {
	c := NewColor(0.2, 0.4, 0.6)
	tex := NewSolidTexture(c)
	for _, uv := range [][2]float64{{0, 0}, {0.5, 0.5}, {1, 1}, {-3, 7}} {
		if got := tex.value(uv[0], uv[1]); !colorApproxEqual(got, c) {
			t.Fatalf("solid texture returned %v, want %v", got, c)
		}
	}
}

func TestCheckerTexture(t *testing.T) {
	even := NewSolidTexture(NewColor(1, 1, 1))
	odd := NewSolidTexture(NewColor(0, 0, 0))
	// 4 longitude cells x 2 latitude cells.
	tex := NewCheckerTexture(even, odd, 4, 2)

	white := NewColor(1, 1, 1)
	black := NewColor(0, 0, 0)
	cases := []struct {
		u, v float64
		want Color
	}{
		{0.1, 0.25, white}, // cell (0,0)
		{0.4, 0.25, black}, // cell (1,0)
		{0.1, 0.75, black}, // cell (0,1)
		{0.4, 0.75, white}, // cell (1,1)
		// The last longitude cell (cell 3) is black. Repeat addressing must
		// give the same answer on both sides of the seam.
		{0.99, 0.25, black},
		{-0.01, 0.25, black},
		{1.01, 0.25, white}, // wraps into cell 0, which is white
		// Latitude clamps at the poles instead of indexing out of range.
		{0.1, 2.0, black},
		{0.1, -2.0, white},
	}
	for _, tc := range cases {
		if got := tex.value(tc.u, tc.v); !colorApproxEqual(got, tc.want) {
			t.Fatalf("checker value(%v,%v) = %v, want %v", tc.u, tc.v, got, tc.want)
		}
	}
}

func TestCheckerTextureInvalidUV(t *testing.T) {
	tex := NewCheckerTexture(NewSolidTexture(NewColor(1, 1, 1)), NewSolidTexture(NewColor(0, 0, 0)), 2, 2)
	if got := tex.value(math.NaN(), 0.5); !colorApproxEqual(got, missingColor) {
		t.Fatalf("NaN u = %v, want missingColor", got)
	}
}

func TestTextureRejectsNil(t *testing.T) {
	if _, err := NewLambertianWithTexture(nil); !errors.Is(err, ErrNilTexture) {
		t.Fatalf("lambertian nil texture: got %v, want ErrNilTexture", err)
	}
	if _, err := NewMetalWithTexture(nil, 0); !errors.Is(err, ErrNilTexture) {
		t.Fatalf("metal nil texture: got %v, want ErrNilTexture", err)
	}
}

func TestNewImageTextureFileMissing(t *testing.T) {
	if _, err := NewImageTextureFile("/nonexistent/raytracing-texture.png", NearestFilter); err == nil {
		t.Fatal("missing file unexpectedly succeeded")
	}
}

// Ensure the decoder-registering blank imports are actually exercised via
// the generic image.Decode entry point used by the library.
func TestDecodersRegistered(t *testing.T) {
	var pngBuf, jpegBuf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"png": pngBuf.Bytes(), "jpeg": jpegBuf.Bytes()} {
		if _, err := NewImageTexture(bytes.NewReader(data), NearestFilter); err != nil {
			t.Fatalf("%s decode failed: %v", name, err)
		}
	}
}
