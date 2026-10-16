package jpegdec

import (
	"testing"
)

func TestScaleOneMatchesStandardLibrary(t *testing.T) {
	files := corpus(t)
	for _, name := range sortedNames(files) {
		data := files[name]
		want, wantErr := referenceNRGBA(t, data)
		for _, workers := range []int{1, 4} {
			d, err := Open(t.Context(), bytesReader(data), int64(len(data)), Options{Workers: workers})
			if err != nil {
				if wantErr == nil {
					t.Errorf("%s: open: %v", name, err)
				}
				continue
			}
			got, err := d.Decode(t.Context(), 0)
			d.Close()
			if wantErr != nil {
				if err == nil {
					t.Errorf("%s: decoded a file the standard library rejects: %v", name, wantErr)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s workers %d: %v", name, workers, err)
				continue
			}
			if !samePixels(got, want) {
				t.Errorf("%s workers %d: %s", name, workers, firstDifference(got, want))
			}
		}
	}
}

func TestSyntheticFilesMatchStandardLibrary(t *testing.T) {
	files := syntheticFiles()
	for _, name := range sortedNames(files) {
		data := files[name]
		want, wantErr := referenceNRGBA(t, data)
		for _, workers := range []int{1, 3} {
			d, err := Open(t.Context(), bytesReader(data), int64(len(data)), Options{Workers: workers})
			if err != nil {
				if wantErr == nil {
					t.Errorf("%s: open: %v", name, err)
				}
				continue
			}
			got, err := d.Decode(t.Context(), 0)
			d.Close()
			if wantErr != nil {
				if err == nil {
					t.Errorf("%s: decoded a file the standard library rejects: %v", name, wantErr)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s workers %d: %v", name, workers, err)
				continue
			}
			if !samePixels(got, want) {
				t.Errorf("%s workers %d: %s", name, workers, firstDifference(got, want))
			}
		}
	}
}
