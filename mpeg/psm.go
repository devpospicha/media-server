package mpeg

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// ProgramStreamMap 记录ps流的编码器信息
type ProgramStreamMap struct {
	streamId             byte
	currentNextIndicator byte //1 bit
	version              byte //5 bits
	info                 []byte
	elementaryStreams    []ElementaryStream
	crc32                uint32
}

func (h *ProgramStreamMap) findElementaryStream(streamId byte) (ElementaryStream, bool) {
	if h.elementaryStreams == nil {
		return ElementaryStream{}, false
	}

	for _, element := range h.elementaryStreams {
		if element.streamId == streamId {
			return element, true
		}
	}

	return ElementaryStream{}, false
}

func (h *ProgramStreamMap) String() string {
	var info string
	if h.info != nil {
		info = hex.EncodeToString(h.info)
	}

	var elements string
	if h.elementaryStreams != nil {
		for _, element := range h.elementaryStreams {
			elements += element.String()
		}
	}

	return fmt.Sprintf("streamId=%x\r\ncurrentNextIndicator=%d\r\nversion=%d\r\ninfo=%s\r\nelements=[\r\n%s]\r\ncrc32=%d\r\n",
		h.streamId, h.currentNextIndicator, h.version, info, elements, h.crc32)
}

func readProgramStreamMap(header *ProgramStreamMap, src []byte) (int, error) {
	length := len(src)
	if length < 16 {
		return -1, nil
	}
	totalLength := 6 + int(binary.BigEndian.Uint16(src[4:]))
	if totalLength > length {
		return -1, nil
	}

	header.streamId = src[3]
	header.currentNextIndicator = src[6] >> 7
	header.version = src[6] & 0x1F

	infoLength := int(binary.BigEndian.Uint16(src[8:]))
	offset := 10
	if infoLength > 0 {
		// +2 reserved elementary_stream_map_length
		if 10+2+infoLength > totalLength-4 {
			return -1, fmt.Errorf("invalid data:%s", hex.EncodeToString(src))
		}

		offset += infoLength
		header.info = src[10:offset]
	}

	elementaryLength := int(binary.BigEndian.Uint16(src[offset:]))
	offset += 2
	if offset+elementaryLength > totalLength-4 {
		return -1, fmt.Errorf("invalid data:%s", hex.EncodeToString(src))
	}

	for i := offset; i < offset+elementaryLength; i += 4 {
		eInfoLength := int(binary.BigEndian.Uint16(src[i+2:]))

		if _, ok := header.findElementaryStream(src[i+1]); !ok {
			element := ElementaryStream{}
			element.streamType = src[i]
			element.streamId = src[i+1]

			if eInfoLength > 0 {
				//if i+4+eInfoLength > offset+elementaryLength {
				if i+4+eInfoLength > totalLength-4 {
					return 0, fmt.Errorf("invalid data:%s", hex.EncodeToString(src))
				}
				element.info = src[i+4 : i+4+eInfoLength]
			}

			header.elementaryStreams = append(header.elementaryStreams, element)
		}

		i += eInfoLength
	}

	header.crc32 = binary.BigEndian.Uint32(src[totalLength-4:])
	return totalLength, nil
}

func (h *ProgramStreamMap) Unmarshal(data []byte) int {
	length := len(data)
	if length < 16 {
		return 0
	}

	totalLength := 6 + int(binary.BigEndian.Uint16(data[4:]))
	if totalLength > length {
		return 0
	}

	h.streamId = data[3]
	h.currentNextIndicator = data[6] >> 7
	h.version = data[6] & 0x1F

	infoLength := int(binary.BigEndian.Uint16(data[8:]))
	offset := 10
	if infoLength > 0 {
		// +2 reserved elementary_stream_map_length
		if 10+2+infoLength > totalLength-4 {
			return 0
		}

		offset += infoLength
		h.info = data[10:offset]
	}

	elementaryLength := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+elementaryLength > totalLength-4 {
		return 0
	}

	end := totalLength - 4
	for i := offset + 4; i <= end; i += 4 {
		streamType := data[i-4]
		streamId := data[i-3]
		eInfoLength := binary.BigEndian.Uint16(data[i-2:])

		i += int(eInfoLength)
		if _, ok := h.findElementaryStream(streamId); !ok {
			element := ElementaryStream{}
			element.streamType = streamType
			element.streamId = streamId
			if eInfoLength > 0 {
				// copy
			}
			h.elementaryStreams = append(h.elementaryStreams, element)
		}
	}

	h.crc32 = binary.BigEndian.Uint32(data[totalLength-4:])
	return totalLength
}

func (h *ProgramStreamMap) Marshal(dst []byte) int {
	binary.BigEndian.PutUint32(dst, PSMStartCode)
	// current_next_indicator
	dst[6] = 0x80
	// reserved
	dst[6] = dst[6] | (0x3 << 5)
	// program_stream_map_version
	dst[6] = dst[6] | 0x1
	// reserved
	dst[7] = 0xFE
	// mark bit
	dst[7] = dst[7] | 0x1

	offset := 10
	if h.info != nil {
		length := len(h.info)
		copy(dst[offset:], h.info)
		binary.BigEndian.PutUint16(dst[8:], uint16(length))
		offset += length
	} else {
		binary.BigEndian.PutUint16(dst[8:], 0)
	}
	//elementary length
	offset += 2
	temp := offset
	for _, elementaryStream := range h.elementaryStreams {
		dst[offset] = elementaryStream.streamType
		offset++
		dst[offset] = elementaryStream.streamId
		offset += 3
		if elementaryStream.info != nil {
			length := len(elementaryStream.info)
			copy(dst[offset:], elementaryStream.info)
			binary.BigEndian.PutUint16(dst[offset-2:], uint16(length))
			offset += length
		} else {
			binary.BigEndian.PutUint16(dst[offset-2:], 0)
		}
	}

	elementaryLength := offset - temp
	binary.BigEndian.PutUint16(dst[temp-2:], uint16(elementaryLength))

	crc32 := CalculateCrcMpeg2(dst[:offset])
	binary.BigEndian.PutUint32(dst[offset:], crc32)

	offset += 4
	binary.BigEndian.PutUint16(dst[4:], uint16(offset-6))

	return offset
}

type ElementaryStream struct {
	streamType byte // 2-34. 0x5 disable
	streamId   byte
	info       []byte
}

func (e ElementaryStream) String() string {
	if e.info == nil {
		return fmt.Sprintf("StreamType=%x\r\nstreamId=%x\r\n", e.streamType, e.streamId)
	} else {
		return fmt.Sprintf("StreamType=%x\r\nstreamId=%x\r\ninfo=%s\r\n", e.streamType, e.streamId, hex.EncodeToString(e.info))
	}
}
