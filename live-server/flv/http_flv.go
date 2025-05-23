package flv

import (
	"encoding/binary"
	"log"

	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/flv"
	"github.com/devpospicha/media-server/flv/amf0"
	rtmp "github.com/devpospicha/media-server/live-server/rtmp"
	"github.com/devpospicha/media-server/live-server/stream"
)

type TransStream struct {
	stream.TCPTransStream

	Muxer                  *flv.Muxer
	flvHeaderBlock         []byte // Save the 9-byte flv header separately, send it only once, and do not send it again when resuming streaming
	flvExtraDataBlock      []byte // metadata and sequence header
	flvExtraDataPreTagSize uint32
}

func (t *TransStream) Input(packet *avformat.AVPacket) ([]*collections.ReferenceCounter[[]byte], int64, bool, error) {
	t.ClearOutStreamBuffer()
	//	log.Print(" http_flv.go -> Input")
	var flvTagSize int
	var data []byte
	var videoKey bool
	var dts int64
	var pts int64
	var keyBuffer bool
	var frameType int

	dts = packet.ConvertDts(1000)
	pts = packet.ConvertPts(1000)
	if utils.AVMediaTypeAudio == packet.MediaType {
		data = packet.Data
		flvTagSize = flv.TagHeaderSize + t.Muxer.ComputeAudioDataHeaderSize() + len(packet.Data)
	} else if utils.AVMediaTypeVideo == packet.MediaType {
		data = avformat.AnnexBPacket2AVCC(packet)
		flvTagSize = flv.TagHeaderSize + t.Muxer.ComputeVideoDataHeaderSize(uint32(pts-dts)) + len(data)
		if videoKey = packet.Key; videoKey {
			frameType = flv.FrameTypeKeyFrame
		}
	}

	// Keyframes are placed at the head of the slice, so create a new slice when a keyframe is encountered
	if videoKey && !t.MWBuffer.IsNewSegment() && t.MWBuffer.HasVideoDataInCurrentSegment() {
		segment, key := t.flushSegment()
		t.AppendOutStreamBuffer(segment)
		keyBuffer = key
	}

	var n int
	var separatorSize int

	// New merge write slice, reserved packet length bytes
	if t.MWBuffer.IsNewSegment() {
		separatorSize = HttpFlvBlockHeaderSize
		// 10 bytes describe the flv packet length, the first 2 bytes describe the invalid byte length
		n = HttpFlvBlockHeaderSize
	} else if t.MWBuffer.ShouldFlush(dts) {
		// End of slice, reserve newline character
		separatorSize += 2
	}

	// Allocate memory of the specified size
	bytes, ok := t.MWBuffer.TryAlloc(separatorSize+flvTagSize, dts, utils.AVMediaTypeVideo == packet.MediaType, videoKey)
	if !ok {
		segment, key := t.flushSegment()
		t.AppendOutStreamBuffer(segment)

		if !keyBuffer {
			keyBuffer = key
		}
		bytes, ok = t.MWBuffer.TryAlloc(HttpFlvBlockHeaderSize+flvTagSize, dts, utils.AVMediaTypeVideo == packet.MediaType, videoKey)
		n = HttpFlvBlockHeaderSize
		utils.Assert(ok)
	}

	// 写flv tag
	n += t.Muxer.Input(bytes[n:], packet.MediaType, len(data), dts, pts, false, frameType)
	copy(bytes[n:], data)

	// Merge until full and then send
	if segment, key := t.MWBuffer.TryFlushSegment(); segment != nil {
		if !keyBuffer {
			keyBuffer = key
		}

		// The memory for the trailing newline character has been allocated, add it directly
		segment.ResetData(FormatSegment(segment.Get()))
		t.AppendOutStreamBuffer(segment)
	}

	return t.OutBuffer[:t.OutBufferSize], 0, keyBuffer, nil
}

func (t *TransStream) AddTrack(track *stream.Track) error {
	if err := t.BaseTransStream.AddTrack(track); err != nil {
		return err
	}

	if utils.AVMediaTypeAudio == track.Stream.MediaType {
		t.Muxer.AddAudioTrack(track.Stream)
	} else if utils.AVMediaTypeVideo == track.Stream.MediaType {
		t.Muxer.AddVideoTrack(track.Stream)

		t.Muxer.MetaData().AddNumberProperty("width", float64(track.Stream.CodecParameters.Width()))
		t.Muxer.MetaData().AddNumberProperty("height", float64(track.Stream.CodecParameters.Height()))
	}
	return nil
}

func (t *TransStream) WriteHeader() error {
	log.Print(" http_flv.go -> WriteHeader")
	var header [4096]byte
	size := t.Muxer.WriteHeader(header[:])
	tags := header[9:size]
	copy(t.flvHeaderBlock[HttpFlvBlockHeaderSize:], header[:9])
	copy(t.flvExtraDataBlock[HttpFlvBlockHeaderSize:], tags)

	t.flvExtraDataPreTagSize = t.Muxer.PrevTagSize()

	// +2 加上末尾换行符
	t.flvExtraDataBlock = t.flvExtraDataBlock[:HttpFlvBlockHeaderSize+size-9+2]
	writeSeparator(t.flvHeaderBlock)
	writeSeparator(t.flvExtraDataBlock)

	t.MWBuffer = stream.NewMergeWritingBuffer(t.ExistVideo)
	return nil
}

func (t *TransStream) ReadExtraData(_ int64) ([]*collections.ReferenceCounter[[]byte], int64, error) {
	return []*collections.ReferenceCounter[[]byte]{collections.NewReferenceCounter(GetHttpFLVBlock(t.flvHeaderBlock)), collections.NewReferenceCounter(GetHttpFLVBlock(t.flvExtraDataBlock))}, 0, nil
}

func (t *TransStream) ReadKeyFrameBuffer() ([]*collections.ReferenceCounter[[]byte], int64, error) {
	t.ClearOutStreamBuffer()

	// Send the merged write slices that already exist in the current memory pool
	t.MWBuffer.ReadSegmentsFromKeyFrameIndex(func(segment *collections.ReferenceCounter[[]byte]) {
		// 修改第一个flv tag的pre tag size为sequence header tag size
		bytes := segment.Get()
		if t.OutBufferSize < 1 {
			binary.BigEndian.PutUint32(GetFLVTag(bytes), t.flvExtraDataPreTagSize)
		}

		t.AppendOutStreamBuffer(segment)
	})

	return t.OutBuffer[:t.OutBufferSize], 0, nil
}

func (t *TransStream) Close() ([]*collections.ReferenceCounter[[]byte], int64, error) {
	t.ClearOutStreamBuffer()

	// 发送剩余的流
	if !t.MWBuffer.IsNewSegment() {
		if segment, _ := t.flushSegment(); segment != nil {
			t.AppendOutStreamBuffer(segment)
		}
	}

	return t.OutBuffer[:t.OutBufferSize], 0, nil
}

// Save as a complete http-flv slice
func (t *TransStream) flushSegment() (*collections.ReferenceCounter[[]byte], bool) {
	// preview end line break
	t.MWBuffer.Reserve(2)
	segment, key := t.MWBuffer.FlushSegment()
	segment.ResetData(FormatSegment(segment.Get()))
	return segment, key
}

func NewHttpTransStream(metadata *amf0.Object, prevTagSize uint32) stream.TransStream {
	return &TransStream{
		Muxer:             flv.NewMuxerWithPrevTagSize(metadata, prevTagSize),
		flvHeaderBlock:    make([]byte, 31),
		flvExtraDataBlock: make([]byte, 4096),
	}
}

func TransStreamFactory(source stream.Source, protocol stream.TransStreamProtocol, tracks []*stream.Track) (stream.TransStream, error) {
	var prevTagSize uint32
	var metaData *amf0.Object

	endInfo := source.GetStreamEndInfo()
	if endInfo != nil {
		prevTagSize = endInfo.FLVPrevTagSize
	}

	if stream.SourceTypeRtmp == source.GetType() {
		metaData = source.(*rtmp.Publisher).Stack.Metadata()
	}

	return NewHttpTransStream(metaData, prevTagSize), nil
}
