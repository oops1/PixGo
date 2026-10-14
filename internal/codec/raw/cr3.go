package raw

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	maxBoxes       = 4096
	uuidSize       = 16
	boxHeaderSize  = 8
	largeBoxHeader = 16
	maxThumbBytes  = 4 << 20
)

var canonUUID = []byte{0x85, 0xc0, 0xb6, 0x87, 0x82, 0x0f, 0x11, 0xe0, 0x81, 0x11, 0xf4, 0xce, 0x46, 0x2b, 0x6a, 0x48}

type box struct {
	kind    string
	payload int64
	end     int64
}

func readBox(r io.ReaderAt, size, position, limit int64) (box, error) {
	var head [largeBoxHeader]byte
	take := min(int64(len(head)), limit-position)
	if err := guard.ReadAtInto(r, head[:take], position, size); err != nil {
		return box{}, err
	}
	total := int64(binary.BigEndian.Uint32(head[0:4]))
	b := box{kind: string(head[4:8]), payload: position + boxHeaderSize}
	switch total {
	case 1:
		if take < largeBoxHeader {
			return box{}, guard.Corrupt("raw: large box header at %d is cut off", position)
		}
		large := binary.BigEndian.Uint64(head[8:16])
		if large < largeBoxHeader || large > uint64(limit-position) {
			total = limit - position
		} else {
			total = int64(large)
		}
		b.payload = position + largeBoxHeader
	case 0:
		total = limit - position
	}
	if total < b.payload-position {
		return box{}, guard.Corrupt("raw: box %q at %d is %d bytes", b.kind, position, total)
	}
	b.end = position + total
	if b.end > limit {
		b.end = limit
	}
	return b, nil
}

func eachBox(ctx context.Context, r io.ReaderAt, size, start, end int64, visit func(box) error) error {
	position := start
	for count := 0; count < maxBoxes && end-position >= boxHeaderSize; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := readBox(r, size, position, end)
		if err != nil {
			return err
		}
		if err := visit(b); err != nil {
			return err
		}
		position = b.end
	}
	return nil
}

type cr3 struct {
	ctx            context.Context
	r              io.ReaderAt
	size           int64
	f              *finder
	orientation    codec.Orientation
	hasOrientation bool
}

func discoverCR3(ctx context.Context, r io.ReaderAt, size int64, f *finder) (codec.Orientation, bool, error) {
	c := &cr3{ctx: ctx, r: r, size: size, f: f}
	err := eachBox(ctx, r, size, 0, size, func(b box) error {
		switch b.kind {
		case "moov":
			return eachBox(ctx, r, size, b.payload, b.end, c.movieChild)
		case "uuid":
			return c.carveUUID(b)
		case "mdat":
			return f.addWithin(b.payload, min(b.end-b.payload, maxCarveBytes))
		}
		return nil
	})
	return c.orientation, c.hasOrientation, err
}

func (c *cr3) movieChild(b box) error {
	if b.kind != "uuid" {
		return nil
	}
	var id [uuidSize]byte
	if b.end-b.payload < uuidSize {
		return nil
	}
	if err := guard.ReadAtInto(c.r, id[:], b.payload, c.size); err != nil {
		return err
	}
	if !bytes.Equal(id[:], canonUUID) {
		return c.carveUUID(b)
	}
	return eachBox(c.ctx, c.r, c.size, b.payload+uuidSize, b.end, c.canonChild)
}

func (c *cr3) carveUUID(b box) error {
	if b.end-b.payload <= uuidSize {
		return nil
	}
	return c.f.addWithin(b.payload+uuidSize, min(b.end-b.payload-uuidSize, maxCarveBytes))
}

func (c *cr3) canonChild(b box) error {
	switch b.kind {
	case "CMT1":
		if !c.hasOrientation {
			if o, ok := exif.OrientationAt(c.r, b.payload, b.end-b.payload); ok {
				c.orientation, c.hasOrientation = o, true
			}
		}
	case "THMB":
		return c.f.addWithin(b.payload, min(b.end-b.payload, maxThumbBytes))
	}
	return nil
}
