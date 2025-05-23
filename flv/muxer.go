package flv

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/flv/amf0"
)

type Muxer struct {
	avformat.BaseMuxer
	metaData    *amf0.Object
	AudioData   AudioData
	VideoData   VideoData
	prevTagSize uint32
}

func (m *Muxer) AddTrack(stream *avformat.AVStream) (int, error) {
	if utils.AVMediaTypeAudio == stream.MediaType {
		return m.AddAudioTrack(stream)
	} else if utils.AVMediaTypeVideo == stream.MediaType {
		return m.AddVideoTrack(stream)
	} else {
		panic(fmt.Sprintf("unsupported media type: %s", stream.MediaType))
	}
}

func (m *Muxer) AddAudioTrack(stream *avformat.AVStream) (int, error) {
	data, err := NewAudioData(stream.CodecID, stream.SampleRate, stream.SampleSize, stream.Channels)
	if err != nil {
		return -1, err
	}

	index, err := m.BaseMuxer.AddTrack(&avformat.SimpleTrack{Stream: stream})
	if err != nil {
		return -1, err
	}

	m.AudioData = *data
	if m.metaData.FindProperty("audiocodecid") == nil {
		m.metaData.AddNumberProperty("audiocodecid", float64(m.AudioData.SoundFormat))
	}

	if m.metaData.FindProperty("audiosamplerate") == nil {
		m.metaData.AddNumberProperty("audiosamplerate", float64(m.AudioData.Rate))
	}

	return index, nil
}

func (m *Muxer) AddVideoTrack(stream *avformat.AVStream) (int, error) {
	data, err := NewVideoData(stream.CodecID)
	if err != nil {
		return -1, err
	}

	index, err := m.BaseMuxer.AddTrack(&avformat.SimpleTrack{Stream: stream})
	if err != nil {
		return -1, err
	}

	m.VideoData = *data
	if m.metaData.FindProperty("videocodecid") == nil {
		m.metaData.AddNumberProperty("videocodecid", float64(m.VideoData.CodecID))
	}
	return index, nil
}

func (m *Muxer) WriteHeader(dst []byte) int {
	log.Print("muxer.go -> WriteHeader")
	// signature
	dst[0] = 0x46
	dst[1] = 0x4C
	dst[2] = 0x56
	// version
	dst[3] = 0x1
	// flags
	var flags byte

	if m.Tracks.FindTrackWithType(utils.AVMediaTypeAudio) != nil {
		flags |= 1 << 2
	}

	if m.Tracks.FindTrackWithType(utils.AVMediaTypeVideo) != nil {
		flags |= 1
	}

	dst[4] = flags
	binary.BigEndian.PutUint32(dst[5:], 0x9)
	// script data
	data := amf0.Data{}
	data.AddString("onMetaData")
	data.Add(m.metaData)
	// 先写metadata
	n, err := data.Marshal(dst[9+TagHeaderSize:])
	if err != nil {
		panic(err)
	}

	// 再写tag
	n += m.WriteTag(dst[9:], TagTypeScriptData, uint32(n), 0)
	totalWritten := 9 + n

	// 写sequence header
	totalWritten += m.writeSequenceHeader(dst[totalWritten:])
	return totalWritten
}

func (m *Muxer) writeSequenceHeader(dst []byte) int {
	var totalWritten int

	for _, track := range m.Tracks.Tracks {
		extraData := track.GetStream().Data
		if len(extraData) == 0 {
			continue
		} else if utils.AVMediaTypeVideo == track.GetStream().MediaType {
			extraData = track.GetStream().CodecParameters.MP4ExtraData()
		}

		//	track := s.muxer.Tracks.Get(packet.Index)
		//	if track.GetStream().Colors != nil {
		//		_, err := s.muxer.Input(s.file, packet.Index, track.GetStream().Colors, packet.Dts, packet.Pts, false, FrameTypeVideoInfoCommand)
		//		if err != nil {
		//			panic(err)
		//		}
		//	}

		stream := track.GetStream()
		n := m.Input(dst[totalWritten:], stream.MediaType, len(extraData), 0, 0, true, 0)

		totalWritten += n
		copy(dst[totalWritten:], extraData)
		totalWritten += len(extraData)
	}

	return totalWritten
}

func (m *Muxer) Input(dst []byte, mediaType utils.AVMediaType, size int, dts, pts int64, header bool, frameType int) int {
	if utils.AVMediaTypeAudio == mediaType {
		n := m.AudioData.Marshal(dst[TagHeaderSize:], header)
		n += m.WriteTag(dst, TagTypeAudioData, uint32(size+n), uint32(dts))
		return n
	} else if utils.AVMediaTypeVideo == mediaType {
		n := m.VideoData.Marshal(dst[TagHeaderSize:], uint32(pts-dts), frameType, header)
		n += m.WriteTag(dst, TagTypeVideoData, uint32(size+n), uint32(dts))
		return n
	} else {
		panic(fmt.Sprintf("unsupported media type: %s", mediaType))
	}
}

func (m *Muxer) WriteTag(dst []byte, tag TagType, dataSize, timestamp uint32) int {
	binary.BigEndian.PutUint32(dst, m.prevTagSize)
	dst[4] = byte(tag)
	bufio.PutUint24(dst[5:], dataSize)
	bufio.PutUint24(dst[8:], timestamp&0xFFFFFF)
	dst[11] = byte(timestamp >> 24)
	bufio.PutUint24(dst[12:], 0)

	m.prevTagSize = 11 + dataSize
	return TagHeaderSize
}

func (m *Muxer) WriteTagWithMediaType(dst []byte, mediaType utils.AVMediaType, dataSize, timestamp uint32) int {
	var tag TagType

	if utils.AVMediaTypeAudio == mediaType {
		tag = TagTypeAudioData
	} else if utils.AVMediaTypeVideo == mediaType {
		tag = TagTypeVideoData
	}

	return m.WriteTag(dst, tag, dataSize, timestamp)
}

func (m *Muxer) PrevTagSize() uint32 {
	return m.prevTagSize
}

func (m *Muxer) MetaData() *amf0.Object {
	return m.metaData
}

func (m *Muxer) ComputeVideoDataHeaderSize(ct uint32) int {
	if m.VideoData.CodecID > 0xF && ct > 0 {
		return 8
	}

	return 5
}

func (m *Muxer) ComputeAudioDataHeaderSize() int {
	if SoundFormatAAC == m.AudioData.SoundFormat {
		return 2
	}

	return 1
}

func NewMuxer(metaData *amf0.Object) *Muxer {
	return NewMuxerWithPrevTagSize(metaData, 0)
}

func NewMuxerWithPrevTagSize(metaData *amf0.Object, prevTagSize uint32) *Muxer {
	if metaData == nil {
		metaData = &amf0.Object{}
	}

	m := &Muxer{
		metaData:    metaData,
		prevTagSize: prevTagSize,
	}

	if metaData.FindProperty("creationtime") == nil {
		m.metaData.AddStringProperty("creationtime", time.Now().Format("2006-01-02 15:04:05"))
	}

	return m
}
