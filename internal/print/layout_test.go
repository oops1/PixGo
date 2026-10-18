package print

import (
	"errors"
	"image"
	"math"
	"testing"
)

func layout(t *testing.T, o Options, p Picture) Page {
	t.Helper()
	pg, err := o.Layout(p, 300)
	if err != nil {
		t.Fatal(err)
	}
	return pg
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestSheetPixelsMatchTheKnownPaperSizes(t *testing.T) {
	for _, c := range []struct {
		p    Paper
		land bool
		w, h int
	}{
		{A4, false, 2480, 3508},
		{A4, true, 3508, 2480},
		{A5, false, 1748, 2480},
		{A3, false, 3508, 4961},
		{Letter, false, 2550, 3300},
		{Legal, false, 2550, 4200},
		{Photo1, false, 1181, 1772},
		{Photo2, true, 2126, 1535},
	} {
		w, h := SheetPixels(c.p, c.land, 300)
		if w != c.w || h != c.h {
			t.Errorf("%s landscape=%v: %dx%d, want %dx%d", c.p.ID, c.land, w, h, c.w, c.h)
		}
	}
}

func TestAutoOrientationFollowsThePictureAspect(t *testing.T) {
	o := plainOptions()
	for _, c := range []struct {
		w, h int
		land bool
	}{{4000, 3000, true}, {3000, 4000, false}, {1000, 1000, false}} {
		pg := layout(t, o, Picture{W: c.w, H: c.h})
		if pg.Landscape != c.land {
			t.Errorf("%dx%d: landscape=%v, want %v", c.w, c.h, pg.Landscape, c.land)
		}
		if (pg.Sheet.X > pg.Sheet.Y) != c.land {
			t.Errorf("%dx%d: sheet %v does not follow the orientation", c.w, c.h, pg.Sheet)
		}
	}
	o.Orientation = OrientPortrait
	if pg := layout(t, o, Picture{W: 4000, H: 3000}); pg.Landscape || pg.Sheet != image.Pt(2480, 3508) {
		t.Errorf("forced portrait: %+v", pg)
	}
	o.Orientation = OrientLandscape
	if pg := layout(t, o, Picture{W: 3000, H: 4000}); !pg.Landscape || pg.Sheet != image.Pt(3508, 2480) {
		t.Errorf("forced landscape: %+v", pg)
	}
}

func TestFitKeepsTheWholePictureCentredInsideTheMargins(t *testing.T) {
	for _, m := range Margins {
		o := plainOptions()
		o.Margin = m
		pg := layout(t, o, Picture{W: 4000, H: 3000})
		if pg.Crop != image.Rect(0, 0, 4000, 3000) || pg.Cut {
			t.Errorf("%s: fit must not crop, got %v cut=%v", m.ID(), pg.Crop, pg.Cut)
		}
		if !pg.Dest.In(pg.Content) {
			t.Errorf("%s: %v leaves the content area %v", m.ID(), pg.Dest, pg.Content)
		}
		if pg.Dest.Dx() != pg.Content.Dx() && pg.Dest.Dy() != pg.Content.Dy() {
			t.Errorf("%s: fit touches neither axis of %v: %v", m.ID(), pg.Content, pg.Dest)
		}
		if got, want := float64(pg.Dest.Dx())/float64(pg.Dest.Dy()), 4.0/3; !near(got, want, 0.003) {
			t.Errorf("%s: aspect %.4f, want %.4f", m.ID(), got, want)
		}
		left, right := pg.Dest.Min.X-pg.Content.Min.X, pg.Content.Max.X-pg.Dest.Max.X
		top, bottom := pg.Dest.Min.Y-pg.Content.Min.Y, pg.Content.Max.Y-pg.Dest.Max.Y
		if abs(left-right) > 1 || abs(top-bottom) > 1 {
			t.Errorf("%s: not centred: %d/%d %d/%d", m.ID(), left, right, top, bottom)
		}
		mm := MMToPx(m.MM(), 300)
		if pg.Content != image.Rect(mm, mm, 3508-mm, 2480-mm) {
			t.Errorf("%s: content %v", m.ID(), pg.Content)
		}
	}
}

func TestFitEnlargesASmallPictureAndShrinksALargeOne(t *testing.T) {
	small := layout(t, plainOptions(), Picture{W: 200, H: 100})
	if small.Dest.Dx() != 3508 || small.PrintDPI > 20 {
		t.Errorf("small: %v %.1f dpi", small.Dest, small.PrintDPI)
	}
	large := layout(t, plainOptions(), Picture{W: 12000, H: 8000})
	if large.Dest.Dx() != 3508 || !near(large.PrintDPI, 1026, 5) {
		t.Errorf("large: %v %.1f dpi", large.Dest, large.PrintDPI)
	}
}

func TestFillCoversTheContentAndCropsTheOverflow(t *testing.T) {
	o := plainOptions()
	o.Placement = PlaceFill
	o.Margin = MarginNarrow
	pg := layout(t, o, Picture{W: 4000, H: 3000})
	if pg.Dest != pg.Content {
		t.Fatalf("fill must cover the content: %v vs %v", pg.Dest, pg.Content)
	}
	if !pg.Cut || pg.Crop.Dx() != 4000 || pg.Crop.Dy() >= 3000 {
		t.Errorf("crop %v", pg.Crop)
	}
	want := float64(pg.Content.Dx()) / float64(pg.Content.Dy())
	if got := float64(pg.Crop.Dx()) / float64(pg.Crop.Dy()); !near(got, want, 0.002) {
		t.Errorf("crop aspect %.4f, content aspect %.4f", got, want)
	}
	if mid := (pg.Crop.Min.Y + pg.Crop.Max.Y) / 2; abs(mid-1500) > 1 {
		t.Errorf("crop is not centred: %v", pg.Crop)
	}
	portrait := layout(t, o, Picture{W: 3000, H: 4000})
	if portrait.Crop.Dy() != 4000 || portrait.Crop.Dx() >= 3000 {
		t.Errorf("portrait crop %v", portrait.Crop)
	}
}

func TestFillOnAMatchingAspectCropsAlmostNothing(t *testing.T) {
	o := plainOptions()
	o.Placement = PlaceFill
	o.Paper = Photo1.ID
	pg := layout(t, o, Picture{W: 1500, H: 1000})
	if pg.Crop.Dx() < 1498 || pg.Crop.Dy() < 998 {
		t.Errorf("a 3:2 picture on 10x15 lost %v", pg.Crop)
	}
}

func TestActualSizeUsesThePictureDensity(t *testing.T) {
	o := plainOptions()
	o.Placement = PlaceActual
	for _, c := range []struct {
		dpi  float64
		w, h int
	}{
		{300, 1500, 1000},
		{600, 750, 500},
		{150, 3000, 2000},
		{0, 1500, 1000},
	} {
		pg := layout(t, o, Picture{W: 1500, H: 1000, DPI: c.dpi})
		if pg.Dest.Dx() != c.w || pg.Dest.Dy() != c.h {
			t.Errorf("dpi %v: dest %v, want %dx%d", c.dpi, pg.Dest, c.w, c.h)
		}
		if pg.Cut {
			t.Errorf("dpi %v: nothing should be cut", c.dpi)
		}
	}
}

func TestActualSizeBiggerThanThePageIsCutAroundTheCentre(t *testing.T) {
	o := plainOptions()
	o.Placement = PlaceActual
	pg := layout(t, o, Picture{W: 6000, H: 4000, DPI: 300})
	if pg.Dest != pg.Content || !pg.Cut {
		t.Fatalf("dest %v content %v cut %v", pg.Dest, pg.Content, pg.Cut)
	}
	if pg.Crop.Dx() != 3508 || pg.Crop.Dy() != 2480 {
		t.Errorf("crop %v, want the page-sized middle", pg.Crop)
	}
	if pg.Crop.Min.X != (6000-3508)/2 || pg.Crop.Min.Y != (4000-2480)/2 {
		t.Errorf("crop %v is not centred", pg.Crop)
	}
	if !near(pg.PrintDPI, 300, 0.01) {
		t.Errorf("print dpi %v", pg.PrintDPI)
	}
}

func TestCustomSizeIsPhysical(t *testing.T) {
	o := plainOptions()
	o.Placement = PlaceCustom
	o.CustomW, o.CustomH = 100, 75
	pg := layout(t, o, Picture{W: 4000, H: 3000})
	if pg.Dest.Dx() != 1181 || pg.Dest.Dy() != 886 {
		t.Errorf("dest %v", pg.Dest)
	}
	if w, h := pg.DestMM(); !near(w, 100, 0.1) || !near(h, 75, 0.1) {
		t.Errorf("printed size %.2f x %.2f mm", w, h)
	}
	if !near(pg.PrintDPI, 4000/(100/25.4), 1) {
		t.Errorf("print dpi %v", pg.PrintDPI)
	}
	o.CustomW, o.CustomH = 0, 75
	if _, err := o.Layout(Picture{W: 4000, H: 3000}, 300); !errors.Is(err, ErrBadSize) {
		t.Errorf("zero width: %v", err)
	}
	o.CustomW, o.CustomH = 1000, 75
	pg = layout(t, o, Picture{W: 4000, H: 3000})
	if pg.Dest.Dx() != pg.Content.Dx() || !pg.Cut {
		t.Errorf("a 1 m wide print must be clipped to the page: %v cut=%v", pg.Dest, pg.Cut)
	}
}

func TestInchesConvertThroughMillimetres(t *testing.T) {
	if UnitInch.MM() != 25.4 || UnitCM.MM() != 10 {
		t.Fatal("unit sizes")
	}
	if got := ParseUnit("in"); got != UnitInch {
		t.Errorf("parse in: %v", got)
	}
	if got := ParseUnit("furlong"); got != UnitCM {
		t.Errorf("parse junk: %v", got)
	}
}

func TestMarginsLeaveRoomOnEveryPaper(t *testing.T) {
	for _, p := range Papers {
		for _, m := range Margins {
			o := plainOptions()
			o.Paper = p.ID
			o.Margin = m
			for _, land := range []Orientation{OrientPortrait, OrientLandscape} {
				o.Orientation = land
				pg, err := o.Layout(Picture{W: 1000, H: 700}, 300)
				if err != nil {
					t.Errorf("%s/%s/%v: %v", p.ID, m.ID(), land, err)
					continue
				}
				if pg.Content.Empty() || !pg.Dest.In(pg.Content) || !pg.Content.In(image.Rectangle{Max: pg.Sheet}) {
					t.Errorf("%s/%s/%v: %+v", p.ID, m.ID(), land, pg)
				}
			}
		}
	}
}

func TestLayoutRejectsBadInput(t *testing.T) {
	o := plainOptions()
	if _, err := o.Layout(Picture{}, 300); !errors.Is(err, ErrNoPicture) {
		t.Errorf("empty picture: %v", err)
	}
	if _, err := o.Layout(Picture{W: 1, H: 1}, 0); err == nil {
		t.Error("0 dpi accepted")
	}
	if _, err := o.Layout(Picture{W: 1, H: 1}, 5000); err == nil {
		t.Error("5000 dpi accepted")
	}
	o.Paper = A3.ID
	if _, err := o.Layout(Picture{W: 10, H: 10}, 1200); !errors.Is(err, ErrTooLarge) {
		t.Errorf("A3 at 1200 dpi: %v", err)
	}
}

func TestOptionsNormalizeAndParse(t *testing.T) {
	o := Options{Paper: "weird", Orientation: 9, Placement: 9, Margin: 9, Unit: 9, CustomW: math.NaN(), CustomH: -4}.Normalize()
	if o.Paper != "a4" || o.Orientation != OrientAuto || o.Placement != PlaceFit || o.Margin != MarginNone || o.Unit != UnitCM || o.CustomW != 0 || o.CustomH != 0 {
		t.Errorf("%+v", o)
	}
	for _, p := range Papers {
		got, ok := PaperByID(p.ID)
		if !ok || got.ID != p.ID {
			t.Errorf("paper %s", p.ID)
		}
		if p.ShortMM > p.LongMM {
			t.Errorf("%s: sides in the wrong order", p.ID)
		}
	}
	for _, v := range Orientations {
		if ParseOrientation(v.ID()) != v {
			t.Errorf("orientation %s", v.ID())
		}
	}
	for _, v := range Placements {
		if ParsePlacement(v.ID()) != v {
			t.Errorf("placement %s", v.ID())
		}
	}
	for _, v := range Margins {
		if ParseMargin(v.ID()) != v {
			t.Errorf("margin %s", v.ID())
		}
	}
	if ParseOrientation("x") != OrientAuto || ParsePlacement("x") != PlaceFit || ParseMargin("x") != MarginNone {
		t.Error("unknown ids must fall back to the defaults")
	}
}

func TestKeepRatio(t *testing.T) {
	p := Picture{W: 4000, H: 3000}
	if got := KeepRatio(p, 100, true); !near(got, 75, 1e-9) {
		t.Errorf("height from width: %v", got)
	}
	if got := KeepRatio(p, 75, false); !near(got, 100, 1e-9) {
		t.Errorf("width from height: %v", got)
	}
}

func TestFitAndActualSizesInMillimetres(t *testing.T) {
	o := plainOptions()
	w, h := o.FitSizeMM(Picture{W: 4000, H: 3000})
	if !near(w, 280, 0.01) || !near(h, 210, 0.01) {
		t.Errorf("fit size %.2f x %.2f", w, h)
	}
	w, h = o.ActualSizeMM(Picture{W: 3000, H: 1500, DPI: 300})
	if !near(w, 254, 0.01) || !near(h, 127, 0.01) {
		t.Errorf("actual size %.2f x %.2f", w, h)
	}
	w, h = o.ActualSizeMM(Picture{W: 3000, H: 1500})
	if !near(w, 254, 0.01) {
		t.Errorf("unknown density must act as %d dpi: %.2f", UnknownDPI, w)
	}
}

func plainOptions() Options {
	o := DefaultOptions()
	o.Margin = MarginNone
	return o
}

func TestDefaultsAreFitOnA4WithNarrowMargins(t *testing.T) {
	o := DefaultOptions()
	if o.Paper != "a4" || o.Orientation != OrientAuto || o.Placement != PlaceFit || o.Margin != MarginNarrow || o.Unit != UnitCM || !o.Keep {
		t.Errorf("%+v", o)
	}
}

func FuzzLayout(f *testing.F) {
	f.Add(4000, 3000, 300.0, 100, uint8(0), uint8(0), uint8(1), uint8(0), 0.0, 0.0)
	f.Add(1, 1, 0.0, 72, uint8(2), uint8(3), uint8(2), uint8(6), 1.0, 2000.0)
	f.Add(100000, 3, 5.0, 1200, uint8(1), uint8(2), uint8(0), uint8(4), 50.0, 50.0)
	f.Fuzz(func(t *testing.T, w, h int, picDPI float64, dpi int, orient, place, margin, paper uint8, cw, ch float64) {
		o := Options{
			Paper:       Papers[int(paper)%len(Papers)].ID,
			Orientation: Orientation(orient % 3),
			Placement:   Placement(place % 4),
			Margin:      Margin(margin % 3),
			CustomW:     cw,
			CustomH:     ch,
		}
		pg, err := o.Layout(Picture{W: w, H: h, DPI: picDPI}, dpi)
		if err != nil {
			return
		}
		sheet := image.Rectangle{Max: pg.Sheet}
		if pg.Content.Empty() || !pg.Content.In(sheet) || pg.Dest.Empty() || !pg.Dest.In(pg.Content) {
			t.Fatalf("%+v", pg)
		}
		if pg.Crop.Empty() || !pg.Crop.In(image.Rect(0, 0, w, h)) {
			t.Fatalf("crop %v of %dx%d", pg.Crop, w, h)
		}
		if int64(pg.Sheet.X)*int64(pg.Sheet.Y) > MaxSheetPixels {
			t.Fatalf("sheet %v", pg.Sheet)
		}
	})
}
