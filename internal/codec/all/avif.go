//go:build !noavif

package all

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/avif"
)

func init() {
	optional = append(optional, func(r *codec.Registry) {
		r.RegisterDecoder(avif.Decoder{})
	})
}
