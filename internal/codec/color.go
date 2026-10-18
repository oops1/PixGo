package codec

import (
	"context"
	"image"
)

const (
	TransferPQ  = 16
	TransferHLG = 18
)

func (c CICP) HDR() bool { return c.Transfer == TransferPQ || c.Transfer == TransferHLG }

func (c ColorInfo) HDR() bool { return c.CICP != nil && c.CICP.HDR() }

type ColorTileReader interface {
	Color() ColorInfo
}

type DeepTileReader interface {
	TileReader
	Deep() bool
	OverviewDeep(ctx context.Context, shift int) (*image.NRGBA64, error)
	RegionDeep(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA64, error)
}

type InkTileReader interface {
	TileReader
	Ink() bool
	OverviewInk(ctx context.Context, shift int) (*image.CMYK, error)
	RegionInk(ctx context.Context, shift int, r image.Rectangle) (*image.CMYK, error)
}
