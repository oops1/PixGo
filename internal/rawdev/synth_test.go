package rawdev

import (
	"image"
	"math"
)

var srgbToXYZD65 = Matrix{
	0.4124564, 0.3575761, 0.1804375,
	0.2126729, 0.7151522, 0.0721750,
	0.0193339, 0.1191920, 0.9503041,
}

var testCamera = diag([3]float64{0.5, 1, 0.62}).Mul(Matrix{
	1.1, -0.1, 0,
	-0.15, 1.1, 0.05,
	0, -0.2, 1.2,
})

var d65 = [2]float64{0.3127, 0.3290}

func sceneAt(x, y int) [3]float64 {
	fx, fy := float64(x), float64(y)
	return [3]float64{
		0.12 + 0.6*(0.5+0.5*math.Sin(fx/23+fy/41)),
		0.10 + 0.7*(0.5+0.5*math.Sin(fx/37-fy/29+1)),
		0.15 + 0.55*(0.5+0.5*math.Sin(fx/51+fy/19+2)),
	}
}

type synthetic struct {
	mosaic Mosaic
	params Params
	scene  func(x, y int) [3]float64
	camera Matrix
	color  Color
}

type synthOptions struct {
	W, H      int
	CFA       [4]uint8
	Black     [4]float64
	White     float64
	Table     func(v float64) float64
	Inverse   func(v float64) float64
	Pad       int
	Crop      image.Rectangle
	Exposure  float64
	Scene     func(x, y int) [3]float64
	SceneGain float64
}

var (
	rggb = [4]uint8{0, 1, 1, 2}
	bggr = [4]uint8{2, 1, 1, 0}
	grbg = [4]uint8{1, 0, 2, 1}
	gbrg = [4]uint8{1, 2, 0, 1}
)

func cameraNeutral(m Matrix) [3]float64 {
	return m.Apply(xyToXYZ(d65))
}

func cameraColor(m Matrix) Color {
	n := cameraNeutral(m)
	return Color{
		ColorMatrix: [2]Matrix{m},
		HasCM:       [2]bool{true},
		Analog:      [3]float64{1, 1, 1},
		Neutral:     &n,
	}
}

func makeSynthetic(o synthOptions) synthetic {
	if o.Scene == nil {
		o.Scene = sceneAt
	}
	if o.SceneGain == 0 {
		o.SceneGain = 1
	}
	if o.White == 0 {
		o.White = 16383
	}
	if o.Black == [4]float64{} {
		o.Black = [4]float64{512, 512, 512, 512}
	}
	if o.Crop.Empty() {
		o.Crop = image.Rect(0, 0, o.W, o.H)
	}
	col := cameraColor(testCamera)
	r, err := col.Resolve()
	if err != nil {
		panic(err)
	}
	n := cameraNeutral(testCamera)
	top := math.Max(n[0], math.Max(n[1], n[2]))
	stride := o.W + 2*o.Pad
	rows := o.H + 2*o.Pad
	pix := make([]uint16, stride*rows)
	for i := range pix {
		pix[i] = uint16(7 * i)
	}
	for y := 0; y < o.H; y++ {
		for x := 0; x < o.W; x++ {
			s := o.Scene(x, y)
			for i := range s {
				s[i] *= o.SceneGain
			}
			xyz := srgbToXYZD65.Apply(s)
			cam := testCamera.Apply(xyz)
			pos := (y&1)<<1 | x&1
			c := cam[o.CFA[pos]] / top
			c = math.Min(math.Max(c, 0), 1)
			black := o.Black[pos]
			v := black + c*(o.White-black)
			if o.Inverse != nil {
				v = o.Inverse(v)
			}
			pix[(o.Pad+y)*stride+o.Pad+x] = uint16(math.Round(v))
		}
	}
	p := Params{CFA: o.CFA, Black: o.Black, Crop: o.Crop, Render: r, Exposure: o.Exposure}
	for i := range p.White {
		p.White[i] = o.White
	}
	if o.Table != nil {
		p.Table = make([]uint16, 4096)
		for i := range p.Table {
			p.Table[i] = uint16(math.Round(o.Table(float64(i))))
		}
	}
	return synthetic{
		mosaic: Mosaic{Pix: pix, Stride: stride, Samples: 1, X: o.Pad, Y: o.Pad, W: o.W, H: o.H},
		params: p,
		scene:  o.Scene,
		camera: testCamera,
		color:  col,
	}
}

func expected8(s [3]float64, gain float64) [3]float64 {
	var out [3]float64
	for i := range s {
		out[i] = 255 * ToneCurve(s[i]*gain, 0)
	}
	return out
}
