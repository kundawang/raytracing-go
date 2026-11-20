# Ray Tracing in Go

![cover image](images/cover.png)

A Go implementation of the book [Ray Tracing in One Weekend](https://raytracing.github.io/). The repository provides a library to describe and render your own scenes. For more detail, see [`examples/main.go`](examples/main.go).

## Getting Started

```shell
git clone git@github.com:y-taka-23/raytracing-go.git
cd raytracing-go
make examples
./bin/example > example.ppm
open example.ppm
```

## Materials

### Lambertian

|color|result|
|:----:|:----:|
|(0.8, 0.1, 0.1)|![red lambertian sphere](images/lambertian_red.png)|
|(1.0, 1.0, 1.0)|![white lambertian sphere](images/lambertian_white.png)|
|(0.0, 0.0, 0.0)|![black lambertian sphere](images/lambertian_black.png)|

### Metalic

|fuzziness|result|
|:----:|:----:|
|0.0|![metalic sphere of fuzziness 0.0](images/metal_f00.png)|
|0.15|![metalic sphere of fuzziness 0.15](images/metal_f15.png)|
|0.3|![metalic sphere of fuzziness 0.3](images/metal_f30.png)|

### Dielectric

|refractive index|result|
|:----:|:----:|
|1.0|![dielectric sphere of refractive index 1.0](images/dielectric_i100.png)|
|1.5|![dielectric sphere of refractive index 1.5](images/dielectric_i150.png)|
|2.0|![dielectric sphere of refractive index 2.0](images/dielectric_i200.png)|


## Textures

Sphere hits carry stable latitude/longitude UVs, so Lambertian and metal
materials can sample a texture instead of a constant albedo. The texture
color participates in the same scattering as a solid color.

### Image textures

`NewImageTexture` (and the file-based `NewImageTextureFile`) decode PNG or
JPEG data. Two reconstruction filters are available:

```go
tex, err := raytracing.NewImageTextureFile("earth.jpg", raytracing.BilinearFilter)
if err != nil {
    log.Fatal(err)
}
mat, err := raytracing.NewLambertianWithTexture(tex)
```

* `NearestFilter` selects the closest texel; `BilinearFilter` blends the
  four surrounding texels.
* Addressing is **repeat horizontally** (the longitude seam is seamless)
  and **clamp vertically** (the poles pin to the edge row, so no
  out-of-range stripes appear).
* Equirectangular maps are stored north-up: the image's top row maps to the
  north pole and its bottom row to the south pole.
* The alpha channel is ignored — transparent pixels keep their straight RGB
  color instead of opening holes in the surface.
* Undecodable/empty images return `ErrBadImage`, images exceeding
  `MaxTextureDimension` (16384) return `ErrTextureTooLarge`, and a nil
  texture passed to a material returns `ErrNilTexture`. Sampling failures
  (e.g. non-finite UVs) fall back to magenta rather than panicking.

### Procedural checkerboard

`NewCheckerTexture` needs no image asset and is handy for verifying the UV
mapping:

```go
checker := raytracing.NewCheckerTexture(
    raytracing.NewSolidTexture(raytracing.NewColor(0.9, 0.9, 0.9)),
    raytracing.NewSolidTexture(raytracing.NewColor(0.1, 0.1, 0.1)),
    16, 8, // cells along longitude and latitude
)
mat, _ := raytracing.NewLambertianWithTexture(checker)
```

The example scene picks up an optional map through `TEXTURE_PATH`:

```shell
TEXTURE_PATH=earth.jpg ./bin/example > example.ppm
```

## Camera Setting

### Angle of View

|virtical angle (degree)|result|
|:----:|:----:|
|90|![result of the vertical angle in 90 degree](images/fov_90.png)|
|60|![result of the vertical angle in 60 degree](images/fov_60.png)|
|30|![result of the vertical angle in 30 degree](images/fov_30.png)|

### Aperture

|aperture|result|
|:----:|:----:|
|0.0|![result of the aperture 0.0](images/aperture_000.png)|
|0.5|![result of the aperture 0.5](images/aperture_050.png)|
|1.0|![result of the aperture 1.0](images/aperture_100.png)|

### Depth of Field

|focus distance|result|
|:----:|:----:|
|6|![result of the depth of field 6](images/dof_06.png)|
|9|![result of the depth of field 9](images/dof_09.png)|
|12|![result of the depth of field 12](images/dof_12.png)|

## Reference

* Shirley, P. (2016). _Ray Tracing in One Weekend_ ([online edition available](https://raytracing.github.io/))
