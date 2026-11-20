package raytracing

import (
	"math"
)

type Hitter interface {
	hit(r ray, tMin, tMax float64) (hitRecord, bool)
}

type Scene struct {
	hitters []Hitter
}

func NewScene() *Scene {
	return &Scene{hitters: []Hitter{}}
}

func (s *Scene) Add(h Hitter) *Scene {
	s.hitters = append(s.hitters, h)
	return s
}

func (s Scene) hit(r ray, tMin, tMax float64) (hitRecord, bool) {
	hitAnything := false
	closestT := math.MaxFloat64
	var closest hitRecord
	for _, h := range s.hitters {
		if hr, ok := h.hit(r, tMin, tMax); ok {
			hitAnything = true
			if hr.t < closestT {
				closestT = hr.t
				closest = hr
			}
		}
	}
	return closest, hitAnything
}

type hitRecord struct {
	point    Point
	normal   Vector
	incident ray
	t        float64
	u        float64
	v        float64
	material Material
}

func newHitRecord(p Point, v Vector, r ray, t, u, vv float64, m Material) hitRecord {
	return hitRecord{point: p, normal: v, incident: r, t: t, u: u, v: vv, material: m}
}

// sphereUV maps a point on a sphere to latitude/longitude texture
// coordinates. The mapping is driven solely by the geometric outward normal
// (the direction from the center to the hit point), so it is stable under
// the viewing ray: the same surface point always yields the same UV.
//
//	u = 1 - (phi + pi) / (2 pi),  phi in [-pi, pi]   (longitude, repeats)
//	v = theta / pi,               theta in [0, pi]    (latitude, clamped)
//
// At the poles the longitude is undefined; it is pinned to u = 0.5 so that
// neighboring samples converge instead of producing stripes.
func sphereUV(center Point, p Point, radius float64) (float64, float64) {
	n := center.to(p).div(radius).normalize()

	// Clamp the components against floating-point drift so asin/acos never
	// return NaN for values barely outside [-1, 1].
	clamp := func(x float64) float64 {
		if x < -1 {
			return -1
		}
		if x > 1 {
			return 1
		}
		return x
	}
	nx, ny, nz := clamp(n.x), clamp(n.y), clamp(n.z)

	theta := math.Acos(-ny)
	phi := math.Atan2(nz, -nx)

	u := 1 - (phi+math.Pi)/(2*math.Pi)
	v := theta / math.Pi

	if ny <= -1+1e-12 || ny >= 1-1e-12 {
		u = 0.5
	}
	return u, v
}

type sphere struct {
	center   Point
	radius   float64
	material Material
}

func NewSphere(p Point, r float64, m Material) Hitter {
	return sphere{center: p, radius: r, material: m}
}

func (s sphere) hit(r ray, tMin, tMax float64) (hitRecord, bool) {

	v := s.center.to(r.origin)
	a := r.direction.norm()
	b := 2 * r.direction.dot(v)
	c := v.norm() - s.radius*s.radius

	disc := b*b - 4*a*c
	if disc < 0 {
		return hitRecord{}, false
	}

	tClose := (-b - math.Sqrt(disc)) / (2 * a)
	if tMin < tClose && tClose < tMax {
		p := r.at(tClose)
		n := s.center.to(p).div(s.radius)
		u, v := sphereUV(s.center, p, s.radius)
		return newHitRecord(p, n, r, tClose, u, v, s.material), true
	}

	tFar := (-b + math.Sqrt(disc)) / (2 * a)
	if tMin < tFar && tFar < tMax {
		p := r.at(tFar)
		n := s.center.to(p).div(s.radius)
		u, v := sphereUV(s.center, p, s.radius)
		return newHitRecord(p, n, r, tFar, u, v, s.material), true
	}

	return hitRecord{}, false
}
