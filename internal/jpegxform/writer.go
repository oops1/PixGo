package jpegxform

import (
	"context"
	"io"
)

const maxTableGroups = 4

type tableGroups struct {
	ids    []int
	ofComp []int
}

func groupTables(h *header, pick func(c *component) int) tableGroups {
	g := tableGroups{ofComp: make([]int, len(h.fr.comps))}
	for _, ci := range h.scan {
		id := pick(&h.fr.comps[ci])
		at := -1
		for i, known := range g.ids {
			if known == id {
				at = i
			}
		}
		if at < 0 {
			at = len(g.ids)
			g.ids = append(g.ids, id)
		}
		g.ofComp[ci] = at
	}
	return g
}

func walkBlocks(ctx context.Context, h *header, grids []grid, pl *plan, visit func(comp int, blk []int16)) error {
	var scratch [blockSize]int16
	for my := 0; my < pl.outMY; my++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for mx := 0; mx < pl.outMX; mx++ {
			for _, ci := range h.scan {
				pc := &pl.comps[ci]
				for v := 0; v < pc.outV; v++ {
					for hh := 0; hh < pc.outH; hh++ {
						visit(ci, pl.fetch(&grids[ci], ci, mx*pc.outH+hh, my*pc.outV+v, &scratch))
					}
				}
			}
		}
	}
	return nil
}

func writeJPEG(ctx context.Context, w io.Writer, h *header, grids []grid, pl *plan) error {
	dcGroups := groupTables(h, func(c *component) int { return c.dcTable })
	acGroups := groupTables(h, func(c *component) int { return c.acTable })
	nc := len(h.fr.comps)
	var dcFreq, acFreq [maxTableGroups][symbolSentinel + 1]int
	preds := make([]int, nc)
	counter := &blockCoder{}
	dcRefs := make([]tableRef, nc)
	acRefs := make([]tableRef, nc)
	for ci := 0; ci < nc; ci++ {
		dcRefs[ci] = tableRef{freq: &dcFreq[dcGroups.ofComp[ci]]}
		acRefs[ci] = tableRef{freq: &acFreq[acGroups.ofComp[ci]]}
	}
	err := walkBlocks(ctx, h, grids, pl, func(ci int, blk []int16) {
		preds[ci] = counter.block(blk, preds[ci], &dcRefs[ci], &acRefs[ci])
	})
	if err != nil {
		return err
	}
	dcSpecs := make([]tableSpec, len(dcGroups.ids))
	acSpecs := make([]tableSpec, len(acGroups.ids))
	dcEnc := make([]*encoderTable, len(dcSpecs))
	acEnc := make([]*encoderTable, len(acSpecs))
	for i := range dcSpecs {
		dcSpecs[i] = optimalSpec(&dcFreq[i])
		dcEnc[i] = dcSpecs[i].encoder()
	}
	for i := range acSpecs {
		acSpecs[i] = optimalSpec(&acFreq[i])
		acEnc[i] = acSpecs[i].encoder()
	}
	head := outputHeader(h, pl, dcSpecs, acSpecs, dcGroups, acGroups)
	if _, err := w.Write(head); err != nil {
		return err
	}
	out := newBitWriter(w)
	coder := &blockCoder{out: out}
	for ci := range preds {
		preds[ci] = 0
		dcRefs[ci].enc = dcEnc[dcGroups.ofComp[ci]]
		acRefs[ci].enc = acEnc[acGroups.ofComp[ci]]
	}
	err = walkBlocks(ctx, h, grids, pl, func(ci int, blk []int16) {
		preds[ci] = coder.block(blk, preds[ci], &dcRefs[ci], &acRefs[ci])
	})
	if err != nil {
		return err
	}
	if err := out.finish(); err != nil {
		return err
	}
	_, err = w.Write([]byte{0xFF, markerEOI})
	return err
}

func appendSegment(dst []byte, marker byte, payload []byte) []byte {
	n := len(payload) + 2
	dst = append(dst, 0xFF, marker, byte(n>>8), byte(n))
	return append(dst, payload...)
}

func transposeQuant(v [64]uint16) [64]uint16 {
	var out [64]uint16
	var natural, swapped [64]uint16
	for i := 0; i < 64; i++ {
		natural[zigzag[i]] = v[i]
	}
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			swapped[r*8+c] = natural[c*8+r]
		}
	}
	for i := 0; i < 64; i++ {
		out[i] = swapped[zigzag[i]]
	}
	return out
}

func outputHeader(h *header, pl *plan, dcSpecs, acSpecs []tableSpec, dcGroups, acGroups tableGroups) []byte {
	change := exifChange{
		oldWidth: h.fr.width, oldHeight: h.fr.height,
		newWidth: pl.outW, newHeight: pl.outH,
		swapResolution: pl.op.Transposes(),
		transformed:    pl.op != Identity,
	}
	out := []byte{0xFF, markerSOI}
	for _, s := range h.segments {
		if s.marker == markerAPP0+2 && isMPF(s.payload) {
			continue
		}
		fixExifSegment(s.payload, change)
		fixXMPSegment(s.payload)
		out = appendSegment(out, s.marker, s.payload)
	}
	var used [4]bool
	wide := false
	for _, c := range h.fr.comps {
		used[c.quant] = true
		wide = wide || h.quant[c.quant].wide
	}
	var dqt []byte
	for id := 0; id < 4; id++ {
		if !used[id] {
			continue
		}
		t := h.quant[id]
		values := t.values
		if pl.op.Transposes() {
			values = transposeQuant(values)
		}
		if t.wide {
			dqt = append(dqt, 0x10|byte(id))
			for _, v := range values {
				dqt = append(dqt, byte(v>>8), byte(v))
			}
			continue
		}
		dqt = append(dqt, byte(id))
		for _, v := range values {
			dqt = append(dqt, byte(v))
		}
	}
	out = appendSegment(out, markerDQT, dqt)
	sof := markerSOF0
	if wide || len(dcSpecs) > 2 || len(acSpecs) > 2 {
		sof = markerSOF1
	}
	frameHeader := []byte{8, byte(pl.outH >> 8), byte(pl.outH), byte(pl.outW >> 8), byte(pl.outW), byte(len(h.fr.comps))}
	for i, c := range h.fr.comps {
		pc := pl.comps[i]
		frameHeader = append(frameHeader, c.id, byte(pc.outH<<4|pc.outV), byte(c.quant))
	}
	out = appendSegment(out, byte(sof), frameHeader)
	for class, specs := range [][]tableSpec{dcSpecs, acSpecs} {
		for id, s := range specs {
			payload := []byte{byte(class<<4 | id)}
			payload = append(payload, s.counts[:]...)
			payload = append(payload, s.symbols...)
			out = appendSegment(out, markerDHT, payload)
		}
	}
	scanHeader := []byte{byte(len(h.scan))}
	for _, ci := range h.scan {
		scanHeader = append(scanHeader, h.fr.comps[ci].id, byte(dcGroups.ofComp[ci]<<4|acGroups.ofComp[ci]))
	}
	scanHeader = append(scanHeader, 0, 63, 0)
	return appendSegment(out, markerSOS, scanHeader)
}
