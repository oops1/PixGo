package isobmff_test

import (
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func TestOpenReadsPictureTracks(t *testing.T) {
	f := av1Still(64, 48)
	f.Major = "avis"
	f.Tracks = []isotest.Track{
		{Handler: "auxv", Samples: 12, Width: 64, Height: 48},
		{Handler: "pict", Samples: 12, Width: 64, Height: 48},
	}
	file := open(t, f)
	if len(file.Tracks) != 2 || file.Tracks[1].Handler != "pict" || file.Tracks[1].Samples != 12 || file.Tracks[1].Width != 64 || file.Tracks[1].Height != 48 {
		t.Fatalf("%+v", file.Tracks)
	}
	track, ok := file.Sequence()
	if !ok || track.Handler != "pict" || track.Samples != 12 {
		t.Fatalf("%+v %v", track, ok)
	}
}

func TestSequenceIgnoresTracksWithoutPictures(t *testing.T) {
	f := av1Still(8, 8)
	f.Tracks = []isotest.Track{{Handler: "auxv", Samples: 3}, {Handler: "pict", Samples: 0}, {Handler: "soun", Samples: 9}}
	if _, ok := open(t, f).Sequence(); ok {
		t.Fatal("sequence found")
	}
	if _, ok := open(t, av1Still(8, 8)).Sequence(); ok {
		t.Fatal("sequence found in a still")
	}
}

func TestOpenKeepsTheFirstMovieBox(t *testing.T) {
	f := av1Still(8, 8)
	f.Tracks = []isotest.Track{{Handler: "pict", Samples: 5}}
	f.Extra = [][]byte{isotest.Box("moov", isotest.Box("trak"))}
	file := open(t, f)
	if len(file.Tracks) != 1 || file.Tracks[0].Samples != 5 {
		t.Fatalf("%+v", file.Tracks)
	}
}

func TestOpenRejectsMalformedMovies(t *testing.T) {
	base := isotest.File{Major: "avis", NoMeta: true}.Bytes()
	hdlr := isotest.FullBox("hdlr", 0, 0, isotest.U32(0), []byte("pict"), make([]byte, 12), []byte{0})
	wrap := func(boxes ...[]byte) []byte {
		return append(append([]byte{}, base...), isotest.Box("moov", isotest.Box("trak", boxes...))...)
	}
	stsz := func(count uint32) []byte {
		return isotest.Box("mdia", hdlr, isotest.Box("minf", isotest.Box("stbl", isotest.FullBox("stsz", 0, 0, isotest.U32(1), isotest.U32(count)))))
	}
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"truncated tkhd", wrap(isotest.FullBox("tkhd", 0, 0, isotest.U32(1))), codec.ErrCorrupt},
		{"truncated version 1 tkhd", wrap(isotest.FullBox("tkhd", 1, 0, isotest.U32(1))), codec.ErrCorrupt},
		{"truncated hdlr", wrap(isotest.Box("mdia", isotest.FullBox("hdlr", 0, 0, isotest.U32(0)))), codec.ErrCorrupt},
		{"truncated stsz", wrap(isotest.Box("mdia", isotest.Box("minf", isotest.Box("stbl", isotest.FullBox("stsz", 0, 0, isotest.U32(0)))))), codec.ErrCorrupt},
		{"too many samples", wrap(stsz(1<<24 + 1)), codec.ErrTooLarge},
		{"sample sizes missing", wrap(isotest.Box("mdia", hdlr, isotest.Box("minf", isotest.Box("stbl", isotest.FullBox("stsz", 0, 0, isotest.U32(0), isotest.U32(9)))))), codec.ErrCorrupt},
		{"track box overrun", append(append([]byte{}, base...), isotest.Box("moov", append(isotest.U32(999), []byte("trak")...))...), codec.ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := openBytes(tt.data)
			requireKind(t, err, tt.want)
		})
	}
	file, err := openBytes(wrap(stsz(1 << 24)))
	if err != nil || len(file.Tracks) != 1 || file.Tracks[0].Samples != 1<<24 {
		t.Fatalf("largest accepted track: %v", err)
	}
}

func TestOpenCapsTheNumberOfTracks(t *testing.T) {
	var traks []byte
	for i := 0; i < 65; i++ {
		traks = append(traks, isotest.Box("trak")...)
	}
	data := append(isotest.File{Major: "avis", NoMeta: true}.Bytes(), isotest.Box("moov", traks)...)
	_, err := openBytes(data)
	requireKind(t, err, codec.ErrTooLarge)
}

func TestOpenReadsVersionOneTrackHeaders(t *testing.T) {
	base := isotest.File{Major: "avis", NoMeta: true}.Bytes()
	tkhd := isotest.FullBox("tkhd", 1, 0, isotest.U64(0), isotest.U64(0), isotest.U32(1), isotest.U32(0), isotest.U64(100), make([]byte, 52), isotest.U32(320<<16), isotest.U32(200<<16))
	data := append(base, isotest.Box("moov", isotest.Box("trak", tkhd))...)
	file, err := openBytes(data)
	if err != nil || len(file.Tracks) != 1 || file.Tracks[0].Width != 320 || file.Tracks[0].Height != 200 {
		t.Fatalf("%+v %v", file, err)
	}
}
