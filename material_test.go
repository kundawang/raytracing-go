package raytracing

import (
	"bufio"
	"bytes"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestTexturedLambertianAttenuation(t *testing.T) {
	// Four texels in longitude, one row: red, green, blue, white.
	img := image.NewRGBA(image.Rect(0, 0, 4, 1))
	img.SetRGBA(0, 0, color.RGBA{R: r8, A: w8})
	img.SetRGBA(1, 0, color.RGBA{G: g8, A: w8})
	img.SetRGBA(2, 0, color.RGBA{B: b8, A: w8})
	img.SetRGBA(3, 0, color.RGBA{R: w8, G: w8, B: w8, A: w8})
	tex := mustImageTexture(t, img, NearestFilter)

	mat, err := NewLambertianWithTexture(tex)
	if err != nil {
		t.Fatal(err)
	}
	s := sphere{center: origin(), radius: 1.0, material: mat}

	// A ray from +x hits the seam point (u=0): nearest sampling picks the
	// first texel, red.
	hr, ok := s.hit(newRay(NewPoint(2, 0, 0), NewVector(-1, 0, 0)), 0.001, math.MaxFloat64)
	if !ok {
		t.Fatal("ray missed the sphere")
	}
	if got := mat.attenuation(hr); !colorApproxEqual(got, NewColor(1, 0, 0)) {
		t.Fatalf("seam attenuation = %v, want red", got)
	}

	// A ray from +z hits u=0.25 -> texel 1, green.
	hr, ok = s.hit(newRay(NewPoint(0, 0, 2), NewVector(0, 0, -1)), 0.001, math.MaxFloat64)
	if !ok {
		t.Fatal("ray missed the sphere")
	}
	if got := mat.attenuation(hr); !colorApproxEqual(got, NewColor(0, float64(g8)/255, 0)) {
		t.Fatalf("u=0.25 attenuation = %v, want green", got)
	}

	// A ray from -z hits u=0.75 -> texel 3, white.
	hr, ok = s.hit(newRay(NewPoint(0, 0, -2), NewVector(0, 0, 1)), 0.001, math.MaxFloat64)
	if !ok {
		t.Fatal("ray missed the sphere")
	}
	if got := mat.attenuation(hr); !colorApproxEqual(got, NewColor(1, 1, 1)) {
		t.Fatalf("u=0.75 attenuation = %v, want white", got)
	}
}

func TestTexturedMetalAttenuation(t *testing.T) {
	tex := NewSolidTexture(NewColor(0.7, 0.6, 0.5))
	mat, err := NewMetalWithTexture(tex, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	hr := hitRecord{u: 0.1, v: 0.2}
	if got := mat.attenuation(hr); !colorApproxEqual(got, NewColor(0.7, 0.6, 0.5)) {
		t.Fatalf("metal attenuation = %v", got)
	}
}

func TestUntexturedMaterialsUnchanged(t *testing.T) {
	// The original constructors must behave exactly as before: constant
	// albedo regardless of UV.
	lamb := NewLambertian(NewColor(0.4, 0.2, 0.1))
	for _, uv := range [][2]float64{{0, 0}, {0.5, 0.5}, {1, 1}} {
		got := lamb.attenuation(hitRecord{u: uv[0], v: uv[1]})
		if !colorApproxEqual(got, NewColor(0.4, 0.2, 0.1)) {
			t.Fatalf("lambertian changed: %v", got)
		}
	}

	metal := NewMetal(NewColor(0.7, 0.6, 0.5), 0)
	if got := metal.attenuation(hitRecord{u: 0.9, v: 0.9}); !colorApproxEqual(got, NewColor(0.7, 0.6, 0.5)) {
		t.Fatalf("metal changed: %v", got)
	}

	dielectric := NewDielectric(1.5)
	if got := dielectric.attenuation(hitRecord{}); !colorApproxEqual(got, NewColor(1, 1, 1)) {
		t.Fatalf("dielectric changed: %v", got)
	}
}

func TestPoleSamplingIsFiniteAndSeamless(t *testing.T) {
	// A high-frequency lat/long checker stresses both the poles and seam.
	tex := NewCheckerTexture(
		NewSolidTexture(NewColor(1, 1, 1)),
		NewSolidTexture(NewColor(0, 0, 0)),
		16, 8,
	)
	mat, err := NewLambertianWithTexture(tex)
	if err != nil {
		t.Fatal(err)
	}
	s := sphere{center: origin(), radius: 1.0, material: mat}

	// Rays aimed slightly off the exact poles must always yield finite
	// colors and never the missing-color fallback.
	for _, dir := range []Vector{
		NewVector(0.001, -1, 0.001),
		NewVector(0.001, 1, -0.001),
		NewVector(-0.001, 0.999, 0),
		NewVector(0, -0.999, 0.001),
	} {
		hr, ok := s.hit(newRay(NewPoint(0, 0, 0), dir), 0.001, math.MaxFloat64)
		if !ok {
			t.Fatalf("ray %v missed the sphere", dir)
		}
		got := mat.attenuation(hr)
		if colorApproxEqual(got, missingColor) {
			t.Fatalf("pole-adjacent hit %v produced missingColor", dir)
		}
		if math.IsNaN(got.x) || math.IsNaN(got.y) || math.IsNaN(got.z) {
			t.Fatalf("pole-adjacent hit %v produced NaN", dir)
		}
	}

	// Exact poles are pinned to u=0.5 and sample without panicking.
	for _, dir := range []Vector{NewVector(0, 1, 0), NewVector(0, -1, 0)} {
		hr, ok := s.hit(newRay(NewPoint(0, 0, 0), dir), 0.001, math.MaxFloat64)
		if !ok {
			t.Fatalf("pole ray %v missed", dir)
		}
		if !approxEqual(hr.u, 0.5) {
			t.Fatalf("pole u = %v, want 0.5", hr.u)
		}
		_ = mat.attenuation(hr)
	}
}

// parsePPM extracts the pixel values (0-255) of an ASCII PPM stream.
func parsePPM(t *testing.T, data []byte) [][3]int {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(bufio.ScanWords)
	var words []string
	for scanner.Scan() {
		words = append(words, scanner.Text())
	}
	if len(words) < 4 || words[0] != "P3" {
		t.Fatalf("not a P3 PPM: %q", words)
	}
	width, _ := strconv.Atoi(words[1])
	height, _ := strconv.Atoi(words[2])
	maxVal, _ := strconv.Atoi(words[3])
	if maxVal != 255 {
		t.Fatalf("unexpected max value %d", maxVal)
	}
	pixels := make([][3]int, width*height)
	for i := range pixels {
		base := 4 + i*3
		r, _ := strconv.Atoi(words[base])
		g, _ := strconv.Atoi(words[base+1])
		b, _ := strconv.Atoi(words[base+2])
		pixels[i] = [3]int{r, g, b}
	}
	return pixels
}

func TestRenderedTexturedSphereIsNotFlatColor(t *testing.T) {
	// End-to-end check: a checker-textured sphere rendered through the real
	// renderer must show both checker colors, proving the map reaches the
	// surface and participates in scattering.
	tex := NewCheckerTexture(
		NewSolidTexture(NewColor(1, 1, 1)),
		NewSolidTexture(NewColor(0.1, 0.1, 0.1)),
		16, 8,
	)
	mat, err := NewLambertianWithTexture(tex)
	if err != nil {
		t.Fatal(err)
	}

	scene := NewScene()
	scene.Add(NewSphere(origin(), 1.0, mat))

	var img bytes.Buffer
	var log bytes.Buffer
	cfg := NewRendererConfig(24, 24,
		WithImageWriter(&img),
		WithLogWriter(&log),
		WithSamplesPerPixel(1),
		WithMaxTraceDepth(2),
	)
	camCfg := NewCameraConfig(1.0, WithPointOfView(NewPoint(0, 0, 3), origin()))
	NewRenderer(cfg).Render(*scene, NewCamera(camCfg))

	pixels := parsePPM(t, img.Bytes())
	bright, dark := 0, 0
	for _, p := range pixels {
		switch {
		case p[0] > 150:
			bright++
		case p[0] < 120:
			dark++
		}
	}
	if bright == 0 || dark == 0 {
		t.Fatalf("sphere rendered as a flat color (bright=%d dark=%d)", bright, dark)
	}

	// Sanity: logging still reaches the configured writer.
	if !strings.Contains(log.String(), "Done.") {
		t.Fatalf("renderer log missing completion marker: %q", log.String())
	}
}
