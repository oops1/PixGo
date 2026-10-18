package jxl

import (
	"math"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/assemble"
)

const (
	enumWhiteD65 = 1
	enumWhiteDCI = 11

	enumPrimariesSRGB   = 1
	enumPrimariesBT2100 = 9
	enumPrimariesP3     = 11

	enumTransferBT709  = 1
	enumTransferLinear = 8
	enumTransferSRGB   = 13
	enumTransferPQ     = 16
	enumTransferHLG    = 18

	cicpBT709     = 1
	cicpBT2020    = 9
	cicpSMPTE431  = 11
	cicpDisplayP3 = 12

	cicpGamma22 = 4
	cicpGamma28 = 5

	gammaDenominator = 1e7
	gammaTolerance   = 0.02
)

func (h imageHeader) cicp() *codec.CICP {
	if h.embeddedProfile || (h.colorSpace != colorSpaceRGB && h.colorSpace != colorSpaceGray) {
		return nil
	}
	enc := h.encoding
	var primaries uint8
	switch {
	case h.colorSpace == colorSpaceGray:
		if enc.whitePoint != enumWhiteD65 {
			return nil
		}
		primaries = cicpBT709
	case enc.primaries == enumPrimariesSRGB && enc.whitePoint == enumWhiteD65:
		primaries = cicpBT709
	case enc.primaries == enumPrimariesBT2100 && enc.whitePoint == enumWhiteD65:
		primaries = cicpBT2020
	case enc.primaries == enumPrimariesP3 && enc.whitePoint == enumWhiteD65:
		primaries = cicpDisplayP3
	case enc.primaries == enumPrimariesP3 && enc.whitePoint == enumWhiteDCI:
		primaries = cicpSMPTE431
	default:
		return nil
	}
	var transfer uint8
	if enc.haveGamma {
		if enc.gamma == 0 {
			return nil
		}
		exponent := gammaDenominator / float64(enc.gamma)
		switch {
		case math.Abs(exponent-1) < gammaTolerance:
			transfer = enumTransferLinear
		case math.Abs(exponent/2.2-1) < gammaTolerance:
			transfer = cicpGamma22
		case math.Abs(exponent/2.8-1) < gammaTolerance:
			transfer = cicpGamma28
		default:
			return nil
		}
	} else {
		switch enc.transfer {
		case enumTransferBT709, enumTransferLinear, enumTransferSRGB, enumTransferPQ, enumTransferHLG:
			transfer = uint8(enc.transfer)
		default:
			return nil
		}
	}
	return &codec.CICP{Primaries: primaries, Transfer: transfer, FullRange: true}
}

func (h imageHeader) color(icc []byte) codec.ColorInfo {
	if h.embeddedProfile {
		return assemble.Color(icc, 2, 2, 0, true)
	}
	if cicp := h.cicp(); cicp != nil {
		return assemble.Color(nil, uint16(cicp.Primaries), uint16(cicp.Transfer), uint16(cicp.Matrix), cicp.FullRange)
	}
	return codec.ColorInfo{}
}
