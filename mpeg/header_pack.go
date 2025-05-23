package mpeg

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// PackHeader PS流包头. [2.5.3.3]
type PackHeader struct {
	systemClockReferenceBase      int64  // 33
	systemClockReferenceExtension uint16 // 9
	programMuxRate                uint32 // 22
	stuffing                      []byte
	mpeg2                         bool
}

func (h *PackHeader) Marshal(dst []byte) int {
	binary.BigEndian.PutUint32(dst, PackHeaderStartCode)
	// 2bits 01
	dst[4] = 0x40
	// 3bits [32..30]
	dst[4] = dst[4] | (byte(h.systemClockReferenceBase>>30) << 3)
	// 1bit marker bit
	dst[4] = dst[4] | 0x4
	// 15bits [29..15]
	// 2bits 29 28
	dst[4] = dst[4] | byte(h.systemClockReferenceBase>>28&0x3)
	// 8bits
	dst[5] = byte(h.systemClockReferenceBase >> 20)
	// 5bits
	dst[6] = byte(h.systemClockReferenceBase >> 12 & 0xF8)
	dst[6] = dst[6] | 0x4
	// 15bits [14:0]
	// 2bits
	dst[6] = dst[6] | byte(h.systemClockReferenceBase>>13&0x3)
	dst[7] = byte(h.systemClockReferenceBase >> 5)
	// 5bits
	dst[8] = byte(h.systemClockReferenceBase&0x1f) << 3
	dst[8] = dst[8] | 0x4

	dst[8] = dst[8] | byte(h.systemClockReferenceExtension>>7&0x3)
	dst[9] = byte(h.systemClockReferenceExtension) << 1
	// 1bits mark bit
	dst[9] = dst[9] | 0x1

	dst[10] = byte(h.programMuxRate >> 14)
	dst[11] = byte(h.programMuxRate >> 6)
	dst[12] = byte(h.programMuxRate) << 2
	// 2bits 2 mark bit
	dst[12] = dst[12] | 0x3

	// 5bits reserved
	// 3bits pack_stuffing_length
	dst[13] = 0xF8
	offset := 14
	if h.stuffing != nil {
		length := len(h.stuffing)
		dst[13] = dst[13] | byte(length)
		copy(dst[offset:], h.stuffing)
		offset += length
	}

	return offset
}

func (h *PackHeader) Unmarshal(data []byte) int {
	length := len(data)
	if length < 14 {
		return 0
	}

	if PackHeaderStartCode != binary.BigEndian.Uint32(data) {
		fmt.Printf("Expected PackHeaderStartCode: 0x%04X, Actual: 0x%04X\r\n", PackHeaderStartCode, binary.BigEndian.Uint32(data))
	}

	h.mpeg2 = data[4]&0xC0 == 0
	// mpeg1 版本占用4bits 没有clockExtension reserved stuffingLength
	h.systemClockReferenceBase = int64(data[4]&0x38)<<27 | (int64(data[4]&0x3) << 28) | (int64(data[5]) << 20) | (int64(data[6]&0xF8) << 12) | (int64(data[6]&0x3) << 13) | (int64(data[7]) << 5) | (int64(data[8] & 0xF8 >> 3))

	h.systemClockReferenceExtension = uint16(data[8]&0x3) << 7
	h.systemClockReferenceExtension = h.systemClockReferenceExtension | uint16(data[9]>>1)

	h.programMuxRate = uint32(data[10]) << 14
	h.programMuxRate = h.programMuxRate | uint32(data[11])<<6
	h.programMuxRate = h.programMuxRate | uint32(data[12]>>2)

	packStuffingLength := 14 + int(data[13]&0x7)
	if packStuffingLength > length {
		return 0
	}

	h.stuffing = data[14:packStuffingLength]
	return packStuffingLength
}

func (h *PackHeader) String() string {
	if h.stuffing == nil {
		return fmt.Sprintf("systemClockReferenceBase=%d\r\nsystemClockReferenceExtension=%d\r\nprogramMuxRate=%d\r\n", h.systemClockReferenceBase,
			h.systemClockReferenceExtension, h.programMuxRate)
	} else {
		return fmt.Sprintf("systemClockReferenceBase=%d\r\nsystemClockReferenceExtension=%d\r\nprogramMuxRate=%d\r\nstuffingLength=%d\r\nstuffing=%s\r\n", h.systemClockReferenceBase,
			h.systemClockReferenceExtension, h.programMuxRate, len(h.stuffing), hex.EncodeToString(h.stuffing))
	}
}
