//go:build !nojxl

package all

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/jxl"
)

func init() {
	optional = append(optional, func(r *codec.Registry) {
		r.RegisterDecoder(jxl.Decoder{})
	})
}
