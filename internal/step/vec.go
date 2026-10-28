package step

import (
	"fmt"
	"math"

	"github.com/oops1/PixGo/internal/codec"
)

func corrupt(format string, args ...any) error {
	return fmt.Errorf("step: %s: %w", fmt.Sprintf(format, args...), codec.ErrCorrupt)
}

func unsupported(format string, args ...any) error {
	return fmt.Errorf("step: %s: %w", fmt.Sprintf(format, args...), codec.ErrUnsupported)
}

func tooLarge(format string, args ...any) error {
	return fmt.Errorf("step: %s: %w", fmt.Sprintf(format, args...), codec.ErrTooLarge)
}

type vec3 [3]float64

func (a vec3) add(b vec3) vec3 { return vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

func (a vec3) sub(b vec3) vec3 { return vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }

func (a vec3) mul(s float64) vec3 { return vec3{a[0] * s, a[1] * s, a[2] * s} }

func (a vec3) dot(b vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func (a vec3) cross(b vec3) vec3 {
	return vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func (a vec3) norm() float64 { return math.Sqrt(a.dot(a)) }

func (a vec3) unit() (vec3, bool) {
	l := a.norm()
	if !(l > 1e-300) || math.IsInf(l, 0) {
		return vec3{}, false
	}
	return a.mul(1 / l), true
}

func (a vec3) finite() bool {
	for _, x := range a {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

func dist(a, b vec3) float64 { return a.sub(b).norm() }

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

type frame struct {
	o, x, y, z vec3
}

func identityFrame() frame {
	return frame{x: vec3{1, 0, 0}, y: vec3{0, 1, 0}, z: vec3{0, 0, 1}}
}

func (f frame) at(a, b, c float64) vec3 {
	return vec3{
		f.o[0] + a*f.x[0] + b*f.y[0] + c*f.z[0],
		f.o[1] + a*f.x[1] + b*f.y[1] + c*f.z[1],
		f.o[2] + a*f.x[2] + b*f.y[2] + c*f.z[2],
	}
}

func (f frame) local(p vec3) vec3 {
	d := p.sub(f.o)
	return vec3{d.dot(f.x), d.dot(f.y), d.dot(f.z)}
}

func makeFrame(o, z, x vec3) frame {
	zu, ok := z.unit()
	if !ok {
		zu = vec3{0, 0, 1}
	}
	xo := x.sub(zu.mul(x.dot(zu)))
	xu, ok := xo.unit()
	if !ok {
		xu = anyPerp(zu)
	}
	return frame{o: o, x: xu, y: zu.cross(xu), z: zu}
}

func anyPerp(z vec3) vec3 {
	a := vec3{1, 0, 0}
	if math.Abs(z[0]) > 0.8 {
		a = vec3{0, 1, 0}
	}
	p := a.sub(z.mul(a.dot(z)))
	u, ok := p.unit()
	if !ok {
		return vec3{0, 0, 1}
	}
	return u
}

type xform struct {
	m [3][3]float64
	t vec3
}

func identityXform() xform {
	return xform{m: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
}

func frameXform(f frame) xform {
	var x xform
	for r := 0; r < 3; r++ {
		x.m[r][0] = f.x[r]
		x.m[r][1] = f.y[r]
		x.m[r][2] = f.z[r]
	}
	x.t = f.o
	return x
}

func (x xform) apply(p vec3) vec3 {
	return vec3{
		x.m[0][0]*p[0] + x.m[0][1]*p[1] + x.m[0][2]*p[2] + x.t[0],
		x.m[1][0]*p[0] + x.m[1][1]*p[1] + x.m[1][2]*p[2] + x.t[1],
		x.m[2][0]*p[0] + x.m[2][1]*p[1] + x.m[2][2]*p[2] + x.t[2],
	}
}

func (x xform) applyDir(p vec3) vec3 {
	return vec3{
		x.m[0][0]*p[0] + x.m[0][1]*p[1] + x.m[0][2]*p[2],
		x.m[1][0]*p[0] + x.m[1][1]*p[1] + x.m[1][2]*p[2],
		x.m[2][0]*p[0] + x.m[2][1]*p[1] + x.m[2][2]*p[2],
	}
}

func (x xform) mul(y xform) xform {
	var r xform
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r.m[i][j] = x.m[i][0]*y.m[0][j] + x.m[i][1]*y.m[1][j] + x.m[i][2]*y.m[2][j]
		}
	}
	r.t = x.apply(y.t)
	return r
}

func (x xform) det() float64 {
	m := x.m
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) - m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) + m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

func (x xform) inverse() (xform, bool) {
	d := x.det()
	if !(math.Abs(d) > 1e-300) || !finite(d) {
		return identityXform(), false
	}
	m := x.m
	var r xform
	r.m[0][0] = (m[1][1]*m[2][2] - m[1][2]*m[2][1]) / d
	r.m[0][1] = (m[0][2]*m[2][1] - m[0][1]*m[2][2]) / d
	r.m[0][2] = (m[0][1]*m[1][2] - m[0][2]*m[1][1]) / d
	r.m[1][0] = (m[1][2]*m[2][0] - m[1][0]*m[2][2]) / d
	r.m[1][1] = (m[0][0]*m[2][2] - m[0][2]*m[2][0]) / d
	r.m[1][2] = (m[0][2]*m[1][0] - m[0][0]*m[1][2]) / d
	r.m[2][0] = (m[1][0]*m[2][1] - m[1][1]*m[2][0]) / d
	r.m[2][1] = (m[0][1]*m[2][0] - m[0][0]*m[2][1]) / d
	r.m[2][2] = (m[0][0]*m[1][1] - m[0][1]*m[1][0]) / d
	r.t = r.applyDir(x.t).mul(-1)
	return r, true
}

func (x xform) scaled(s float64) xform {
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			x.m[i][j] *= s
		}
	}
	return x
}

func (x xform) isIdentity() bool {
	return x == identityXform()
}

func num(v value) (float64, bool) {
	switch v.kind {
	case vNum:
		return v.num, true
	case vTyped:
		if len(v.list) == 1 {
			return num(v.list[0])
		}
	}
	return 0, false
}

func argNum(a []value, i int) (float64, bool) {
	if i < 0 || i >= len(a) {
		return 0, false
	}
	x, ok := num(a[i])
	if !ok || !finite(x) {
		return 0, false
	}
	return x, true
}

func argInt(a []value, i int) (int, bool) {
	x, ok := argNum(a, i)
	if !ok || x != math.Trunc(x) || math.Abs(x) > 1e9 {
		return 0, false
	}
	return int(x), true
}

func argRef(a []value, i int) (int, bool) {
	if i < 0 || i >= len(a) || a[i].kind != vRef {
		return 0, false
	}
	return a[i].ref, true
}

func argList(a []value, i int) ([]value, bool) {
	if i < 0 || i >= len(a) || a[i].kind != vList {
		return nil, false
	}
	return a[i].list, true
}

func argBool(a []value, i int, def bool) bool {
	if i < 0 || i >= len(a) || a[i].kind != vEnum {
		return def
	}
	switch string(a[i].text) {
	case "T", "t":
		return true
	case "F", "f":
		return false
	}
	return def
}

func argEnum(a []value, i int) string {
	if i < 0 || i >= len(a) || a[i].kind != vEnum {
		return ""
	}
	return string(a[i].text)
}

func numList(l []value) ([]float64, bool) {
	out := make([]float64, len(l))
	for i, v := range l {
		x, ok := num(v)
		if !ok || !finite(x) {
			return nil, false
		}
		out[i] = x
	}
	return out, true
}
