package rawdevtest

import "math"

var SRGBToXYZ = [9]float64{
	0.4124564, 0.3575761, 0.1804375,
	0.2126729, 0.7151522, 0.0721750,
	0.0193339, 0.1191920, 0.9503041,
}

var D65White = [3]float64{0.95047, 1, 1.08883}

var ColorMatrix = [9]float64{
	0.55, -0.05, 0,
	-0.15, 1.1, 0.05,
	0, -0.124, 0.744,
}

func Apply(m [9]float64, v [3]float64) [3]float64 {
	return [3]float64{
		m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
		m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
		m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
	}
}

func Neutral() [3]float64 {
	n := Apply(ColorMatrix, D65White)
	top := math.Max(n[0], math.Max(n[1], n[2]))
	return [3]float64{n[0] / top, n[1] / top, n[2] / top}
}

func SceneAt(x, y int) [3]float64 {
	fx, fy := float64(x), float64(y)
	return [3]float64{
		0.12 + 0.6*(0.5+0.5*math.Sin(fx/23+fy/41)),
		0.10 + 0.7*(0.5+0.5*math.Sin(fx/37-fy/29+1)),
		0.15 + 0.55*(0.5+0.5*math.Sin(fx/51+fy/19+2)),
	}
}

func Camera(scene [3]float64) [3]float64 {
	cam := Apply(ColorMatrix, Apply(SRGBToXYZ, scene))
	top := math.Max(Apply(ColorMatrix, D65White)[0], math.Max(Apply(ColorMatrix, D65White)[1], Apply(ColorMatrix, D65White)[2]))
	for i := range cam {
		cam[i] = math.Min(math.Max(cam[i]/top, 0), 1)
	}
	return cam
}

type Mosaic struct {
	W, H  int
	CFA   [4]uint8
	Black [4]float64
	White float64
	Pix   []uint16
}

func Bayer(w, h int, cfa [4]uint8, black [4]float64, white float64, scene func(x, y int) [3]float64) Mosaic {
	m := Mosaic{W: w, H: h, CFA: cfa, Black: black, White: white, Pix: make([]uint16, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pos := (y&1)<<1 | x&1
			c := Camera(scene(x, y))[cfa[pos]]
			m.Pix[y*w+x] = uint16(math.Round(black[pos] + c*(white-black[pos])))
		}
	}
	return m
}

func Linear(w, h int, black, white float64, scene func(x, y int) [3]float64) []uint16 {
	pix := make([]uint16, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := Camera(scene(x, y))
			for k := 0; k < 3; k++ {
				pix[(y*w+x)*3+k] = uint16(math.Round(black + c[k]*(white-black)))
			}
		}
	}
	return pix
}
