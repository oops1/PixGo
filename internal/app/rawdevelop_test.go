package app

import (
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/settings"
)

type developerStub struct {
	stubFormat
	on *bool
}

func (d developerStub) SetDevelop(on bool) { *d.on = on }
func (d developerStub) Develops() bool     { return *d.on }

func developerRegistry(on *bool) *codec.Registry {
	reg := formatsRegistry()
	stub := stubFor(codec.RAW)
	reg.RegisterDecoder(developerStub{stubFormat: stub, on: on})
	return reg
}

func TestStartingWithDevelopmentOffReachesTheDecoder(t *testing.T) {
	on := true
	formatsApp(t, developerRegistry(&on), func(s *settings.Settings) { s.RawDevelop = false })
	if on {
		t.Fatal("the decoder still develops although the setting is off")
	}
}

func TestStartingWithDevelopmentOnLeavesTheDecoderDeveloping(t *testing.T) {
	on := false
	formatsApp(t, developerRegistry(&on), nil)
	if !on {
		t.Fatal("the decoder does not develop although the setting is on")
	}
}

func TestChangingTheSettingReachesTheDecoderBothWays(t *testing.T) {
	on := true
	a := formatsApp(t, developerRegistry(&on), nil)
	a.Settings.Update(func(s *settings.Settings) { s.RawDevelop = false })
	eventually(t, a, "the decoder stops developing", func() bool { return !on })
	a.Settings.Update(func(s *settings.Settings) { s.RawDevelop = true })
	eventually(t, a, "the decoder develops again", func() bool { return on })
}

func TestDecodersWithoutTheSwitchAreLeftAlone(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.RAW), func(s *settings.Settings) { s.RawDevelop = false })
	if a.applyRawDevelop(true) {
		t.Fatal("a decoder that cannot develop reported a change")
	}
}
