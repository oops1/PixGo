package resize

import (
	"image"
	"testing"
)

var lim = Limits{MaxSide: 20000, MaxPixels: 100_000_000}

func TestInitialValues(t *testing.T) {
	m := NewModel(image.Pt(4000, 3000), image.Pt(2000, 1500), lim)
	if m.WidthText() != "2000" || m.HeightText() != "1500" || !m.Keep() || m.Unit() != UnitPixels {
		t.Fatalf("got %s %s", m.WidthText(), m.HeightText())
	}
	if size, st := m.Result(); st != StatusOK || size != image.Pt(2000, 1500) {
		t.Fatalf("result %v %v", size, st)
	}
	m = NewModel(image.Pt(4000, 3000), image.Point{}, lim)
	if m.WidthText() != "4000" || m.HeightText() != "3000" {
		t.Fatalf("empty initial must use the original: %s %s", m.WidthText(), m.HeightText())
	}
}

func TestKeepProportionsFollowsTheOtherSide(t *testing.T) {
	m := NewModel(image.Pt(4000, 3000), image.Pt(4000, 3000), lim)
	m.SetWidth("1000")
	if m.HeightText() != "750" {
		t.Fatalf("height %s", m.HeightText())
	}
	m.SetHeight("300")
	if m.WidthText() != "400" {
		t.Fatalf("width %s", m.WidthText())
	}
	m.SetWidth("1")
	if m.HeightText() != "1" {
		t.Fatalf("height must not drop below 1: %s", m.HeightText())
	}
	m.SetKeep(false)
	m.SetWidth("1234")
	if m.HeightText() != "1" {
		t.Fatalf("height changed without the lock: %s", m.HeightText())
	}
	m.SetKeep(true)
	if m.HeightText() != "926" {
		t.Fatalf("turning the lock on must re-derive the height: %s", m.HeightText())
	}
}

func TestEmptyAndGarbageInput(t *testing.T) {
	m := NewModel(image.Pt(100, 50), image.Pt(100, 50), lim)
	m.SetWidth("")
	if _, st := m.Result(); st != StatusEmpty || m.HeightText() != "50" {
		t.Fatalf("status %v height %s", st, m.HeightText())
	}
	m.SetWidth("12ab3x")
	if m.WidthText() != "123" {
		t.Fatalf("got %q", m.WidthText())
	}
	m.SetWidth("0")
	if _, st := m.Result(); st != StatusEmpty {
		t.Fatalf("zero is not a size: %v", st)
	}
	m.SetWidth("99999999999")
	if len(m.WidthText()) > maxDigits {
		t.Fatalf("too long: %q", m.WidthText())
	}
}

func TestLimits(t *testing.T) {
	small := Limits{MaxSide: 5000, MaxPixels: 4_000_000}
	m := NewModel(image.Pt(1000, 1000), image.Pt(1000, 1000), small)
	m.SetWidth("2000")
	if size, st := m.Result(); st != StatusOK || size != image.Pt(2000, 2000) {
		t.Fatalf("%v %v", size, st)
	}
	m.SetWidth("2001")
	if _, st := m.Result(); st != StatusTooBig {
		t.Fatalf("4 MP + must be refused, got %v", st)
	}
	m.SetWidth("6000")
	if _, st := m.Result(); st != StatusTooBig {
		t.Fatalf("the side limit: %v", st)
	}
	m.SetKeep(false)
	m.SetWidth("5000")
	m.SetHeight("800")
	if _, st := m.Result(); st != StatusOK {
		t.Fatalf("5000x800 is 4 MP: %v", st)
	}
	m.SetHeight("801")
	if _, st := m.Result(); st != StatusTooBig {
		t.Fatalf("5000x801: %v", st)
	}
}

func TestPercentUnit(t *testing.T) {
	m := NewModel(image.Pt(4000, 3000), image.Pt(2000, 1500), lim)
	m.SetUnit(UnitPercent)
	if m.WidthText() != "50" || m.HeightText() != "50" {
		t.Fatalf("got %s %s", m.WidthText(), m.HeightText())
	}
	if size, st := m.Result(); st != StatusOK || size != image.Pt(2000, 1500) {
		t.Fatalf("%v %v", size, st)
	}
	m.SetWidth("25")
	if m.HeightText() != "25" {
		t.Fatalf("keep in percent: %s", m.HeightText())
	}
	if size, _ := m.Result(); size != image.Pt(1000, 750) {
		t.Fatalf("%v", size)
	}
	m.SetKeep(false)
	m.SetHeight("10")
	if size, _ := m.Result(); size != image.Pt(1000, 300) {
		t.Fatalf("%v", size)
	}
	m.SetUnit(UnitPixels)
	if m.WidthText() != "1000" || m.HeightText() != "300" {
		t.Fatalf("back to pixels: %s %s", m.WidthText(), m.HeightText())
	}
}

func TestPercentOverTheLimitIsRefused(t *testing.T) {
	m := NewModel(image.Pt(4000, 3000), image.Pt(4000, 3000), lim)
	m.SetUnit(UnitPercent)
	m.SetWidth("500")
	if _, st := m.Result(); st != StatusTooBig {
		t.Fatalf("500%% of 12 MP is 300 MP: %v", st)
	}
}

func TestTinyPercentStaysAtOnePixel(t *testing.T) {
	m := NewModel(image.Pt(10, 10), image.Pt(10, 10), lim)
	m.SetUnit(UnitPercent)
	m.SetWidth("1")
	if size, st := m.Result(); st != StatusOK || size != image.Pt(1, 1) {
		t.Fatalf("%v %v", size, st)
	}
}
