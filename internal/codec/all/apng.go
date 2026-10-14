//go:build !noapng

package all

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/apng"
)

func init() {
	early = append(early, func(r *codec.Registry) {
		r.RegisterDecoder(apng.Decoder{})
	})
}
