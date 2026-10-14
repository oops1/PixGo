//go:build !noraw

package all

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/raw"
)

func init() {
	optional = append(optional, func(r *codec.Registry) {
		r.RegisterDecoder(raw.Decoder{})
	})
}
