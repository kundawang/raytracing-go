package raytracing

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-12
}

func TestSphereUV(t *testing.T) {
	cases := []struct {
		name string
		p    Point
		u    float64
		v    float64
	}{
		// RTIOW standard equirectangular mapping on the unit sphere.
		{"+x is the longitude seam, u=0", NewPoint(1, 0, 0), 0.0, 0.5},
		{"-x is the opposite meridian, u=0.5", NewPoint(-1, 0, 0), 0.5, 0.5},
		{"+z maps to u=0.25", NewPoint(0, 0, 1), 0.25, 0.5},
		{"-z maps to u=0.75", NewPoint(0, 0, -1), 0.75, 0.5},
		{"north pole, v=1 and u pinned", NewPoint(0, 1, 0), 0.5, 1.0},
		{"south pole, v=0 and u pinned", NewPoint(0, -1, 0), 0.5, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, v := sphereUV(origin(), tc.p, 1.0)
			if !approxEqual(u, tc.u) || !approxEqual(v, tc.v) {
				t.Fatalf("sphereUV(%v) = (%v, %v), want (%v, %v)", tc.p, u, v, tc.u, tc.v)
			}
		})
	}
}

func TestSphereUVRange(t *testing.T) {
	// Every direction on the sphere must produce UVs inside [0, 1].
	for i := 0; i <= 200; i++ {
		theta := math.Pi * float64(i) / 200.0
		for j := 0; j <= 400; j++ {
			phi := 2 * math.Pi * float64(j) / 400.0
			p := NewPoint(
				math.Sin(theta)*math.Cos(phi),
				math.Cos(theta),
				math.Sin(theta)*math.Sin(phi),
			)
			u, v := sphereUV(origin(), p, 1.0)
			if math.IsNaN(u) || math.IsNaN(v) || u < 0 || u > 1 || v < 0 || v > 1 {
				t.Fatalf("UV out of range at %v: u=%v v=%v", p, u, v)
			}
		}
	}
}

func TestSphereUVTranslatedAndScaled(t *testing.T) {
	// The mapping depends only on the outward normal, so translation and
	// radius must not change the UV of the equivalent surface direction.
	center := NewPoint(2, 3, 4)
	radius := 2.0
	p := NewPoint(4, 3, 4) // center + radius * (1, 0, 0)
	u, v := sphereUV(center, p, radius)
	if !approxEqual(u, 0.0) || !approxEqual(v, 0.5) {
		t.Fatalf("got (%v, %v), want (0, 0.5)", u, v)
	}
}

func TestSphereUVStableAcrossRays(t *testing.T) {
	// Two different viewing rays that land on the same surface point must
	// produce identical UVs: the mapping is geometric, not view dependent.
	s := sphere{center: origin(), radius: 1.0, material: NewLambertian(NewColor(1, 1, 1))}

	first, ok := s.hit(newRay(NewPoint(3, 0, 0), NewVector(-1, 0, 0)), 0.001, math.MaxFloat64)
	if !ok {
		t.Fatal("first ray missed the sphere")
	}
	second, ok := s.hit(newRay(NewPoint(1, 2, 0), NewVector(0, -1, 0)), 0.001, math.MaxFloat64)
	if !ok {
		t.Fatal("second ray missed the sphere")
	}
	if !approxEqual(first.point.x, second.point.x) || !approxEqual(first.point.y, second.point.y) {
		t.Fatalf("rays hit different points: %v vs %v", first.point, second.point)
	}
	if !approxEqual(first.u, second.u) || !approxEqual(first.v, second.v) {
		t.Fatalf("UV not stable: (%v,%v) vs (%v,%v)", first.u, first.v, second.u, second.v)
	}
}
