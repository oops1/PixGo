package isobmff

import (
	"context"
	"io"
)

type Span struct {
	Type    string
	Start   int64
	Payload int64
	End     int64
}

func (s Span) Len() int64 { return s.End - s.Payload }

func Walk(ctx context.Context, r io.ReaderAt, size, start, end int64, maxBoxes int, visit func(Span) error) error {
	if start < 0 || start > end || end > size {
		return corrupt("heif: box range %d..%d outside %d bytes", start, end, size)
	}
	position := start
	for count := 0; end-position >= boxHeaderSize; count++ {
		if count >= maxBoxes {
			return tooLarge("heif: more than %d boxes in one level", maxBoxes)
		}
		if count%contextInterval == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		var head [largeBoxHeaderSize]byte
		window := min(int64(len(head)), end-position)
		if err := readAtInto(r, head[:window], position, size); err != nil {
			return err
		}
		header, err := parseBoxHeader(head[:window], uint64(end-position))
		if err != nil {
			return err
		}
		span := Span{
			Type:    header.kind,
			Start:   position,
			Payload: position + int64(header.headLen),
			End:     position + int64(header.total),
		}
		if err := visit(span); err != nil {
			return err
		}
		position = span.End
	}
	return nil
}
