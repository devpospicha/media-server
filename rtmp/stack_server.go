package rtmp

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/flv"
	"github.com/devpospicha/media-server/flv/amf0"
)

type OnStreamHandler interface {
	OnPublish(app, stream string) utils.HookState

	OnPlay(app, stream string) utils.HookState
}

// ServerStack 处理推拉流的交互消息
type ServerStack struct {
	Stack
	FLV               *flv.Demuxer
	handshakeData     []byte
	handshakeDataSize int

	handler      OnStreamHandler
	app          string
	stream       string
	playStreamID uint32

	receiveDataSize      uint32 // 统计接受数据长度
	totalReceiveDataSize uint32
}

func (s *ServerStack) DoHandshake(conn net.Conn) error {
	if HandshakeStateUninitialized == s.handshakeState && s.handshakeDataSize > 0 {
		if s.handshakeData[0] < VERSION {
			fmt.Printf("unkonw rtmp version: %d\r\n", s.handshakeData[0])
		}

		s.handshakeState = HandshakeStateVersionSent
	}

	if HandshakeStateVersionSent == s.handshakeState && s.handshakeDataSize >= 1+HandshakePacketSize {

		c1 := s.handshakeData[1 : 1+HandshakePacketSize]

		// time
		_ = binary.BigEndian.Uint32(c1)
		// zero
		_ = binary.BigEndian.Uint32(c1[4:])

		// 收到c0c1,应答s0s1s2
		s0s1s2 := make([]byte, TotalHandshakePacketSize)

		// random bytes
		n := GenerateS0S1S2(s0s1s2, c1)
		utils.Assert(n == len(s0s1s2))

		_, err := conn.Write(s0s1s2)
		if err != nil {
			return err
		}

		s.handshakeState = HandshakeStateAckSent
	}

	if HandshakeStateAckSent == s.handshakeState && TotalHandshakePacketSize == s.handshakeDataSize {
		s.handshakeState = HandshakeStateDone
	}

	return nil
}

func (s *ServerStack) Input(conn net.Conn, data []byte) error {
	length, n := len(data), 0
	// Count the total length of the received streaming data and notify the client at a fixed frequency period
	// If the client is not notified, when the length of the streaming data reaches WindowSize, Adobe Flash Media Live Encoder will stop streaming
	s.receiveDataSize += uint32(length)
	s.totalReceiveDataSize += uint32(length)
	if s.receiveDataSize > DefaultWindowSize/2 {
		bytes := [4]byte{}
		binary.BigEndian.PutUint32(bytes[:], s.totalReceiveDataSize)
		acknowledgement := Chunk{
			Type:          ChunkType0,
			ChunkStreamID: ChunkStreamIdNetwork,
			Timestamp:     0,
			TypeID:        MessageTypeIDAcknowledgement,
			StreamID:      0,
			Length:        4,

			Body: bytes[:],
			Size: 4,
		}

		_ = s.SendChunks(conn, acknowledgement)
		s.receiveDataSize = 0
	}

	// 握手处理
	if HandshakeStateDone != s.handshakeState {
		if need := TotalHandshakePacketSize - s.handshakeDataSize; need > 0 {
			n = bufio.MinInt(need, length)
			copy(s.handshakeData, data[:n])
			s.handshakeDataSize += n
		}

		err := s.DoHandshake(conn)
		if err != nil {
			return err
		} else if n == length {
			return nil
		} else if HandshakeStateDone != s.handshakeState {
			panic("handshake failed: expected handshake state to be done, but it is not")
		}
	}

	_, err := s.Stack.Input(conn, data[n:], func(chunk *Chunk) error {
		return s.ProcessMessage(chunk)
	},
		func(mediaType utils.AVMediaType, data []byte, first bool) {
			s.FLV.BaseDemuxer.DataPipeline.Write(data, s.FLV.BaseDemuxer.FindBufferIndexByMediaType(mediaType), mediaType)
		},
	)

	return err
}

func (s *ServerStack) ProcessMessage(chunk *Chunk) error {
	switch chunk.TypeID {
	case MessageTypeIDAudio:
		if ChunkType0 == chunk.Type {
			s.audioTimestamp = chunk.Timestamp
		} else {
			s.audioTimestamp += chunk.Timestamp
		}

		index := s.FLV.FindBufferIndexByMediaType(utils.AVMediaTypeAudio)
		audioData, _ := s.FLV.DataPipeline.Feat(index)

		discard, err := s.FLV.ProcessAudioData(audioData, s.audioTimestamp)
		if discard || err != nil {
			s.FLV.DataPipeline.DiscardBackPacket(index)
		}

		return err
	case MessageTypeIDVideo:
		// type0是绝对时间戳, 其余是相对时间戳
		if ChunkType0 == chunk.Type {
			s.videoTimestamp = chunk.Timestamp
		} else {
			s.videoTimestamp += chunk.Timestamp
		}

		index := s.FLV.FindBufferIndexByMediaType(utils.AVMediaTypeVideo)
		videoData, err := s.FLV.DataPipeline.Feat(index)
		if err != nil {
			return err
		}

		discard, err := s.FLV.ProcessVideoData(videoData, s.videoTimestamp)
		if discard || err != nil {
			s.FLV.DataPipeline.DiscardBackPacket(index)
		}
		return err
	}

	return nil
}

func (s *ServerStack) OnConnect(conn net.Conn, chunk *Chunk, transactionId float64, data *amf0.Data) error {

	//	|              Handshaking done               |
	//	|                     |                       |
	//	|                     |                       |
	//	|                     |                       |
	//	|                     |                       |
	//	|----------- Command Message(connect) ------->|
	//	|                                             |
	//	|<------- Window Acknowledgement Size --------|
	//	|                                             |
	//	|<----------- Set Peer Bandwidth -------------|
	//	|                                             |
	//	|-------- Window Acknowledgement Size ------->|
	//	|                                             |
	//	|<------ User Control Message(StreamBegin) ---|
	//	|                                             |
	//	|<------------ Command Message ---------------|
	//	|       (_result- connect response)           |
	//	|                                             |

	//		The command structure from server to client is as follows:
	//	+--------------+----------+----------------------------------------+
	//	| Field Name   |   Type   |             Description                |
	//		+--------------+----------+----------------------------------------+
	//	| Command Name |  String  | _result or _error; indicates whether   |
	//	|              |          | the response is result or error.       |
	//	+--------------+----------+----------------------------------------+
	//	| Transaction  |  Number  | Transaction ID is 1 for connect        |
	//	| ID           |          | responses                              |
	//	|              |          |                                        |
	//	+--------------+----------+----------------------------------------+
	//	| Properties   |  Object  | Name-value pairs that describe the     |
	//	|              |          | properties(fmsver etc.) of the         |
	//	|              |          | connection.                            |
	//	+--------------+----------+----------------------------------------+
	//	| Information  |  Object  | Name-value pairs that describe the     |
	//	|              |          | response from|the server. ’code’,      |
	//	|              |          | ’level’, ’description’ are names of few|
	//	|              |          | among such information.                |
	//	+--------------+----------+----------------------------------------+

	//	window Size
	//	chunk Size
	//	bandwidth
	acknow := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdNetwork,
		Timestamp:     0,
		TypeID:        MessageTypeIDWindowAcknowledgementSize,
		StreamID:      0,
		Length:        len(windowSize),

		Body: windowSize,
		Size: len(windowSize),
	}

	bandwidth := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdNetwork,
		Timestamp:     0,
		TypeID:        MessageTypeIDSetPeerBandWith,
		StreamID:      0,
		Length:        len(peerBandwidth),

		Body: peerBandwidth,
		Size: len(peerBandwidth),
	}

	setChunkSize := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdNetwork,
		Timestamp:     0,
		TypeID:        MessageTypeIDSetChunkSize,
		StreamID:      0,
		Length:        len(chunkSize),

		Body: chunkSize,
		Size: len(chunkSize),
	}

	var tmp [512]byte
	writer := amf0.Data{}
	writer.AddString(MessageResult)
	//always equal to 1 for the connect command
	writer.AddNumber(transactionId)
	//https://en.wikipedia.org/wiki/Real-Time_Messaging_Protocol#Connect
	properties := &amf0.Object{}
	properties.AddStringProperty("fmsVer", "FMS/3,5,5,2004")
	properties.AddNumberProperty("capabilities", 31)
	properties.AddNumberProperty("mode", 1)

	information := &amf0.Object{}
	information.AddStringProperty("level", "status")
	information.AddStringProperty("code", "NetConnection.Connect.Success")
	information.AddStringProperty("description", "Connection succeeded")
	information.AddNumberProperty("clientId", 0)
	information.AddNumberProperty("objectEncoding", 3.0)

	writer.Add(properties)
	writer.Add(information)
	n, _ := writer.Marshal(tmp[:])

	result := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdSystem,
		Timestamp:     0,
		TypeID:        MessageTypeIDCommandAMF0,
		StreamID:      0,
		Length:        n,

		Body: tmp[:n],
		Size: n,
	}

	if data.Size() > 2 && amf0.DataTypeObject == data.Get(2).Type() {
		object := data.Get(2).(*amf0.Object)
		if property := object.FindProperty("app"); property != nil && amf0.DataTypeString == property.Value.Type() {
			s.app = string(property.Value.(amf0.String))
		}
	}

	err := s.SendChunks(conn, acknow, bandwidth, result, setChunkSize)
	s.maxLocalChunkSize = MaxChunkSize
	return err
}

func (s *ServerStack) OnCreateStream(conn net.Conn, chunk *Chunk, transactionId float64, data *amf0.Data) error {

	//		The command structure from server to client is as follows:
	//	+--------------+----------+----------------------------------------+
	//	| Field Name   |   Type   |             Description                |
	//		+--------------+----------+----------------------------------------+
	//	| Command Name |  String  | _result or _error; indicates whether   |
	//	|              |          | the response is result or error.       |
	//	+--------------+----------+----------------------------------------+
	//	| Transaction  |  Number  | ID of the command that response belongs|
	//	| ID           |          | to.                                    |
	//	+--------------+----------+----------------------------------------+
	//	| Command      |  Object  | If there exists any command info this  |
	//	| Object       |          | is set, else this is set to null type. |
	//	+--------------+----------+----------------------------------------+
	//	| Stream       |  Number  | The return value is either a stream ID |
	//	| ID           |          | or an error information object.        |
	//	+--------------+----------+----------------------------------------+

	writer := amf0.Data{}
	writer.AddString(MessageResult)
	writer.AddNumber(transactionId)
	writer.Add(amf0.Null{})
	writer.AddNumber(1) // 应答0, 某些推流端可能会失败

	var tmp [128]byte
	n, _ := writer.Marshal(tmp[:])

	response := Chunk{
		Type:          ChunkType0,
		ChunkStreamID: ChunkStreamIdSystem,
		Timestamp:     0,
		TypeID:        MessageTypeIDCommandAMF0,
		StreamID:      0,
		Length:        n,

		Body: tmp[:n],
		Size: n,
	}

	return s.SendChunks(conn, response)
}

func (s *ServerStack) OnPublish(conn net.Conn, chunk *Chunk, transactionId float64, data *amf0.Data) error {
	//  The command structure from the client to the server is as follows:
	// +--------------+----------+----------------------------------------+
	// | Field Name   |   Type   |             Description                |
	// 	+--------------+----------+----------------------------------------+
	// | Command Name |  String  | Name of the command, set to "publish". |
	// +--------------+----------+----------------------------------------+
	// | Transaction  |  Number  | Transaction ID set to 0.               |
	// | ID           |          |                                        |
	// +--------------+----------+----------------------------------------+
	// | Command      |  Null    | Command information object does not    |
	// | Object       |          | exist. Set to null type.               |
	// +--------------+----------+----------------------------------------+
	// | Publishing   |  String  | Name with which the stream is          |
	// | Name         |          | published.                             |
	// +--------------+----------+----------------------------------------+
	// | Publishing   |  String  | Type of publishing. Set to "live",     |
	// | Type         |          | "record", or "append".                 |
	// |              |          | record: The stream is published and the|
	// |              |          | data is recorded to a new file.The file|
	// |              |          | is stored on the server in a           |
	// |              |          | subdirectory within the directory that |
	// |              |          | contains the server application. If the|
	// |              |          | file already exists, it is overwritten.|
	// |              |          | append: The stream is published and the|
	// |              |          | data is appended to a file. If no file |
	// |              |          | is found, it is created.               |
	// |              |          | live: Live data is published without   |
	// |              |          | recording it in a file.                |
	// +--------------+----------+----------------------------------------+
	// stream
	if data.Size() > 3 {
		if stream, ok := data.Get(3).(amf0.String); ok {
			s.stream = string(stream)
		}
	}

	state := s.handler.OnPublish(s.app, s.stream)
	if utils.HookStateOK == state {
		return s.responseStatus(conn, transactionId, "status", "NetStream.Publish.Start", "Start publishing")
	} else if utils.HookStateOccupy == state {
		_ = s.responseStatus(conn, transactionId, "error", "NetStream.Publish.BadName", "Already publishing")
		return fmt.Errorf("stream source already exists")
	} else {
		return fmt.Errorf("hook execution failed")
	}
}

func (s *ServerStack) OnPlay(conn net.Conn, chunk *Chunk, transactionId float64, data *amf0.Data) error {
	// The command structure from the client to the server is as follows:
	// +--------------+----------+-----------------------------------------+
	// | Field Name   |   Type   |             Description                 |
	// 	+--------------+----------+-----------------------------------------+
	// | Command Name |  String  | Name of the command. Set to "play".     |
	// +--------------+----------+-----------------------------------------+
	// | Transaction  |  Number  | Transaction ID set to 0.                |
	// | ID           |          |                                         |
	// +--------------+----------+-----------------------------------------+
	// | Command      |   Null   | Command information does not exist.     |
	// | Object       |          | Set to null type.                       |
	// +--------------+----------+-----------------------------------------+
	// | Stream Name  |  String  | Name of the stream to play.             |
	// |              |          | To play video (FLV) files, specify the  |
	// |              |          | name of the stream without a file       |
	// |              |          | extension (for example, "sample"). To   |
	// |              |          | play back MP3 or ID3 tags, you must     |
	// |              |          | precede the stream name with mp3:       |
	// |              |          | (for example, "mp3:sample". To play     |
	// |              |          | H.264/AAC files, you must precede the   |
	// |              |          | stream name with mp4: and specify the   |
	// |              |          | file extension. For example, to play the|
	// |              |          | file sample.m4v,specify "mp4:sample.m4v"|
	// |              |          |                                         |
	// +--------------+----------+-----------------------------------------+
	// | Start        |  Number  | An optional parameter that specifies    |
	// |              |          | the start time in seconds. The default  |
	// |              |          | value is -2, which means the subscriber |
	// |              |          | first tries to play the live stream     |
	// |              |          | specified in the Stream Name field. If a|
	// |              |          | live stream of that name is not found,it|
	// |              |          | plays the recorded stream of the same   |
	// |              |          | name. If there is no recorded stream    |
	// |              |          | with that name, the subscriber waits for|
	// |              |          | a new live stream with that name and    |
	// |              |          | plays it when available. If you pass -1 |
	// |              |          | in the Start field, only the live stream|
	// |              |          | specified in the Stream Name field is   |
	// |              |          | played. If you pass 0 or a positive     |
	// |              |          | number in the Start field, a recorded   |
	// |              |          | stream specified in the Stream Name     |
	// |              |          | field is played beginning from the time |
	// |              |          | specified in the Start field. If no     |
	// |              |          | recorded stream is found, the next item |
	// |              |          | in the playlist is played.              |
	// +--------------+----------+-----------------------------------------+
	// | Duration     |  Number  | An optional parameter that specifies the|
	// |              |          | duration of playback in seconds. The    |
	// |              |          | default value is -1. The -1 value means |
	// |              |          | a live stream is played until it is no  |
	// |              |          | longer available or a recorded stream is|
	// |              |          | played until it ends. If you pass 0, it |
	// |              |          | plays the single frame since the time   |
	// |              |          | specified in the Start field from the   |
	// |              |          | beginning of a recorded stream. It is   |
	// |              |          | assumed that the value specified in     |
	// |              |          | the Start field is equal to or greater  |
	// |              |          | than 0. If you pass a positive number,  |
	// |              |          | it plays a live stream for              |
	// |              |          | the time period specified in the        |
	// |              |          | Duration field. After that it becomes   |
	// |              |          | available or plays a recorded stream    |
	// |              |          | for the time specified in the Duration  |
	// |              |          | field. (If a stream ends before the     |
	// |              |          | time specified in the Duration field,   |
	// |              |          | playback ends when the stream ends.)    |
	// |              |          | If you pass a negative number other     |
	// |              |          | than -1 in the Duration field, it       |
	// |              |          | interprets the value as if it were -1.  |
	// +--------------+----------+-----------------------------------------+
	// | Reset        | Boolean  | An optional Boolean value or number     |
	// |              |          | that specifies whether to flush any     |
	// |              |          | previous playlist.                      |
	// +--------------+----------+-----------------------------------------+

	if data.Size() > 3 {
		if stream, ok := data.Get(3).(amf0.String); ok {
			s.stream = string(stream)
		}
	}

	s.playStreamID = chunk.StreamID
	state := s.handler.OnPlay(s.app, s.stream)

	if utils.HookStateOK == state {
		return s.responseStatus(conn, transactionId, "status", "NetStream.Play.Start", "Start live")
	} else {
		return fmt.Errorf("hook execution failed")
	}
}

func (s *ServerStack) SetOnStreamHandler(handler OnStreamHandler) {
	s.handler = handler
}

func (s *ServerStack) Close() {
	s.handler = nil
}

func NewStackServer(autoFree bool) *ServerStack {
	stack := &ServerStack{
		FLV:           flv.NewDemuxer(autoFree),
		handshakeData: make([]byte, TotalHandshakePacketSize),
		Stack: Stack{
			maxLocalChunkSize:  DefaultChunkSize,
			maxRemoteChunkSize: DefaultChunkSize,
		},
	}

	stack.Stack.commandHandlers = map[string]func(conn net.Conn, chunk *Chunk, transactionId float64, amf0Data *amf0.Data) error{
		MessageConnect:      stack.OnConnect,
		MessageCreateStream: stack.OnCreateStream,
		MessagePublish:      stack.OnPublish,
		MessagePlay:         stack.OnPlay,
	}

	return stack
}
