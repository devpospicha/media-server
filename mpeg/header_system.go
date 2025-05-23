package mpeg

import (
	"encoding/binary"
	"fmt"
)

// SystemHeader [2.5.3.5] 记录stream id: 00 00 00 `E0`, 00 00 00 `C0`
type SystemHeader struct {
	rateBound                 uint32 // 22
	audioBound                byte   // 6 [0,32]
	fixedFlag                 byte   // 1
	cspsFlag                  byte   // 1
	systemAudioLockFlag       byte   // 1
	systemVideoLockFlag       byte   // 1
	videoBound                byte   // 5 [0,16]
	packetRateRestrictionFlag byte   // 1

	streams []StreamHeader
}

func (h *SystemHeader) findStream(id byte) (StreamHeader, bool) {
	if h.streams == nil {
		return StreamHeader{}, false
	}
	for _, s := range h.streams {
		if s.streamId == id {
			return s, true
		}
	}

	return StreamHeader{}, false
}

func (h *SystemHeader) Unmarshal(data []byte) int {
	length := len(data)
	if length < 6 {
		return 0
	}

	totalLength := int(6 + binary.BigEndian.Uint16(data[4:]))
	if totalLength > length {
		return 0
	}

	h.rateBound = uint32(data[6]) & 0x7E << 15
	h.rateBound = h.rateBound | uint32(data[7])<<7
	h.rateBound = h.rateBound | uint32(data[8]>>1)

	h.audioBound = data[9] >> 2
	h.fixedFlag = data[9] >> 1 & 0x1
	h.cspsFlag = data[9] & 0x1

	h.systemAudioLockFlag = data[10] >> 7
	h.systemVideoLockFlag = data[10] >> 6 & 0x1
	h.videoBound = data[10] & 0x1F
	h.packetRateRestrictionFlag = data[11] >> 7

	offset := 12
	for ; offset < totalLength && (data[offset]&0x80) == 0x80 && (totalLength-offset)%3 == 0; offset += 3 {
		if _, ok := h.findStream(data[offset]); ok {
			continue
		}

		sHeader := StreamHeader{}
		sHeader.streamId = data[offset]
		sHeader.bufferBoundScale = data[offset+1] >> 5 & 0x1
		sHeader.bufferSizeBound = uint16(data[offset+1]&0x1F) << 8
		sHeader.bufferSizeBound = sHeader.bufferSizeBound | uint16(data[offset+2])
		h.streams = append(h.streams, sHeader)
	}

	return totalLength
}

func (h *SystemHeader) Marshal(dst []byte) int {
	binary.BigEndian.PutUint32(dst, SystemHeaderStartCode)
	dst[6] = 0x80
	dst[6] = dst[6] | byte(h.rateBound>>15)
	dst[7] = byte(h.rateBound >> 7)
	dst[8] = byte(h.rateBound) << 1
	//mark bit
	dst[8] = dst[8] | 0x1
	dst[9] = h.audioBound << 2
	dst[9] = dst[9] | (h.fixedFlag << 1)
	dst[9] = dst[9] | h.cspsFlag

	dst[10] = h.systemAudioLockFlag << 7
	dst[10] = dst[10] | (h.systemVideoLockFlag << 6)
	dst[10] = dst[10] | 0x20
	dst[10] = dst[10] | h.videoBound

	dst[11] = h.packetRateRestrictionFlag << 7
	dst[11] = dst[11] | 0x7F

	offset := 12
	for i, s := range h.streams {
		s.Marshal(dst[offset:])
		offset += (i + 1) * 3
	}

	binary.BigEndian.PutUint16(dst[4:], uint16(offset-6))
	return offset
}

func (h *SystemHeader) String() string {
	sprintf := fmt.Sprintf(
		"rateBound=%d\r\n"+
			"audioBound=%d\r\n"+
			"fixedFlag=%d\r\n"+
			"cspsFlag=%d\r\n"+
			"systemAudioLockFlag=%d\r\n"+
			"systemVideoLockFlag=%d\r\n"+
			"videoBound=%d\r\n"+
			"packetRateRestrictionFlag=%d\r\n",
		h.rateBound,
		h.audioBound,
		h.fixedFlag,
		h.cspsFlag,
		h.systemAudioLockFlag,
		h.systemVideoLockFlag,
		h.videoBound,
		h.packetRateRestrictionFlag,
	)

	sprintf += "streams=[\r\n"
	for _, stream := range h.streams {
		sprintf += stream.String()
	}
	sprintf += "]\r\n"

	return sprintf
}

// StreamHeader 3bytes.
type StreamHeader struct {
	streamId byte
	//'11'
	bufferBoundScale byte   //1
	bufferSizeBound  uint16 //13
}

func (h *StreamHeader) String() string {
	return fmt.Sprintf("streamId=%x\r\nbufferBoundScale=%d\r\nbufferSizeBound=%d\r\n", h.streamId, h.bufferBoundScale, h.bufferSizeBound)
}

func (h *StreamHeader) Marshal(data []byte) {
	data[0] = h.streamId
	data[1] = 0xc0
	data[1] = data[1] | (h.bufferBoundScale << 5)
	data[1] = data[1] | byte(h.bufferSizeBound&0x1F00>>8)
	data[2] = byte(h.bufferSizeBound & 0xFF)
}
