package rtmp

import (
	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/flv"
	"github.com/devpospicha/media-server/flv/amf0"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/rtmp"
)

type transStream struct {
	stream.TCPTransStream
	sequenceHeader []byte // sequence sequenceHeader

	muxer      *flv.Muxer
	audioChunk rtmp.Chunk
	videoChunk rtmp.Chunk
	chunkSize  int
	metaData   *amf0.Object // Metadata carried by the stream pusher
}

func (t *transStream) Input(packet *avformat.AVPacket) ([]*collections.ReferenceCounter[[]byte], int64, bool, error) {
	t.ClearOutStreamBuffer()
	//log.Printf(" Input = %s", packet)
	var data []byte
	var chunk *rtmp.Chunk
	var videoPkt bool
	var videoKey bool
	// chunk payload message body data size
	var payloadSize int
	// flv data header
	var dataHeaderSize int
	var dts int64
	var pts int64
	var keyBuffer bool
	var frameType int

	dts = packet.ConvertDts(1000)
	pts = packet.ConvertPts(1000)
	ct := pts - dts

	// chunk = header+payload(audio data / video data)
	if utils.AVMediaTypeAudio == packet.MediaType {
		data = packet.Data
		chunk = &t.audioChunk
		dataHeaderSize = t.muxer.ComputeAudioDataHeaderSize()
	} else if utils.AVMediaTypeVideo == packet.MediaType {
		videoPkt = true
		if videoKey = packet.Key; videoKey {
			frameType = flv.FrameTypeKeyFrame
		}

		data = avformat.AnnexBPacket2AVCC(packet)
		chunk = &t.videoChunk
		dataHeaderSize = t.muxer.ComputeVideoDataHeaderSize(uint32(ct))
	}

	payloadSize += dataHeaderSize + len(data)

	// 遇到视频关键帧, 发送剩余的流, 创建新切片
	if videoKey && !t.MWBuffer.IsNewSegment() && t.MWBuffer.HasVideoDataInCurrentSegment() {
		segment, key := t.MWBuffer.FlushSegment()
		t.AppendOutStreamBuffer(segment)
		keyBuffer = key
	}

	// type为0的header大小
	chunkHeaderSize := 12
	// type为3的chunk数量
	numChunks := (payloadSize - 1) / t.chunkSize
	// 整个chunk大小
	totalSize := chunkHeaderSize + payloadSize + numChunks
	// 如果时间戳超过3字节, 每个chunk都需要多4字节的扩展时间戳
	if dts >= 0xFFFFFF {
		totalSize += (1 + numChunks) * 4
	}

	// Allocate memory of the specified size
	bytes, ok := t.MWBuffer.TryAlloc(totalSize, dts, videoPkt, videoKey)
	if !ok {
		segment, key := t.MWBuffer.FlushSegment()
		if !keyBuffer {
			keyBuffer = key
		}

		t.AppendOutStreamBuffer(segment)
		bytes, ok = t.MWBuffer.TryAlloc(totalSize, dts, videoPkt, videoKey)
		utils.Assert(ok)
	}

	// 写第一个type为0的chunk
	chunk.Length = payloadSize
	chunk.Timestamp = uint32(dts)
	n := chunk.MarshalHeader(bytes)

	// 封装成flv
	if videoPkt {
		n += t.muxer.VideoData.Marshal(bytes[n:], uint32(ct), frameType, false)
	} else {
		n += t.muxer.AudioData.Marshal(bytes[n:], false)
	}

	// 将flv data写入chunk body
	n += chunk.WriteBody(bytes[n:], data, t.chunkSize, dataHeaderSize)
	utils.Assert(len(bytes) == n)

	// 合并写满了再发
	if segment, key := t.MWBuffer.TryFlushSegment(); segment != nil {
		keyBuffer = key
		t.AppendOutStreamBuffer(segment)
	}

	return t.OutBuffer[:t.OutBufferSize], 0, keyBuffer, nil
}

func (t *transStream) ReadExtraData(_ int64) ([]*collections.ReferenceCounter[[]byte], int64, error) {
	utils.Assert(len(t.sequenceHeader) > 0)

	// 发送sequence sequenceHeader
	return []*collections.ReferenceCounter[[]byte]{collections.NewReferenceCounter(t.sequenceHeader)}, 0, nil
}

func (t *transStream) ReadKeyFrameBuffer() ([]*collections.ReferenceCounter[[]byte], int64, error) {
	t.ClearOutStreamBuffer()

	// 发送当前内存池已有的合并写切片
	t.MWBuffer.ReadSegmentsFromKeyFrameIndex(func(bytes *collections.ReferenceCounter[[]byte]) {
		t.AppendOutStreamBuffer(bytes)
	})

	return t.OutBuffer[:t.OutBufferSize], 0, nil
}

func (t *transStream) WriteHeader() error {
	utils.Assert(t.Tracks != nil)
	utils.Assert(!t.BaseTransStream.Completed)

	var audioStream *avformat.AVStream
	var videoStream *avformat.AVStream
	var audioCodecId utils.AVCodecID
	var videoCodecId utils.AVCodecID

	for _, track := range t.Tracks {
		if utils.AVMediaTypeAudio == track.Stream.MediaType {
			audioStream = track.Stream
			audioCodecId = audioStream.CodecID
			t.audioChunk = rtmp.NewAudioChunk()
		} else if utils.AVMediaTypeVideo == track.Stream.MediaType {
			videoStream = track.Stream
			videoCodecId = videoStream.CodecID
			t.videoChunk = rtmp.NewVideoChunk()
		}
	}

	utils.Assert(audioStream != nil || videoStream != nil)

	// 初始化
	t.BaseTransStream.Completed = true
	t.muxer = flv.NewMuxer(t.metaData)
	if utils.AVCodecIdNONE != audioCodecId {
		t.muxer.AddAudioTrack(audioStream)
	}

	if utils.AVCodecIdNONE != videoCodecId {
		t.muxer.AddVideoTrack(videoStream)
	}

	var n int
	t.sequenceHeader = make([]byte, 4096)

	// 生成数据头
	if audioStream != nil {
		n += t.muxer.AudioData.Marshal(t.sequenceHeader[12:], true)
		extra := audioStream.Data
		copy(t.sequenceHeader[n+12:], extra)
		n += len(extra)

		t.audioChunk.Length = n
		t.audioChunk.MarshalHeader(t.sequenceHeader)
		n += 12
	}

	if videoStream != nil {
		tmp := n
		n += t.muxer.VideoData.Marshal(t.sequenceHeader[n+12:], 0, 0, true)
		extra := videoStream.CodecParameters.MP4ExtraData()
		copy(t.sequenceHeader[n+12:], extra)
		n += len(extra)

		t.videoChunk.Length = 5 + len(extra)
		t.videoChunk.MarshalHeader(t.sequenceHeader[tmp:])
		n += 12
	}

	// 创建元数据chunk
	var body [1024]byte
	metadata := amf0.Data{}
	metadata.AddString("onMetaData")
	metadata.Add(t.muxer.MetaData())
	length, _ := metadata.Marshal(body[:])

	metaDataChunk := rtmp.Chunk{
		Type:          rtmp.ChunkType0,
		ChunkStreamID: 5,
		Timestamp:     0,
		TypeID:        rtmp.MessageTypeIDDataAMF0,
		StreamID:      1,
		Body:          body[:length],
		Length:        length,
	}

	var tmp [1600]byte
	size := metaDataChunk.Marshal(tmp[:], rtmp.MaxChunkSize)
	// metadata放在sequence之前
	copy(t.sequenceHeader[size:], t.sequenceHeader[:n])
	copy(t.sequenceHeader, tmp[:][:size])

	n += size
	t.sequenceHeader = t.sequenceHeader[:n]
	t.MWBuffer = stream.NewMergeWritingBuffer(t.ExistVideo)
	return nil
}

func (t *transStream) Close() ([]*collections.ReferenceCounter[[]byte], int64, error) {
	t.ClearOutStreamBuffer()

	// 发送剩余的流
	if segment, _ := t.MWBuffer.FlushSegment(); segment != nil {
		t.AppendOutStreamBuffer(segment)
	}

	return t.OutBuffer[:t.OutBufferSize], 0, nil
}

func NewTransStream(chunkSize int, metaData *amf0.Object) stream.TransStream {
	return &transStream{chunkSize: chunkSize, metaData: metaData}
}

func TransStreamFactory(source stream.Source, protocol stream.TransStreamProtocol, tracks []*stream.Track) (stream.TransStream, error) {
	// 获取推流的元数据
	var metaData *amf0.Object
	if stream.SourceTypeRtmp == source.GetType() {
		metaData = source.(*Publisher).Stack.Metadata()
	}
	return NewTransStream(rtmp.MaxChunkSize, metaData), nil
}
