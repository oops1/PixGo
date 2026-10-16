package view

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/viewer"
)

func TestEmptyCanvasIsNotAnImage(t *testing.T) {
	r := newRig(t)
	info := r.c.AccessInfo()
	if info.Role != widget.RolePanel || info.Name != "" {
		t.Errorf("empty canvas exposes %+v", info)
	}
}

func TestImageNodeCarriesNameDimensionsAndZoom(t *testing.T) {
	i18n.SetLang("en")
	r := newRig(t)
	r.c.SetImage(source(200, 100), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	info := r.c.AccessInfo()
	if info.Role != widget.RoleImage {
		t.Fatalf("role %s", info.Role)
	}
	if info.Name != "200 × 100" {
		t.Errorf("fallback name %q", info.Name)
	}
	if info.Value != "100%" {
		t.Errorf("zoom value %q", info.Value)
	}
	r.c.SetDescription(func() string { return "beach.jpg — 200 × 100" })
	if got := r.c.AccessInfo().Name; got != "beach.jpg — 200 × 100" {
		t.Errorf("described name %q", got)
	}
	r.c.SetDescription(nil)
	if got := r.c.AccessInfo().Name; got != "200 × 100" {
		t.Errorf("name after clearing %q", got)
	}
}

func TestImageValueFollowsZoomAndLanguage(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(800, 600), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	i18n.SetLang("ru")
	fit := r.c.AccessInfo().Value
	r.c.Actual()
	r.frame()
	if actual := r.c.AccessInfo().Value; actual != "100%" || actual == fit {
		t.Errorf("zoom values: fit %q actual %q", fit, actual)
	}
}

func TestPreviewIsDescribed(t *testing.T) {
	i18n.SetLang("ru")
	r := newRig(t)
	r.c.SetImage(source(200, 100), codec.OrientNormal, viewer.ZoomFit)
	r.c.SetPreview(true)
	if got := r.c.AccessInfo().Description; got != i18n.T("viewer.preview") {
		t.Errorf("description %q", got)
	}
}

func TestPanStepsMoveByATenthOfTheArea(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(2000, 1500), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	r.c.PanSteps(1, 0)
	r.frame()
	before := r.c.View().CenterU
	r.c.PanSteps(1, 0)
	r.frame()
	moved := r.c.View().CenterU - before
	if moved <= 0 {
		t.Fatalf("right step moved the view by %v", moved)
	}
	if want := float64(areaW) / panStepDivisor; moved < want-1 || moved > want+1 {
		t.Errorf("step %v, want about %v", moved, want)
	}
	r.c.PanSteps(-1, 0)
	r.frame()
	if got := r.c.View().CenterU; got < before-1 || got > before+1 {
		t.Errorf("back step ended at %v, started at %v", got, before)
	}
}

func TestPanStepsDoNothingWhenTheImageFits(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(200, 100), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	before := *r.c.View()
	r.c.PanSteps(3, 3)
	if *r.c.View() != before {
		t.Error("view changed although the image fits")
	}
}

func TestReducedPictureIsDescribed(t *testing.T) {
	i18n.SetLang("en")
	r := newRig(t)
	src := source(200, 100)
	src.Shift = 2
	r.c.SetImage(src, codec.OrientNormal, viewer.ZoomFit)
	if got := r.c.AccessInfo().Description; got != "Showing a reduced image" {
		t.Errorf("description %q", got)
	}
	r.c.SetPreview(true)
	if got := r.c.AccessInfo().Description; got != "Preview. Showing a reduced image" {
		t.Errorf("description %q", got)
	}
}

func TestFullPictureHasNoDescription(t *testing.T) {
	i18n.SetLang("en")
	r := newRig(t)
	r.c.SetImage(source(200, 100), codec.OrientNormal, viewer.ZoomFit)
	if got := r.c.AccessInfo().Description; got != "" {
		t.Errorf("description %q", got)
	}
}
