package all

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/bmp"
	"github.com/oops1/PixGo/internal/codec/gif"
	"github.com/oops1/PixGo/internal/codec/ico"
	"github.com/oops1/PixGo/internal/codec/jpeg"
	"github.com/oops1/PixGo/internal/codec/png"
	"github.com/oops1/PixGo/internal/codec/svg"
	"github.com/oops1/PixGo/internal/codec/tiff"
	"github.com/oops1/PixGo/internal/codec/webp"
)

func Register(r *codec.Registry) {
	r.RegisterDecoder(jpeg.Decoder{})
	r.RegisterDecoder(png.Decoder{})
	r.RegisterDecoder(gif.Decoder{})
	r.RegisterDecoder(bmp.Decoder{})
	r.RegisterDecoder(tiff.Decoder{})
	r.RegisterDecoder(webp.Decoder{})
	r.RegisterDecoder(ico.Decoder{})
	r.RegisterDecoder(ico.CursorDecoder{})
	r.RegisterDecoder(svg.Decoder{})

	r.RegisterEncoder(jpeg.Encoder{})
	r.RegisterEncoder(png.Encoder{})
	r.RegisterEncoder(bmp.Encoder{})
}
