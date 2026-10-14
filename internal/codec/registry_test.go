package codec

import (
	"bytes"
	"context"
	"io"
	"testing"
)

type stubDecoder struct {
	id    ID
	exts  []string
	magic string
}

func (s stubDecoder) ID() ID               { return s.id }
func (s stubDecoder) Name() string         { return string(s.id) }
func (s stubDecoder) Extensions() []string { return s.exts }
func (s stubDecoder) Sniff(head []byte) bool {
	return s.magic != "" && bytes.HasPrefix(head, []byte(s.magic))
}
func (s stubDecoder) Probe(context.Context, io.ReaderAt, int64, Limits) (Info, error) {
	return Info{Format: s.id}, nil
}
func (s stubDecoder) Decode(context.Context, io.ReaderAt, int64, Options) (*Image, error) {
	return nil, ErrUnsupported
}

func stubRegistry() *Registry {
	r := NewRegistry()
	r.RegisterDecoder(stubDecoder{id: PNG, exts: []string{".png"}, magic: "PNG"})
	r.RegisterDecoder(stubDecoder{id: AVIF, exts: []string{".avif"}, magic: "AVIF"})
	r.RegisterDecoder(stubDecoder{id: HEIC, exts: []string{".heic", ".HEIF"}, magic: "HEIC"})
	return r
}

func ids(list []Decoder) []ID {
	out := make([]ID, 0, len(list))
	for _, d := range list {
		out = append(out, d.ID())
	}
	return out
}

func sameIDs(a, b []ID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRegisteredListsEveryDecoderWhateverItsSwitch(t *testing.T) {
	tests := []struct {
		name     string
		disabled []ID
		wantAll  []ID
		wantLive []ID
	}{
		{"nothing switched off", nil, []ID{PNG, AVIF, HEIC}, []ID{PNG, AVIF, HEIC}},
		{"one switched off", []ID{AVIF}, []ID{PNG, AVIF, HEIC}, []ID{PNG, HEIC}},
		{"all switched off", []ID{PNG, AVIF, HEIC}, []ID{PNG, AVIF, HEIC}, nil},
		{"unknown id switched off", []ID{JXL}, []ID{PNG, AVIF, HEIC}, []ID{PNG, AVIF, HEIC}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := stubRegistry()
			for _, id := range tc.disabled {
				r.SetEnabled(id, false)
			}

			all, live := ids(r.Registered()), ids(r.Decoders())

			if !sameIDs(all, tc.wantAll) {
				t.Errorf("Registered = %v, want %v", all, tc.wantAll)
			}
			if !sameIDs(live, tc.wantLive) {
				t.Errorf("Decoders = %v, want %v", live, tc.wantLive)
			}
		})
	}
}

func TestRegisteredReturnsACopy(t *testing.T) {
	r := stubRegistry()

	list := r.Registered()
	list[0] = nil

	if r.Registered()[0] == nil {
		t.Fatal("changing the returned slice changed the registry")
	}
}

func TestIsEnabledFollowsSetEnabled(t *testing.T) {
	steps := []struct {
		name string
		do   func(*Registry)
		id   ID
		want bool
	}{
		{"fresh registry", func(*Registry) {}, AVIF, true},
		{"never registered id", func(*Registry) {}, JXL, true},
		{"switched off", func(r *Registry) { r.SetEnabled(AVIF, false) }, AVIF, false},
		{"neighbour untouched", func(r *Registry) { r.SetEnabled(AVIF, false) }, HEIC, true},
		{"switched off and on", func(r *Registry) { r.SetEnabled(AVIF, false); r.SetEnabled(AVIF, true) }, AVIF, true},
		{"switched off twice then on once", func(r *Registry) {
			r.SetEnabled(HEIC, false)
			r.SetEnabled(HEIC, false)
			r.SetEnabled(HEIC, true)
		}, HEIC, true},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			r := stubRegistry()
			tc.do(r)

			if got := r.IsEnabled(tc.id); got != tc.want {
				t.Fatalf("IsEnabled(%s) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestSwitchedOffDecoderStopsClaimingFiles(t *testing.T) {
	tests := []struct {
		name    string
		off     bool
		path    string
		head    string
		support bool
		detects ID
	}{
		{"enabled by extension", false, "a.avif", "", true, ""},
		{"disabled by extension", true, "a.avif", "", false, ""},
		{"enabled by signature", false, "a.bin", "AVIF....", false, AVIF},
		{"disabled by signature", true, "a.bin", "AVIF....", false, ""},
		{"extension case is ignored", false, "A.HEIC", "", true, ""},
		{"second extension of a decoder", false, "a.heif", "", true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := stubRegistry()
			r.SetEnabled(AVIF, !tc.off)

			if got := r.Supports(tc.path); got != tc.support {
				t.Errorf("Supports(%q) = %v, want %v", tc.path, got, tc.support)
			}
			d, ok := r.Detect([]byte(tc.head), tc.path)
			if tc.detects == "" {
				if ok && tc.head != "" {
					t.Errorf("Detect found %s for a switched-off signature", d.ID())
				}
				return
			}
			if !ok || d.ID() != tc.detects {
				t.Errorf("Detect = %v, %v, want %s", d, ok, tc.detects)
			}
		})
	}
}

func TestExtensionsDropOnlyTheSwitchedOffDecoder(t *testing.T) {
	r := stubRegistry()
	r.SetEnabled(HEIC, false)

	got := r.Extensions()

	want := []string{".avif", ".png"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Extensions = %v, want %v", got, want)
	}
	r.SetEnabled(HEIC, true)
	if n := len(r.Extensions()); n != 4 {
		t.Fatalf("after switching on: %d extensions, want 4", n)
	}
}
