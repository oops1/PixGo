package platform

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxDisplayProfileBytes = 16 << 20

type DisplayProfile interface {
	Profile() (icc []byte, name string)
}

func readProfileFile(path string) ([]byte, string) {
	if path == "" {
		return nil, ""
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxDisplayProfileBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxDisplayProfileBytes {
		return nil, ""
	}
	if len(data) < 128 || string(data[36:40]) != "acsp" {
		return nil, ""
	}
	base := filepath.Base(path)
	return data, strings.TrimSuffix(base, filepath.Ext(base))
}
