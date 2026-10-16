package tiff

import (
	"bufio"
	"compress/zlib"
	"context"
	"errors"
	"image"
	"io"
	"sync"

	xlzw "golang.org/x/image/tiff/lzw"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const streamBufferSize = 32 << 10

type bytePool struct {
	pool sync.Pool
}

func (p *bytePool) get(n int) []byte {
	if p != nil {
		if v := p.pool.Get(); v != nil {
			if buf := v.(*[]byte); cap(*buf) >= n {
				return (*buf)[:n]
			}
		}
	}
	return make([]byte, n)
}

func (p *bytePool) put(buf []byte) {
	buf = buf[:0]
	p.pool.Put(&buf)
}

type streamPools struct {
	bands   bytePool
	rows    bytePool
	readers sync.Pool
	zlibs   sync.Pool
}

func (p *streamPools) reader(src io.Reader) *bufio.Reader {
	if v := p.readers.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(src)
		return br
	}
	return bufio.NewReaderSize(src, streamBufferSize)
}

func (p *streamPools) putReader(br *bufio.Reader) {
	br.Reset(nil)
	p.readers.Put(br)
}

type chunkStream struct {
	lay       *layout
	pools     *streamPools
	source    io.Reader
	buffered  *bufio.Reader
	lzw       io.Closer
	zlib      io.ReadCloser
	row       []byte
	validW    int
	validRows int
	consumed  int
}

type packBitsReader struct {
	source  *bufio.Reader
	literal int
	repeat  int
	value   byte
}

func (p *packBitsReader) Read(out []byte) (int, error) {
	written := 0
	for written < len(out) {
		switch {
		case p.literal > 0:
			n, err := p.source.Read(out[written:min(len(out), written+p.literal)])
			written += n
			p.literal -= n
			if err != nil {
				return written, unexpected(err)
			}
		case p.repeat > 0:
			n := min(p.repeat, len(out)-written)
			for i := 0; i < n; i++ {
				out[written+i] = p.value
			}
			written += n
			p.repeat -= n
		default:
			header, err := p.source.ReadByte()
			if err != nil {
				if written > 0 && err == io.EOF {
					return written, nil
				}
				return written, err
			}
			switch {
			case header < 128:
				p.literal = int(header) + 1
			case header > 128:
				p.value, err = p.source.ReadByte()
				if err != nil {
					return written, unexpected(err)
				}
				p.repeat = 257 - int(header)
			}
		}
	}
	return written, nil
}

func unexpected(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

func (rd *reader) openChunk(ctx context.Context, index, skipRows int) (*chunkStream, error) {
	lay := rd.lay
	_, _, validW, validRows := lay.chunkRect(index)
	offset, length := int64(lay.offsets[index]), int64(lay.counts[index])
	consumed := skipRows
	if lay.compression == compressionNone {
		skip := int64(skipRows) * int64(lay.rowBytes)
		offset += skip
		length -= skip
		skipRows = 0
		if length < 0 {
			return nil, guard.Corrupt("tiff: chunk %d is shorter than its rows", index)
		}
	}
	section := io.NewSectionReader(guard.NewReader(ctx, rd.r, rd.size), offset, length)
	cs := &chunkStream{
		lay:       lay,
		pools:     &rd.pools,
		buffered:  rd.pools.reader(section),
		row:       rd.pools.rows.get(lay.rowBytes),
		validW:    validW,
		validRows: validRows,
		consumed:  consumed,
	}
	switch lay.compression {
	case compressionNone:
		cs.source = cs.buffered
	case compressionLZW:
		lzwReader := xlzw.NewReader(cs.buffered, xlzw.MSB, 8)
		cs.source, cs.lzw = lzwReader, lzwReader
	case compressionDeflate:
		if err := cs.openZlib(ctx); err != nil {
			cs.close()
			return nil, err
		}
	case compressionPackBits:
		cs.source = &packBitsReader{source: cs.buffered}
	}
	if lay.compression != compressionNone && skipRows > 0 {
		if _, err := io.CopyN(io.Discard, cs.source, int64(skipRows)*int64(lay.rowBytes)); err != nil {
			cs.close()
			return nil, classifyStream(ctx, err)
		}
	}
	return cs, nil
}

func (cs *chunkStream) openZlib(ctx context.Context) error {
	if v := cs.pools.zlibs.Get(); v != nil {
		reader := v.(io.ReadCloser)
		if err := reader.(zlib.Resetter).Reset(cs.buffered, nil); err != nil {
			return classifyStream(ctx, err)
		}
		cs.zlib, cs.source = reader, reader
		return nil
	}
	reader, err := zlib.NewReader(cs.buffered)
	if err != nil {
		return classifyStream(ctx, err)
	}
	cs.zlib, cs.source = reader, reader
	return nil
}

func (cs *chunkStream) close() {
	if cs.lzw != nil {
		cs.lzw.Close()
	}
	if cs.zlib != nil {
		cs.pools.zlibs.Put(cs.zlib)
		cs.zlib = nil
	}
	cs.pools.putReader(cs.buffered)
	cs.pools.rows.put(cs.row)
	cs.buffered, cs.row = nil, nil
}

func classifyStream(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return guard.Corrupt("tiff: chunk data ends early")
	}
	return guard.Classify(ctx, err, nil)
}

func (cs *chunkStream) readBand(ctx context.Context, rows int, bands *bytePool) (*image.NRGBA, error) {
	lay := cs.lay
	if rows <= 0 || cs.consumed+rows > cs.validRows {
		return nil, guard.Corrupt("tiff: band of %d rows beyond the chunk", rows)
	}
	img := &image.NRGBA{Pix: bands.get(cs.validW * rows * 4), Stride: cs.validW * 4, Rect: image.Rect(0, 0, cs.validW, rows)}
	lastRowBytes := rowBytesFor(lay, cs.validW)
	for r := 0; r < rows; r++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		want := lay.rowBytes
		if cs.consumed+1 == cs.validRows {
			want = lastRowBytes
		}
		if _, err := io.ReadFull(cs.source, cs.row[:want]); err != nil {
			return nil, classifyStream(ctx, err)
		}
		cs.consumed++
		if lay.predictor {
			lay.undoPredictor(cs.row, cs.validW)
		}
		if err := lay.convertRow(img.Pix[r*img.Stride:r*img.Stride+cs.validW*4], cs.row, cs.validW); err != nil {
			return nil, err
		}
	}
	return img, nil
}
