package rtmp

import (
	"encoding/binary"

	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
)

// https://en.wikipedia.org/wiki/Real-Time_Messaging_Protocol
// https://rtmp.veriskope.com/pdf/rtmp_specification_1.0.pdf
type ChunkType byte
type ChunkStreamID int
type MessageTypeID int
type MessageStreamID int
type UserControlMessageEvent uint16
type TransactionID int

/*
Chunk Format
Each Chunk consists of a header and Body. The header itself has
three parts:
+--------------+----------------+--------------------+--------------+
| Basic Header | Chunk Header | Extended Timestamp | Chunk Body |
+--------------+----------------+--------------------+--------------+
| |
|<------------------- Chunk Header ----------------->|
*/

/**
type 0
0 1 2 3
0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| Timestamp |message length |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| message length (cont) |message type id| msg stream id |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| message stream id (cont) |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

const (
	ChunkType0 = ChunkType(0x00)
	ChunkType1 = ChunkType(0x01) //The message stream ID is not included
	ChunkType2 = ChunkType(0x02) //Neither the stream ID nor the message length is included;
	ChunkType3 = ChunkType(0x03) //basic header

	ChunkStreamIdNetwork = ChunkStreamID(2)
	ChunkStreamIdSystem  = ChunkStreamID(3)
	ChunkStreamIdAudio   = ChunkStreamID(4)
	ChunkStreamIdVideo   = ChunkStreamID(6)
	ChunkStreamIdSource  = ChunkStreamID(8)

	MessageTypeIDSetChunkSize              = MessageTypeID(1)
	MessageTypeIDAbortMessage              = MessageTypeID(2)
	MessageTypeIDAcknowledgement           = MessageTypeID(3)
	MessageTypeIDUserControlMessage        = MessageTypeID(4)
	MessageTypeIDWindowAcknowledgementSize = MessageTypeID(5)
	MessageTypeIDSetPeerBandWith           = MessageTypeID(6)

	MessageTypeIDAudio                      = MessageTypeID(8)
	MessageTypeIDVideo                      = MessageTypeID(9)
	MessageTypeIDDataAMF0                   = MessageTypeID(18) // MessageTypeIDDataAMF0 MessageTypeIDDataAMF3 metadata:creation time, duration, theme...
	MessageTypeIDDataAMF3                   = MessageTypeID(15)
	MessageTypeIDCommandAMF0                = MessageTypeID(20) // MessageTypeIDCommandAMF0 MessageTypeIDCommandAMF3  connect, createStream, publish, play, pause
	MessageTypeIDCommandAMF3                = MessageTypeID(17)
	MessageTypeIDSharedObjectAMF0           = MessageTypeID(19)
	MessageTypeIDSharedObjectAMF3           = MessageTypeID(16)
	MessageTypeIDAggregateMessage           = MessageTypeID(22)
	UserControlMessageEventStreamBegin      = 0x00 // server告知client开始处理流
	UserControlMessageEventStreamEOF        = 0x01
	UserControlMessageEventStreamDry        = 0x02
	UserControlMessageEventSetBufferLength  = 0x03 // server告知client, 缓冲区长度
	UserControlMessageEventStreamIsRecorded = 0x04 // server告知client该StreamID的流是录制流
	UserControlMessageEventPingRequest      = 0x06 // server发送给client
	UserControlMessageEventPingResponse     = 0x07 // client应答给server

	TransactionIDConnect      = TransactionID(1)
	TransactionIDCreateStream = TransactionID(2)
	TransactionIDPlay         = TransactionID(0)
	DefaultChunkSize          = 128
	MaxChunkSize              = 60000
	DefaultWindowSize         = 2500000

	MessageResult        = "_result"
	MessageError         = "_error"
	MessageConnect       = "connect"
	MessageCall          = "call"
	MessageClose         = "close"
	MessageFcPublish     = "FCPublish"
	MessageReleaseStream = "releaseStream"
	MessageCreateStream  = "createStream"
	MessageStreamLength  = "getStreamLength"
	MessagePublish       = "publish"
	MessagePlay          = "play"
	MessagePlay2         = "play2"
	MessageDeleteStream  = "deleteStream"
	MessageReceiveAudio  = "receiveAudio"
	MessageReceiveVideo  = "receiveVideo"
	MessageSeek          = "seek"
	MessagePause         = "pause"
	MessageOnStatus      = "onStatus"
	MessageOnMetaData    = "onMetaData"
)

type Chunk struct {
	// basic header
	Type          ChunkType     // fmt 1-3bytes.低6位等于0,2字节;低6位等于1,3字节
	ChunkStreamID ChunkStreamID // currentChunk stream id. customized by users

	Timestamp uint32        // type0-绝对时间戳/type1和type2-与上一个chunk的差值
	Length    int           // message length
	TypeID    MessageTypeID // message type id
	StreamID  uint32        // message stream id. customized by users. LittleEndian

	Body []byte // 消息体
	Size int    // 消息体大小
}

func (h *Chunk) MarshalHeader(dst []byte) int {
	var n int
	n++

	dst[0] = byte(h.Type) << 6
	if h.ChunkStreamID <= 63 {
		dst[0] = dst[0] | byte(h.ChunkStreamID)
	} else if h.ChunkStreamID <= 0xFF {
		dst[0] = dst[0] & 0xC0
		dst[1] = byte(h.ChunkStreamID)
		n++
	} else if h.ChunkStreamID <= 0xFFFF {
		dst[0] = dst[0] & 0xC0
		dst[0] = dst[0] | 0x1
		binary.BigEndian.PutUint16(dst[1:], uint16(h.ChunkStreamID))
		n += 2
	}

	if h.Type < ChunkType3 {
		if h.Timestamp >= 0xFFFFFF {
			bufio.PutUint24(dst[n:], 0xFFFFFF)
		} else {
			bufio.PutUint24(dst[n:], h.Timestamp)
		}
		n += 3
	}

	if h.Type < ChunkType2 {
		bufio.PutUint24(dst[n:], uint32(h.Length))
		n += 4
		dst[n-1] = byte(h.TypeID)
	}

	if h.Type < ChunkType1 {
		binary.LittleEndian.PutUint32(dst[n:], h.StreamID)
		n += 4
	}

	if h.Timestamp >= 0xFFFFFF {
		binary.BigEndian.PutUint32(dst[n:], h.Timestamp)
		n += 4
	}

	return n
}

func (h *Chunk) Marshal(dst []byte, maxChunkSize int) int {
	utils.Assert(len(h.Body) >= h.Length)
	n := h.MarshalHeader(dst)
	n += h.WriteBody(dst[n:], h.Body[:h.Length], maxChunkSize, 0)
	return n
}

func (h *Chunk) WriteBody(dst, data []byte, maxChunkSize, offset int) int {
	utils.Assert(maxChunkSize > 0)
	// utils.Assert(h.Length == len(data))

	var n int
	var payloadSize int
	for length := len(data); length > 0; length -= payloadSize {
		// 写一个ChunkType3用作分割
		if n > 0 {
			dst[n] = (0x3 << 6) | byte(h.ChunkStreamID)
			n++

			if h.Timestamp >= 0xFFFFFF {
				binary.BigEndian.PutUint32(dst[n:], h.Timestamp)
				n += 4
			}
		}

		payloadSize = bufio.MinInt(length, maxChunkSize-offset)
		offset = 0
		copy(dst[n:], data[:payloadSize])
		n += payloadSize
		data = data[payloadSize:]
	}

	return n
}

func (h *Chunk) Reset() {
	//h.ChunkStreamID = 0
	// 如果当前包没有携带timestamp字段, 默认和前一包一致
	//h.Timestamp = 0
	// 如果当前包没有携带length字段, 默认和前一包一致
	//h.Length = 0
	// 如果当前包没有携带tid字段, 默认和前一包一致
	//h.TypeID = 0
	h.StreamID = 0
	h.Size = 0
}

func (h *Chunk) ComputePacketSize(maxChunkSize int) int {
	var index int
	index++

	if h.ChunkStreamID <= 63 {
	} else if h.ChunkStreamID <= 0xFF {
		index++
	} else if h.ChunkStreamID <= 0xFFFF {
		index += 2
	}

	if h.Type < ChunkType3 {
		index += 3
	}

	if h.Type < ChunkType2 {
		index += 4
	}

	if h.Type < ChunkType1 {
		index += 4
	}

	if h.Timestamp >= 0xFFFFFF {
		index += 4
	}

	var n int
	var payloadSize int
	for length := h.Length; length > 0; length -= payloadSize {
		// 写一个ChunkType3用作分割
		if n > 0 {
			n++
			if h.Timestamp >= 0xFFFFFF {
				n += 4
			}
		}

		payloadSize = bufio.MinInt(length, maxChunkSize)
		n += payloadSize
	}

	return index + n
}

func NewAudioChunk() Chunk {
	return Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdAudio,
		TypeID:        MessageTypeIDAudio,
		StreamID:      1,
	}
}

func NewVideoChunk() Chunk {
	return Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdVideo,
		TypeID:        MessageTypeIDVideo,
		StreamID:      1,
	}
}
