package infopanel

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/meta"
)

type CardKind uint8

const (
	CardSizes CardKind = iota
	CardFile
	CardCamera
	CardColor
	CardGPS
	CardNote
)

type Row struct {
	Label string
	Value string
	Link  bool
	Copy  string
	URL   string
}

type Card struct {
	Kind  CardKind
	Title string
	Rows  []Row
}

type Model struct {
	Path     string
	Name     string
	Format   string
	FileSize string
	Cards    []Card
}

type Input struct {
	Path       string
	Name       string
	Info       codec.Info
	FileSize   int64
	Modified   time.Time
	Created    time.Time
	HasCreated bool
	Meta       *meta.Metadata
}

func BuildModel(in Input) Model {
	m := Model{Path: in.Path, Name: in.Name, Format: in.Info.FormatName}
	if in.FileSize > 0 {
		m.FileSize = i18n.FormatBytes(in.FileSize)
	}
	m.Cards = append(m.Cards, sizesCard(in), fileCard(in))
	var extra []Card
	if in.Meta != nil {
		if c, ok := cameraCard(in.Meta); ok {
			extra = append(extra, c)
		}
		if c, ok := colorCard(in.Meta); ok {
			extra = append(extra, c)
		}
		if c, ok := gpsCard(in.Meta); ok {
			extra = append(extra, c)
		}
	}
	if len(extra) == 0 && in.Meta != nil {
		extra = append(extra, Card{Kind: CardNote, Rows: []Row{{Label: i18n.T("info.noMetadata")}}})
	}
	m.Cards = append(m.Cards, extra...)
	return m
}

func sizesCard(in Input) Card {
	w, h := in.Info.Width, in.Info.Height
	if in.Info.Orientation.Valid() {
		w, h = in.Info.Orientation.Size(w, h)
	}
	c := Card{Kind: CardSizes, Title: i18n.T("info.sizes")}
	if w > 0 && h > 0 {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.dimensions"), Value: i18n.T("format.dimensions", w, h)})
		if ar := meta.AspectRatio(w, h); ar != "" {
			c.Rows = append(c.Rows, Row{Label: i18n.T("info.aspect"), Value: ar})
		}
	}
	return c
}

func fileCard(in Input) Card {
	c := Card{Kind: CardFile, Title: i18n.T("info.file")}
	c.Rows = append(c.Rows, Row{Label: i18n.T("info.location"), Value: in.Path, Link: true, Copy: in.Path})
	if in.HasCreated && !in.Created.IsZero() {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.created"), Value: i18n.FormatDateTime(in.Created)})
	}
	if !in.Modified.IsZero() {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.modified"), Value: i18n.FormatDateTime(in.Modified)})
	}
	return c
}

func cameraCard(md *meta.Metadata) (Card, bool) {
	e := md.Exif
	if e == nil {
		return Card{}, false
	}
	c := Card{Kind: CardCamera, Title: i18n.T("info.camera")}
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			c.Rows = append(c.Rows, Row{Label: i18n.T(label), Value: value})
		}
	}
	add("info.make", e.Make)
	add("info.model", e.Model)
	add("info.lens", e.Lens)
	if e.Exposure.Valid() {
		add("info.exposure", i18n.T("format.exposure", e.Exposure.String()))
	}
	if e.FNumber.Valid() {
		add("info.aperture", i18n.T("format.aperture", e.FNumber.FNumberString()))
	}
	if e.ISO > 0 {
		add("info.iso", fmt.Sprint(e.ISO))
	}
	if e.FocalLength.Valid() {
		add("info.focalLength", i18n.T("format.focalLength", e.FocalLength.String()))
	}
	if !e.TakenAt.IsZero() {
		add("info.taken", i18n.FormatDateTime(e.TakenAt))
	}
	if e.Flash != nil {
		if e.Flash.Fired {
			add("info.flash", i18n.T("format.flashFired"))
		} else {
			add("info.flash", i18n.T("format.flashNone"))
		}
	}
	add("info.software", e.Software)
	return c, len(c.Rows) > 0
}

func colorCard(md *meta.Metadata) (Card, bool) {
	icc := md.ICC
	if icc == nil {
		return Card{}, false
	}
	c := Card{Kind: CardColor, Title: i18n.T("info.color")}
	if icc.Description != "" {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.profile"), Value: icc.Description})
	}
	if icc.ColorSpace != "" {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.colorSpace"), Value: icc.ColorSpace})
	}
	return c, len(c.Rows) > 0
}

func gpsCard(md *meta.Metadata) (Card, bool) {
	g := md.GPS
	if g == nil {
		return Card{}, false
	}
	c := Card{Kind: CardGPS, Title: i18n.T("info.gps")}
	lat, lon, _ := strings.Cut(g.DMS(), ", ")
	if lat != "" {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.latitude"), Value: lat}, Row{Label: i18n.T("info.longitude"), Value: lon})
	}
	if g.HasAlt {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.altitude"), Value: i18n.T("format.altitude", altitudeText(g.Alt))})
	}
	c.Rows = append(c.Rows, Row{Label: i18n.T("info.openMap"), Value: "OpenStreetMap", Link: true, URL: mapURL(g)})
	return c, true
}

func mapURL(g *meta.GPS) string {
	return fmt.Sprintf("https://www.openstreetmap.org/?mlat=%.6f&mlon=%.6f#map=15/%.6f/%.6f", g.Lat, g.Lon, g.Lat, g.Lon)
}

func altitudeText(alt float64) string {
	r := math.Round(alt*10) / 10
	if r == math.Trunc(r) {
		return i18n.FormatFloat(r, 0)
	}
	return i18n.FormatFloat(r, 1)
}
