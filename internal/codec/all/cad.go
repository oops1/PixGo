//go:build !no3d

package all

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/cad"
)

func init() {
	optional = append(optional, func(r *codec.Registry) {
		r.RegisterDecoder(cad.STLDecoder{})
		r.RegisterDecoder(cad.STEPDecoder{})
	})
}
