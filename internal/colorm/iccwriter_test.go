package colorm

import (
	"encoding/binary"
	"math"
	"unicode/utf16"
)

type testTag struct {
	sig  string
	data []byte
}

func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func u16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
func f16(v float64) []byte {
	return binary.BigEndian.AppendUint32(nil, uint32(int32(math.Round(v*65536))))
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func buildICC(major byte, class, space, pcs string, tags []testTag) []byte {
	head := make([]byte, 128)
	copy(head[4:], "test")
	head[8] = major
	head[9] = 0x20
	copy(head[12:], class)
	copy(head[16:], space)
	copy(head[20:], pcs)
	copy(head[36:], "acsp")
	copy(head[40:], "MSFT")
	copy(head[68:], f16(0.9642))
	copy(head[72:], f16(1))
	copy(head[76:], f16(0.8249))
	table := u32(uint32(len(tags)))
	off := 128 + 4 + 12*len(tags)
	var body []byte
	for _, t := range tags {
		data := pad4(append([]byte(nil), t.data...))
		table = append(table, cat([]byte(t.sig), u32(uint32(off+len(body))), u32(uint32(len(t.data))))...)
		body = append(body, data...)
	}
	out := cat(head, table, body)
	binary.BigEndian.PutUint32(out[0:], uint32(len(out)))
	return out
}

func descV2(s string) []byte {
	return cat([]byte("desc"), u32(0), u32(uint32(len(s)+1)), []byte(s), []byte{0}, make([]byte, 78))
}

func descV4(s string) []byte {
	u := utf16.Encode([]rune(s))
	var enc []byte
	for _, c := range u {
		enc = append(enc, u16(c)...)
	}
	return cat([]byte("mluc"), u32(0), u32(1), u32(12), []byte("enUS"), u32(uint32(len(enc))), u32(28), enc)
}

func xyzTag(v Vec3) []byte {
	return cat([]byte("XYZ "), u32(0), f16(v[0]), f16(v[1]), f16(v[2]))
}

func curvGamma(g float64) []byte {
	return cat([]byte("curv"), u32(0), u32(1), u16(uint16(math.Round(g*256))))
}

func curvTable(t []float64) []byte {
	b := cat([]byte("curv"), u32(0), u32(uint32(len(t))))
	for _, v := range t {
		b = append(b, u16(uint16(math.Round(clamp01(v)*65535)))...)
	}
	return b
}

func paraTag(kind uint16, p ...float64) []byte {
	b := cat([]byte("para"), u32(0), u16(kind), u16(0))
	for _, v := range p {
		b = append(b, f16(v)...)
	}
	return b
}

func srgbPara() []byte {
	return paraTag(3, 2.4, 1/1.055, 0.055/1.055, 1/12.92, 0.04045)
}

func srgbTable(n int) []byte {
	t := make([]float64, n)
	for i := range t {
		t[i] = srgbDecode(float64(i) / float64(n-1))
	}
	return curvTable(t)
}

func chadTag(m Mat3) []byte {
	b := cat([]byte("sf32"), u32(0))
	for _, v := range m {
		b = append(b, f16(v)...)
	}
	return b
}

type colorants struct {
	r, g, b Vec3
}

var (
	srgbD50  = colorants{Vec3{0.4360747, 0.2225045, 0.0139322}, Vec3{0.3850649, 0.7168786, 0.0971045}, Vec3{0.1430804, 0.0606169, 0.7141733}}
	p3D50    = colorants{Vec3{0.5151, 0.2412, -0.0011}, Vec3{0.2919, 0.6922, 0.0419}, Vec3{0.1571, 0.0666, 0.7841}}
	adobeD50 = colorants{Vec3{0.6097559, 0.3111242, 0.0194811}, Vec3{0.2052401, 0.6256560, 0.0608902}, Vec3{0.1492240, 0.0632197, 0.7448387}}
)

func (c colorants) matrix() Mat3 {
	return Mat3{c.r[0], c.g[0], c.b[0], c.r[1], c.g[1], c.b[1], c.r[2], c.g[2], c.b[2]}
}

func matrixProfile(major byte, desc string, c colorants, trc []byte) []byte {
	d := descV2(desc)
	if major >= 4 {
		d = descV4(desc)
	}
	return buildICC(major, "mntr", "RGB ", "XYZ ", []testTag{
		{"desc", d},
		{"wtpt", xyzTag(d50)},
		{"rXYZ", xyzTag(c.r)}, {"gXYZ", xyzTag(c.g)}, {"bXYZ", xyzTag(c.b)},
		{"rTRC", trc}, {"gTRC", trc}, {"bTRC", trc},
	})
}

func srgbV2() []byte { return matrixProfile(2, "sRGB IEC61966-2.1", srgbD50, srgbTable(1024)) }
func srgbV4() []byte { return matrixProfile(4, "sRGB v4", srgbD50, srgbPara()) }
func displayP3() []byte {
	return matrixProfile(4, "Display P3", p3D50, srgbPara())
}
func adobeRGB() []byte {
	return matrixProfile(2, "Adobe RGB (1998)", adobeD50, curvGamma(2.19921875))
}

func grayProfile(g float64) []byte {
	return buildICC(2, "mntr", "GRAY", "XYZ ", []testTag{
		{"desc", descV2("Gray Gamma 2.2")},
		{"wtpt", xyzTag(d50)},
		{"kTRC", curvGamma(g)},
	})
}

func xyzEnc(v float64) uint16 { return uint16(math.Round(clamp01(v/xyzEncodedMax) * 65535)) }

func mabHeader(in, out int, offB, offMat, offM, offClut, offA int) []byte {
	return cat([]byte("mAB "), u32(0), []byte{byte(in), byte(out), 0, 0}, u32(uint32(offB)), u32(uint32(offMat)), u32(uint32(offM)), u32(uint32(offClut)), u32(uint32(offA)))
}

func mabMatrixA2B(c colorants, trc []byte) []byte {
	m := c.matrix()
	curves := cat(trc, trc, trc)
	curves = pad4(curves)
	var mat []byte
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			mat = append(mat, f16(m[i*3+j]/xyzEncodedMax)...)
		}
	}
	mat = append(mat, f16(0)...)
	mat = append(mat, f16(0)...)
	mat = append(mat, f16(0)...)
	ident := pad4(cat(curvTableIdentity(), curvTableIdentity(), curvTableIdentity()))
	offB := 32
	offMat := offB + len(ident)
	offM := offMat + len(mat)
	return cat(mabHeader(3, 3, offB, offMat, offM, 0, 0), ident, mat, curves)
}

func curvTableIdentity() []byte { return cat([]byte("curv"), u32(0), u32(0)) }

func mabMatrixB2A(c colorants, encode []byte) []byte {
	inv, _ := c.matrix().Inverse()
	var mat []byte
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			mat = append(mat, f16(inv[i*3+j]*xyzEncodedMax)...)
		}
	}
	for i := 0; i < 3; i++ {
		mat = append(mat, f16(0)...)
	}
	ident := pad4(cat(curvTableIdentity(), curvTableIdentity(), curvTableIdentity()))
	curves := pad4(cat(encode, encode, encode))
	offB := 32
	offMat := offB + len(ident)
	offM := offMat + len(mat)
	h := mabHeader(3, 3, offB, offMat, offM, 0, 0)
	copy(h, "mBA ")
	return cat(h, ident, mat, curves)
}

func srgbEncodeTable(n int) []byte {
	t := make([]float64, n)
	for i := range t {
		t[i] = srgbEncode(float64(i) / float64(n-1))
	}
	return curvTable(t)
}

func mft2Header(in, out, grid int, inN, outN int) []byte {
	b := cat([]byte("mft2"), u32(0), []byte{byte(in), byte(out), byte(grid), 0})
	for _, v := range [...]float64{1, 0, 0, 0, 1, 0, 0, 0, 1} {
		b = append(b, f16(v)...)
	}
	return cat(b, u16(uint16(inN)), u16(uint16(outN)))
}

func mft2XYZMatrix(c colorants, trc func(float64) float64) []byte {
	m := c.matrix()
	const inN = 1024
	b := mft2Header(3, 3, 2, inN, 2)
	for ch := 0; ch < 3; ch++ {
		for i := 0; i < inN; i++ {
			b = append(b, u16(uint16(math.Round(trc(float64(i)/(inN-1))*65535)))...)
		}
	}
	for r := 0; r < 2; r++ {
		for g := 0; g < 2; g++ {
			for bl := 0; bl < 2; bl++ {
				x := m.Apply(Vec3{float64(r), float64(g), float64(bl)})
				for k := 0; k < 3; k++ {
					b = append(b, u16(xyzEnc(x[k]))...)
				}
			}
		}
	}
	for ch := 0; ch < 3; ch++ {
		b = append(b, u16(0)...)
		b = append(b, u16(65535)...)
	}
	return b
}

func mft1XYZMatrix(c colorants, trc func(float64) float64) []byte {
	m := c.matrix()
	b := cat([]byte("mft1"), u32(0), []byte{3, 3, 2, 0})
	for _, v := range [...]float64{1, 0, 0, 0, 1, 0, 0, 0, 1} {
		b = append(b, f16(v)...)
	}
	for ch := 0; ch < 3; ch++ {
		for i := 0; i < 256; i++ {
			b = append(b, byte(math.Round(trc(float64(i)/255)*255)))
		}
	}
	for r := 0; r < 2; r++ {
		for g := 0; g < 2; g++ {
			for bl := 0; bl < 2; bl++ {
				x := m.Apply(Vec3{float64(r), float64(g), float64(bl)})
				for k := 0; k < 3; k++ {
					b = append(b, byte(math.Round(clamp01(x[k]/xyzEncodedMax)*255)))
				}
			}
		}
	}
	for ch := 0; ch < 3; ch++ {
		for i := 0; i < 256; i++ {
			b = append(b, byte(i))
		}
	}
	return b
}

func labLegacyEnc(x Vec3) [3]uint16 {
	l, a, b := xyzToLab(x)
	return [3]uint16{
		uint16(math.Round(math.Min(math.Max(l/100*65280, 0), 65535))),
		uint16(math.Round(math.Min(math.Max((a+128)*256, 0), 65535))),
		uint16(math.Round(math.Min(math.Max((b+128)*256, 0), 65535))),
	}
}

func cmykModel(c, m, y, k float64) Vec3 {
	ink := func(v, cross float64) float64 {
		return 0.012 + 0.98*math.Pow((1-v)*(1-k), 2.2)*(1-cross)
	}
	return srgbD50.matrix().Apply(Vec3{ink(c, 0.25*m), ink(m, 0.2*c), ink(y, 0.3*m)})
}

func cmykProfile(grid int, a2bTag, b2aTag bool) []byte {
	b := mft2Header(4, 3, grid, 2, 2)
	for ch := 0; ch < 4; ch++ {
		b = append(b, u16(0)...)
		b = append(b, u16(65535)...)
	}
	for ci := 0; ci < grid; ci++ {
		for mi := 0; mi < grid; mi++ {
			for yi := 0; yi < grid; yi++ {
				for ki := 0; ki < grid; ki++ {
					f := float64(grid - 1)
					x := cmykModel(float64(ci)/f, float64(mi)/f, float64(yi)/f, float64(ki)/f)
					for _, v := range labLegacyEnc(x) {
						b = append(b, u16(v)...)
					}
				}
			}
		}
	}
	for ch := 0; ch < 3; ch++ {
		b = append(b, u16(0)...)
		b = append(b, u16(65535)...)
	}
	return buildICC(2, "prtr", "CMYK", "Lab ", []testTag{
		{"desc", descV2("Test CMYK")},
		{"wtpt", xyzTag(d50)},
		{"A2B0", b},
		{"A2B1", b},
	})
}

func perceptualPair() []byte {
	rel := mabMatrixA2B(srgbD50, srgbTable(1024))
	darker := mabMatrixA2B(colorants{
		Vec3{srgbD50.r[0] * 0.5, srgbD50.r[1] * 0.5, srgbD50.r[2] * 0.5},
		Vec3{srgbD50.g[0] * 0.5, srgbD50.g[1] * 0.5, srgbD50.g[2] * 0.5},
		Vec3{srgbD50.b[0] * 0.5, srgbD50.b[1] * 0.5, srgbD50.b[2] * 0.5},
	}, srgbTable(1024))
	return buildICC(4, "mntr", "RGB ", "XYZ ", []testTag{
		{"desc", descV4("Pair")},
		{"wtpt", xyzTag(d50)},
		{"A2B0", darker},
		{"A2B1", rel},
	})
}

func lutDisplay() []byte {
	return buildICC(4, "mntr", "RGB ", "XYZ ", []testTag{
		{"desc", descV4("LUT display")},
		{"wtpt", xyzTag(d50)},
		{"A2B1", mabMatrixA2B(srgbD50, srgbTable(1024))},
		{"B2A1", mabMatrixB2A(srgbD50, srgbEncodeTable(4096))},
	})
}

func labByte(x Vec3) [3]byte {
	l, a, b := xyzToLab(x)
	cl := func(v float64) byte { return byte(math.Round(math.Min(math.Max(v, 0), 255))) }
	return [3]byte{cl(l / 100 * 255), cl(a + 128), cl(b + 128)}
}

func mft1Lab(grid int, model func(r, g, b float64) Vec3) []byte {
	b := cat([]byte("mft1"), u32(0), []byte{3, 3, byte(grid), 0})
	for _, v := range [...]float64{1, 0, 0, 0, 1, 0, 0, 0, 1} {
		b = append(b, f16(v)...)
	}
	ident := func() {
		for ch := 0; ch < 3; ch++ {
			for i := 0; i < 256; i++ {
				b = append(b, byte(i))
			}
		}
	}
	ident()
	f := float64(grid - 1)
	for r := 0; r < grid; r++ {
		for g := 0; g < grid; g++ {
			for bl := 0; bl < grid; bl++ {
				lab := labByte(model(float64(r)/f, float64(g)/f, float64(bl)/f))
				b = append(b, lab[:]...)
			}
		}
	}
	ident()
	return b
}

func mft2Lab(grid int, model func(r, g, b float64) Vec3) []byte {
	b := mft2Header(3, 3, grid, 2, 2)
	for ch := 0; ch < 3; ch++ {
		b = append(b, u16(0)...)
		b = append(b, u16(65535)...)
	}
	f := float64(grid - 1)
	for r := 0; r < grid; r++ {
		for g := 0; g < grid; g++ {
			for bl := 0; bl < grid; bl++ {
				for _, v := range labLegacyEnc(model(float64(r)/f, float64(g)/f, float64(bl)/f)) {
					b = append(b, u16(v)...)
				}
			}
		}
	}
	for ch := 0; ch < 3; ch++ {
		b = append(b, u16(0)...)
		b = append(b, u16(65535)...)
	}
	return b
}

func srgbModel(r, g, b float64) Vec3 {
	return srgbD50.matrix().Apply(Vec3{srgbDecode(r), srgbDecode(g), srgbDecode(b)})
}

func labProfile(tag []byte) []byte {
	return buildICC(2, "mntr", "RGB ", "Lab ", []testTag{{"desc", descV2("Lab LUT")}, {"wtpt", xyzTag(d50)}, {"A2B0", tag}})
}
