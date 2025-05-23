package rtmp

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"

	bufio2 "github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/flv/amf0"
)

// 事先缓存需要使用的参数
var windowSize []byte
var chunkSize []byte
var peerBandwidth []byte

func init() {
	windowSize = make([]byte, 4)
	chunkSize = make([]byte, 4)
	peerBandwidth = make([]byte, 5)

	binary.BigEndian.PutUint32(windowSize, DefaultWindowSize)
	binary.BigEndian.PutUint32(chunkSize, MaxChunkSize)
	binary.BigEndian.PutUint32(peerBandwidth, DefaultWindowSize)
	peerBandwidth[4] = 0x2 //dynamic
}

type OnMessageHandler interface {
	OnProtocolControl() // 协议栈控制消息, 决定消息的解析和收发

	OnUserControl() // 0-StreamBegin/1-StreamEOF/2-StreamDry/3-StreamBufferLength/4-StreamIsRecorded/6-PingRequest/7-PingResponse

	OnConnectionCommand() // 使用AMF负载命令消息 20-AMF0/17-AMF3

	OnStreamCommand()

	OnDataMessage() // 发送元数据或自定义的用户数据 18-AMF0/15-AMF3

	OnSharedObjectMessage() // Flash Object(多对name-value的集合) 19-AMF0/16-AMF3

	OnAudioMessage()

	OnVideoMessage()

	OnAggregateMessage() // 聚合消息
}

type Stack struct {
	handshakeState     HandshakeState // 握手状态
	maxLocalChunkSize  int            // client chunk最大大小 默认128
	maxRemoteChunkSize int            // server chunk最大大小 默认128
	windowSize         int

	parser          Parser // chunk解析器
	metaData        *amf0.Object
	writeBuffer     []byte
	commandHandlers map[string]func(conn net.Conn, chunk *Chunk, transactionId float64, amf0Data *amf0.Data) error

	audioTimestamp uint32
	videoTimestamp uint32
}

func (s *Stack) Write(conn net.Conn, data []byte) error {
	_, err := conn.Write(data)
	return err
}

func (s *Stack) SendChunks(conn net.Conn, chunks ...Chunk) error {
	length := cap(s.writeBuffer)
	var offset int
	for _, chunk := range chunks {
		size := chunk.ComputePacketSize(s.maxLocalChunkSize)
		n := offset + size
		// 扩容
		if length < n {
			length = n * 3
			bytes := make([]byte, length)
			copy(bytes, s.writeBuffer[:offset])
			s.writeBuffer = bytes
		}

		utils.Assert(chunk.Marshal(s.writeBuffer[offset:], s.maxLocalChunkSize) == size)
		offset = n
	}

	return s.Write(conn, s.writeBuffer[:offset])
}

// Input 解析chunk, 返回chunk以及chunk的完整性
func (s *Stack) Input(conn net.Conn, data []byte, onMessage func(chunk *Chunk) error, onStream func(mediaType utils.AVMediaType, data []byte, first bool)) (int, error) {
	length, n := len(data), 0
	var err error

	for n < length {
		var chunk *Chunk
		var i int

		chunk, i, err = s.parser.ReadChunk(data[n:], s.maxRemoteChunkSize, onStream)

		n += i
		if err != nil || chunk == nil {
			break
		}

		var amf0Data *amf0.Data
		amf0Data, err = s.PreprocessChunk(chunk)
		if err != nil {
			break
		} else if MessageTypeIDCommandAMF0 == chunk.TypeID || MessageTypeIDSharedObjectAMF0 == chunk.TypeID {
			if amf0.DataTypeString != amf0Data.Get(0).Type() {
				return n, fmt.Errorf("the first element of amf0 must be of type command: %s", hex.EncodeToString(chunk.Body[:chunk.Size]))
			} else if amf0.DataTypeNumber != amf0Data.Get(1).Type() {
				return n, fmt.Errorf("the second element of amf0 must be id of transaction: %s", hex.EncodeToString(chunk.Body[:chunk.Size]))
			}

			if "@setDataFrame" == string(amf0Data.Get(0).(amf0.String)) {
				//if "onMetaData" != string(amf0Data.Get(1).(amf0.String)) {
				//	break
				//}

				if amf0.DataTypeObject == amf0Data.Get(2).Type() {
					s.metaData = amf0Data.Get(2).(*amf0.Object)
				} else if amf0.DataTypeECMAArray == amf0Data.Get(2).Type() {
					s.metaData = amf0Data.Get(2).(*amf0.ECMAArray).Object
				}
			}

			handler := s.commandHandlers[string(amf0Data.Get(0).(amf0.String))]

			if handler != nil {
				if err = handler(conn, chunk, float64(amf0Data.Get(1).(amf0.Number)), amf0Data); err != nil {
					break
				}
			}
		}

		// 回调message
		if err = onMessage(chunk); err != nil {
			break
		}
	}

	return n, err
}

// PreprocessChunk 处理控制消息和解析命令消息
func (s *Stack) PreprocessChunk(chunk *Chunk) (*amf0.Data, error) {
	var data *amf0.Data
	switch chunk.TypeID {
	case MessageTypeIDSetChunkSize:
		s.maxRemoteChunkSize = int(binary.BigEndian.Uint32(chunk.Body)) & 0x7FFFFFFF
		break
	case MessageTypeIDAbortMessage:
		// streamId := binary.BigEndian.Uint32(chunk.Body)
		// 中止对应StreamID, 正在解析的消息
		break
	case MessageTypeIDAcknowledgement:
		// 对方收到的字节数. 收到Window Size大小的字节后，向对方发送确认消息
		// recvDataSize := binary.BigEndian.Uint32(chunk.Body)
		break
	case MessageTypeIDUserControlMessage:
		reader := bufio2.NewBytesReader(chunk.Body)
		event, err := reader.ReadUint16()
		if err != nil {
			return nil, err
		}

		// stream id
		_, err = reader.ReadUint32()
		if err != nil {
			return nil, err
		}

		if UserControlMessageEventStreamBegin == event {

		} else if UserControlMessageEventStreamEOF == event {

		} else if UserControlMessageEventStreamDry == event {

		} else if UserControlMessageEventSetBufferLength == event {
			// buffer length
			_, err = reader.ReadUint32()
			if err != nil {
				return nil, err
			}

		} else if UserControlMessageEventStreamIsRecorded == event {

		} else if UserControlMessageEventPingRequest == event {
			// 发送应答消息

		} else if UserControlMessageEventPingResponse == event {

		}

		//data := chunk.Body[2:]
		break
	case MessageTypeIDWindowAcknowledgementSize:
		// 设置窗口大小, 未收到ACK确认之前，最多发送的字节数
		s.windowSize = int(binary.BigEndian.Uint32(chunk.Body))
		break
	case MessageTypeIDSetPeerBandWith:
		// 限制对方输出带宽
		// windowSize := binary.BigEndian.Uint32(chunk.Body)
		// limit type 0-hard/1-soft/2-dynamic
		// limitType := chunk.Body[4:]
		break
	case MessageTypeIDDataAMF0, MessageTypeIDCommandAMF0, MessageTypeIDSharedObjectAMF0:
		data = &amf0.Data{}
		if err := data.Unmarshal(chunk.Body[:chunk.Size]); err != nil {
			return nil, err
		} else if data.Size() < 2 {
			return nil, fmt.Errorf("")
		}
		break
	case MessageTypeIDDataAMF3:
		break
	case MessageTypeIDCommandAMF3:
		break
	case MessageTypeIDSharedObjectAMF3:
		break
	case MessageTypeIDAggregateMessage:
		break
	}

	return data, nil
}

func (s *Stack) responseStatus(conn net.Conn, transactionId float64, level, code, description string) error {
	amf0Writer := amf0.Data{}
	amf0Writer.AddString(MessageOnStatus)
	amf0Writer.AddNumber(transactionId)
	amf0Writer.Add(amf0.Null{})

	object := &amf0.Object{}
	object.AddStringProperty("level", level)
	object.AddStringProperty("code", code)
	object.AddStringProperty("description", description)

	amf0Writer.Add(object)

	var tmp [128]byte
	n, _ := amf0Writer.Marshal(tmp[:])

	response := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdSystem,
		Timestamp:     0,
		TypeID:        MessageTypeIDCommandAMF0,
		StreamID:      1,
		Length:        n,
		Body:          tmp[:n],
		Size:          n,
	}

	return s.SendChunks(conn, response)
}

func (s *Stack) Close() {
}

func (s *Stack) SendStreamBeginChunk(conn net.Conn) error {
	bytes := make([]byte, 6)
	binary.BigEndian.PutUint16(bytes, 0)
	binary.BigEndian.PutUint32(bytes[2:], 1)

	streamBegin := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdNetwork,
		Timestamp:     0,
		TypeID:        MessageTypeIDUserControlMessage,
		StreamID:      0,
		Length:        6,
		Body:          bytes,
		Size:          len(windowSize),
	}

	return s.SendChunks(conn, streamBegin)
}

func (s *Stack) SendStreamEOFChunk(conn net.Conn) error {
	bytes := make([]byte, 6)
	binary.BigEndian.PutUint16(bytes, 1)
	binary.BigEndian.PutUint32(bytes[2:], 1)

	streamEof := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdNetwork,
		Timestamp:     0,
		TypeID:        MessageTypeIDUserControlMessage,
		StreamID:      0,
		Length:        6,
		Body:          bytes,
		Size:          6,
	}

	return s.SendChunks(conn, streamEof)
}

func (s *Stack) Metadata() *amf0.Object {
	return s.metaData
}
