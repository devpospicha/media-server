package rtmp

import (
	"fmt"

	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
)

type ParserState byte

const (
	ParserStateInit              = ParserState(0)
	ParserStateBasicHeader       = ParserState(1)
	ParserStateTimestamp         = ParserState(2)
	ParserStateMessageLength     = ParserState(3)
	ParserStateStreamType        = ParserState(4)
	ParserStateStreamId          = ParserState(5)
	ParserStateExtendedTimestamp = ParserState(6)
	ParserStatePayload           = ParserState(7)
)

type Parser struct {
	state        ParserState
	extended     bool // 时间戳是否扩展
	headerOffset int

	chunks []*Chunk
	header Chunk
}

func (p *Parser) ReadChunk(data []byte, maxChunkSize int, onAvDataCb func(mediaType utils.AVMediaType, data []byte, first bool)) (*Chunk, int, error) {
	length, i := len(data), 0

	for i < length {
		switch p.state {

		case ParserStateInit:
			t := ChunkType(data[i] >> 6)
			if t > ChunkType3 {
				return nil, -1, fmt.Errorf("unknow chunk type: %d", int(t))
			}

			p.header.ChunkStreamID = 0
			if data[i]&0x3F == 0 {
				p.headerOffset = 1
			} else if data[i]&0x3F == 1 {
				p.headerOffset = 2
			} else {
				p.headerOffset = 0
				p.header.ChunkStreamID = ChunkStreamID(data[i] & 0x3F)
			}

			p.header.Type = t
			p.state = ParserStateBasicHeader
			i++
			break

		case ParserStateBasicHeader:
			for ; p.headerOffset > 0 && i < length; i++ {
				p.header.ChunkStreamID <<= 8
				p.header.ChunkStreamID |= ChunkStreamID(data[i])
				p.headerOffset--
			}

			if p.headerOffset == 0 {
				if p.header.Type < ChunkType3 {
					p.state = ParserStateTimestamp
				} else if p.extended {
					p.state = ParserStateExtendedTimestamp
				} else {
					p.state = ParserStatePayload
				}
			}
			break

		case ParserStateTimestamp:
			for ; p.headerOffset < 3 && i < length; i++ {
				p.header.Timestamp <<= 8
				p.header.Timestamp |= uint32(data[i])
				p.headerOffset++
			}

			if p.headerOffset == 3 {
				p.headerOffset = 0

				p.header.Timestamp &= 0xFFFFFF
				p.extended = p.header.Timestamp == 0xFFFFFF
				if p.header.Type < ChunkType2 {
					p.state = ParserStateMessageLength
				} else if p.extended {
					p.state = ParserStateExtendedTimestamp
				} else {
					p.state = ParserStatePayload
				}
			}
			break

		case ParserStateMessageLength:
			for ; p.headerOffset < 3 && i < length; i++ {
				p.header.Length <<= 8
				p.header.Length |= int(data[i])
				p.headerOffset++
			}

			if p.headerOffset == 3 {
				p.headerOffset = 0

				//24位有效
				p.header.Length &= 0x00FFFFFF
				p.state = ParserStateStreamType
			}
			break

		case ParserStateStreamType:
			p.header.TypeID = MessageTypeID(data[i])
			i++

			if p.header.Type == ChunkType0 {
				p.state = ParserStateStreamId
			} else if p.extended {
				p.state = ParserStateExtendedTimestamp
			} else {
				p.state = ParserStatePayload
			}
			break

		case ParserStateStreamId:
			for ; p.headerOffset < 4 && i < length; i++ {
				p.header.StreamID |= uint32(data[i]) << (8 * p.headerOffset)
				p.headerOffset++
			}

			if p.headerOffset == 4 {
				p.headerOffset = 0

				if p.extended {
					p.state = ParserStateExtendedTimestamp
				} else {
					p.state = ParserStatePayload
				}
			}
			break

		case ParserStateExtendedTimestamp:
			for ; p.headerOffset < 4 && i < length; i++ {
				p.header.Timestamp <<= 8
				p.header.Timestamp |= uint32(data[i])
				p.headerOffset++
			}

			if p.headerOffset == 4 {
				p.headerOffset = 0
				p.state = ParserStatePayload
			}
			break

		case ParserStatePayload:
			// 根据Chunk Stream ID, Type ID查找或创建对应的Chunk
			var chunk *Chunk

			if p.header.TypeID != 0 {
				for _, c := range p.chunks {
					if c.TypeID == p.header.TypeID {
						chunk = c
						break
					}
				}
			}

			if chunk == nil && p.header.ChunkStreamID != 0 {
				for _, c := range p.chunks {
					if c.ChunkStreamID == p.header.ChunkStreamID {
						chunk = c
						break
					}
				}
			}

			if chunk == nil {
				chunk = &Chunk{}
				*chunk = p.header
				p.chunks = append(p.chunks, chunk)
			}

			if p.header.StreamID != 0 {
				chunk.StreamID = p.header.StreamID
			}

			if p.header.Length > 0 && p.header.Length != chunk.Length {
				chunk.Length = p.header.Length
			}

			// 以第一包的type为准
			if chunk.Size == 0 {
				chunk.Type = p.header.Type
			}

			// 时间戳为0, 认为和上一个包相同. 这是一种常见的节省空间的做法.
			if p.header.Timestamp > 0 {
				chunk.Timestamp = p.header.Timestamp
			}

			if p.header.TypeID > 0 {
				chunk.TypeID = p.header.TypeID
			}

			if chunk.Length == 0 {
				return nil, i, fmt.Errorf("bad message. the length of an rtmp message cannot be zero")
			}

			// 计算能拷贝的有效长度
			rest := length - i
			need := chunk.Length - chunk.Size
			consume := bufio.MinInt(need, maxChunkSize-(chunk.Size%maxChunkSize))
			consume = bufio.MinInt(consume, rest)

			mediaType := utils.AVMediaTypeUnknown
			if MessageTypeIDAudio == p.header.TypeID || (ChunkStreamIdAudio == p.header.ChunkStreamID && p.header.TypeID == 0) {
				mediaType = utils.AVMediaTypeAudio
			} else if MessageTypeIDVideo == p.header.TypeID || (ChunkStreamIdVideo == p.header.ChunkStreamID && p.header.TypeID == 0) {
				mediaType = utils.AVMediaTypeVideo
			}

			if utils.AVMediaTypeUnknown != mediaType && onAvDataCb != nil {
				onAvDataCb(mediaType, data[i:i+consume], chunk.Size == 0)
			} else {
				if len(chunk.Body) < chunk.Length {
					bytes := make([]byte, chunk.Length+1024)
					copy(bytes, chunk.Body[:chunk.Size])
					chunk.Body = bytes
				}

				copy(chunk.Body[chunk.Size:], data[i:i+consume])
			}

			chunk.Size += consume
			i += consume
			if chunk.Size >= chunk.Length {
				result := *chunk
				chunk.Reset()
				p.Reset()
				return &result, i, nil
			} else if chunk.Size%maxChunkSize == 0 {
				p.state = ParserStateInit
			}
			break
		}
	}

	return nil, i, nil
}

func (p *Parser) Reset() {
	p.header = Chunk{}
	p.state = ParserStateInit
	p.headerOffset = 0
	p.extended = false
}
