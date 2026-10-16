package jpegxform

import (
	"context"
	"errors"
	"io"
)

const (
	blockSize = 64
	dcLimit   = 16383
	maxDCSize = 15
)

var zigzag = [blockSize]uint8{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

type grid struct {
	rows       [][]int16
	wide, high int
}

func (g *grid) block(bx, by int) []int16 {
	return g.rows[by][bx*blockSize : bx*blockSize+blockSize : bx*blockSize+blockSize]
}

func ioFailure(err error) error {
	if err == nil || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func decodeScan(ctx context.Context, src *byteSource, h *header) ([]grid, error) {
	grids, err := decodeScanBlocks(ctx, src, h)
	if err != nil {
		if ioErr := ioFailure(src.err); ioErr != nil {
			return nil, ioErr
		}
		return nil, err
	}
	return grids, nil
}

func decodeScanBlocks(ctx context.Context, src *byteSource, h *header) ([]grid, error) {
	fr := &h.fr
	nc := len(fr.comps)
	dcTables := make([]*huffDecoder, nc)
	acTables := make([]*huffDecoder, nc)
	grids := make([]grid, nc)
	for i := range fr.comps {
		c := &fr.comps[i]
		var err error
		if dcTables[i], err = newHuffDecoder(h.dc[c.dcTable]); err != nil {
			return nil, err
		}
		if acTables[i], err = newHuffDecoder(h.ac[c.acTable]); err != nil {
			return nil, err
		}
		grids[i] = grid{rows: make([][]int16, c.blocksHigh), wide: c.blocksWide, high: c.blocksHigh}
	}
	br := newBitReader(src)
	preds := make([]int, nc)
	count := 0
	for my := 0; my < fr.mcusY; my++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for i := range fr.comps {
			c := &fr.comps[i]
			for v := 0; v < c.v; v++ {
				grids[i].rows[my*c.v+v] = make([]int16, c.blocksWide*blockSize)
			}
		}
		for mx := 0; mx < fr.mcusX; mx++ {
			if h.restart > 0 && count > 0 && count%h.restart == 0 {
				if err := br.restart(); err != nil {
					return nil, err
				}
				for i := range preds {
					preds[i] = 0
				}
			}
			for _, i := range h.scan {
				c := &fr.comps[i]
				for v := 0; v < c.v; v++ {
					row := grids[i].rows[my*c.v+v]
					for hh := 0; hh < c.h; hh++ {
						at := (mx*c.h + hh) * blockSize
						if err := decodeBlock(br, row[at:at+blockSize:at+blockSize], dcTables[i], acTables[i], &preds[i]); err != nil {
							return nil, err
						}
					}
				}
			}
			if br.overrun() {
				return nil, corrupt("scan data ends early")
			}
			count++
		}
	}
	return grids, nil
}

func decodeBlock(br *bitReader, blk []int16, dc, ac *huffDecoder, pred *int) error {
	s, ok := br.decode(dc)
	if !ok || s > maxDCSize {
		return corrupt("bad DC code")
	}
	if s > 0 {
		*pred += br.receive(s)
		if *pred > dcLimit || *pred < -dcLimit-1 {
			return corrupt("DC value %d out of range", *pred)
		}
	}
	blk[0] = int16(*pred)
	for k := 1; k < blockSize; {
		rs, ok := br.decode(ac)
		if !ok {
			return corrupt("bad AC code")
		}
		r, s := rs>>4, rs&15
		if s == 0 {
			if r != 15 {
				break
			}
			k += 16
			continue
		}
		k += r
		if k >= blockSize {
			return corrupt("AC run beyond the block")
		}
		blk[zigzag[k]] = int16(br.receive(s))
		k++
	}
	return nil
}
