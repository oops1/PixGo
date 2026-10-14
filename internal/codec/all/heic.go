//go:build !noheic

package all

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/heic"
)

func init() {
	optional = append(optional, func(r *codec.Registry) {
		r.RegisterDecoder(heic.Decoder{})
	})
}
