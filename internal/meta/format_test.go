package meta

import (
	"math"
	"testing"
)

func TestRationalBasics(t *testing.T) {
	if (Rational{1, 0}).Valid() || !(Rational{0, 1}).Valid() || !(Rational{-3, 2}).Valid() {
		t.Fatal("Valid")
	}
	if (Rational{1, 0}).Float() != 0 || (Rational{1, 4}).Float() != 0.25 || (Rational{-3, 2}).Float() != -1.5 {
		t.Fatal("Float")
	}
}

func TestRationalString(t *testing.T) {
	cases := []struct {
		in   Rational
		want string
	}{
		{Rational{1, 250}, "1/250"},
		{Rational{1, 3}, "1/3"},
		{Rational{3, 10}, "1/3"},
		{Rational{1, 2}, "1/2"},
		{Rational{5, 10}, "1/2"},
		{Rational{10, 20}, "1/2"},
		{Rational{2, 1}, "2"},
		{Rational{5, 2}, "2.5"},
		{Rational{25, 10}, "2.5"},
		{Rational{30, 1}, "30"},
		{Rational{300, 10}, "30"},
		{Rational{31, 10}, "3.1"},
		{Rational{304, 100}, "3"},
		{Rational{1, 1}, "1"},
		{Rational{13, 10}, "1.3"},
		{Rational{10, 10}, "1"},
		{Rational{0, 5}, "0"},
		{Rational{5, 0}, ""},
		{Rational{0, 0}, ""},
		{Rational{1, 8000}, "1/8000"},
		{Rational{125, 1000000}, "1/8000"},
		{Rational{1, 4000}, "1/4000"},
		{Rational{4, 10}, "0.4"},
		{Rational{6, 10}, "0.6"},
		{Rational{8, 10}, "0.8"},
		{Rational{9, 10}, "0.9"},
		{Rational{2, 3}, "0.7"},
		{Rational{-1, 2}, "-1/2"},
		{Rational{1, -2}, "-1/2"},
		{Rational{-1, -2}, "1/2"},
		{Rational{24, 1}, "24"},
		{Rational{141, 2}, "70.5"},
		{Rational{1, math.MaxInt64}, "1/9223372036854775807"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Fatalf("%+v: %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFNumberString(t *testing.T) {
	cases := []struct {
		in   Rational
		want string
	}{
		{Rational{4, 1}, "4.0"},
		{Rational{40, 10}, "4.0"},
		{Rational{28, 10}, "2.8"},
		{Rational{14, 10}, "1.4"},
		{Rational{283, 100}, "2.8"},
		{Rational{11, 1}, "11.0"},
		{Rational{22, 1}, "22.0"},
		{Rational{5, 0}, ""},
		{Rational{0, 1}, "0.0"},
	}
	for _, c := range cases {
		if got := c.in.FNumberString(); got != c.want {
			t.Fatalf("%+v: %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGPSFormatting(t *testing.T) {
	g := &GPS{Lat: 55 + 45.0/60 + 21.1/3600, Lon: 37 + 37.0/60 + 2.5/3600}
	if got := g.DMS(); got != "55°45′21.1″ N, 37°37′02.5″ E" {
		t.Fatalf("DMS %q", got)
	}
	if got := g.Decimal(); got != "55.755861, 37.617361" {
		t.Fatalf("Decimal %q", got)
	}
	south := &GPS{Lat: -33.8688, Lon: -151.2093}
	if got := south.DMS(); got != "33°52′07.7″ S, 151°12′33.5″ W" {
		t.Fatalf("DMS %q", got)
	}
	if got := south.Decimal(); got != "-33.868800, -151.209300" {
		t.Fatalf("Decimal %q", got)
	}
	zero := &GPS{}
	if got := zero.DMS(); got != "0°00′00.0″ N, 0°00′00.0″ E" {
		t.Fatalf("zero DMS %q", got)
	}
	carry := &GPS{Lat: 10 + 59.0/60 + 59.99/3600, Lon: 179.99999999}
	if got := carry.DMS(); got != "11°00′00.0″ N, 180°00′00.0″ E" {
		t.Fatalf("carry DMS %q", got)
	}
	edge := &GPS{Lat: 89.9999999, Lon: -0.00001}
	if got := edge.DMS(); got != "90°00′00.0″ N, 0°00′00.0″ W" {
		t.Fatalf("edge DMS %q", got)
	}
}

func TestGPSFormattingGuardsInvalidValues(t *testing.T) {
	var nilGPS *GPS
	for _, g := range []*GPS{nilGPS, {Lat: math.NaN()}, {Lon: math.Inf(1)}, {Lat: 91}, {Lon: -181}} {
		if g.DMS() != "" || g.Decimal() != "" {
			t.Fatalf("%+v formatted", g)
		}
	}
}

func TestAspectRatio(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{6000, 4000, "3:2"},
		{1920, 1080, "16:9"},
		{4000, 3000, "4:3"},
		{1000, 1000, "1:1"},
		{6016, 4016, "3:2"},
		{1234, 567, "1234:567"},
		{4000, 6000, "2:3"},
		{3000, 4000, "3:4"},
		{1080, 1920, "9:16"},
		{1280, 800, "16:10"},
		{1024, 1280, "4:5"},
		{3440, 1440, "43:18"},
		{2560, 1080, "64:27"},
		{2520, 1080, "21:9"},
		{3, 2, "3:2"},
		{1, 1, "1:1"},
		{1000, 999, "1:1"},
		{7, 5, "7:5"},
		{1920, 1088, "16:9"},
		{3000, 2000, "3:2"},
		{800, 600, "4:3"},
		{101, 7, "101:7"},
		{2000, 1000, "2:1"},
		{5000, 1000, "5:1"},
		{0, 100, ""},
		{100, 0, ""},
		{-5, 5, ""},
	}
	for _, c := range cases {
		if got := AspectRatio(c.w, c.h); got != c.want {
			t.Fatalf("%dx%d: %q, want %q", c.w, c.h, got, c.want)
		}
	}
}

func TestAspectRatioNeverPanics(t *testing.T) {
	for _, w := range []int{1, 2, 3, 7, 1 << 20, 1<<31 - 1, math.MaxInt64} {
		for _, h := range []int{1, 2, 3, 7, 1 << 20, 1<<31 - 1, math.MaxInt64} {
			if AspectRatio(w, h) == "" {
				t.Fatalf("%dx%d empty", w, h)
			}
		}
	}
}
