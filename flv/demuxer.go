package flv

import (
	"fmt"

	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/bufio"
	utils "github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/flv/amf0"
)

const (
	TagHeaderSize = 15
)

type Demuxer struct {
	avformat.BaseDemuxer

	flag        *TypeFlag
	tag         Tag
	tagDataSize int

	metadata       *amf0.Data // 元数据
	preTagDataSize uint32

	// onAV1Descriptor func(data []byte, ts uint32)
}

func (d *Demuxer) Metadata() *amf0.Data {
	return d.metadata
}

func (d *Demuxer) Input(data []byte) (int, error) {
	length := len(data)
	var n int

	// 解析flv头
	if d.flag == nil {
		if length < 9 {
			return 0, nil
		}

		flag, err := UnmarshalHeader(data)
		if err != nil {
			return 0, err
		}

		d.flag = flag
		n = 9
	}

	for n < length {
		// 读取tag data
		if need := d.tag.DataSize - d.tagDataSize; need > 0 {
			n += d.readTagData(data[n:])
			if d.tag.DataSize != d.tagDataSize {
				break
			} else if err := d.processTag(); err != nil {
				return n, err
			}
		}

		if n+TagHeaderSize > length {
			break
		}

		d.tag = UnmarshalTag(data[n:])
		n += TagHeaderSize
	}

	return n, nil
}

func (d *Demuxer) readTagData(data []byte) int {
	min := bufio.MinInt(d.tag.DataSize-d.tagDataSize, len(data))
	mediaType := TagType2AVMediaType(d.tag.Type)
	n, _ := d.BaseDemuxer.DataPipeline.Write(data[:min], d.BaseDemuxer.FindBufferIndexByMediaType(mediaType), mediaType)
	d.tagDataSize = n
	return min
}

func (d *Demuxer) processTag() error {
	index := d.BaseDemuxer.FindBufferIndexByMediaType(TagType2AVMediaType(d.tag.Type))
	bytes, _ := d.BaseDemuxer.DataPipeline.Feat(index)

	var err error
	var discard bool

	defer func() {
		d.tag = Tag{}
		d.tagDataSize = 0

		if discard || err != nil {
			d.DataPipeline.DiscardBackPacket(index)
		}
	}()

	utils.Assert(d.tag.PrevTagSize == 0 || d.preTagDataSize+11 == d.tag.PrevTagSize)
	d.preTagDataSize = uint32(d.tag.DataSize)

	if TagTypeAudioData == d.tag.Type {
		discard, err = d.ProcessAudioData(bytes, d.tag.Timestamp)
	} else if TagTypeVideoData == d.tag.Type {
		discard, err = d.ProcessVideoData(bytes, d.tag.Timestamp)
	} else if TagTypeScriptData == d.tag.Type {
		data := amf0.Data{}
		err = data.Unmarshal(bytes)

		if err != nil {
			println(err.Error())
		} else {
			d.metadata = &data
		}
	} else {
		fmt.Printf("unkonw tag type %d\r\n", d.tag.Type)
		discard = true
	}

	return err
}

func (d *Demuxer) ProcessAudioData(data []byte, ts uint32) (bool, error) {
	audioData := AudioData{}
	frame, header, err := audioData.Unmarshal(data)
	if err != nil {
		return true, err
	}

	id, err := SoundFormat2AVCodecID(audioData.SoundFormat, audioData.Size)
	if err != nil {
		return true, err
	}

	rate := GetSampleRate(audioData.Rate)
	var bits int
	var channels int
	if 0 == audioData.Size {
		bits = 8
	} else {
		bits = 16
	}

	if 1 == audioData.Type {
		channels = 2
	} else {
		channels = 1
	}

	config := avformat.AudioConfig{
		SampleRate:    rate,
		SampleSize:    bits,
		Channels:      channels,
		HasADTSHeader: false,
	}

	return false, d.processAudioData(id, ts, frame, header, config)
}

func (d *Demuxer) ProcessVideoData(data []byte, ts uint32) (bool, error) {
	videoData := VideoData{}
	frame, header, frameType, ct, err := videoData.Unmarshal(data)
	if err != nil {
		return true, err
	} else if FrameTypeVideoInfoCommand == frameType {
		track := d.Tracks.FindTrackWithType(utils.AVMediaTypeVideo)
		bytes := make([]byte, len(frame))
		copy(bytes, frame)
		track.GetStream().Colors = bytes
		return true, nil
	}

	id, err := VideoCodecID2AVCodecID(videoData.CodecID)
	if err != nil {
		return true, err
	}

	return false, d.processVideoData(id, ts, frame, header, frameType == FrameTypeKeyFrame, ct)
}

func (d *Demuxer) processAudioData(id utils.AVCodecID, ts uint32, frame []byte, header bool, config avformat.AudioConfig) error {
	bufferIndex := d.FindBufferIndexByMediaType(utils.AVMediaTypeAudio)
	if header {
		d.BaseDemuxer.OnNewAudioTrack(bufferIndex, id, 1000, frame, config)
	} else {
		d.BaseDemuxer.OnAudioPacket(bufferIndex, id, frame, int64(ts))
	}
	return nil
}

func (d *Demuxer) processVideoData(id utils.AVCodecID, ts uint32, frame []byte, header, key bool, ct int) error {
	bufferIndex := d.FindBufferIndexByMediaType(utils.AVMediaTypeVideo)
	if header {
		d.BaseDemuxer.OnNewVideoTrack(bufferIndex, id, 1000, frame)
	} else {
		d.BaseDemuxer.OnVideoPacket(bufferIndex, id, frame, key, int64(ts), int64(ts+uint32(ct)), avformat.PacketTypeAVCC)
	}
	return nil
}

func NewDemuxer(autoFree bool) *Demuxer {
	demuxer := &Demuxer{
		BaseDemuxer: avformat.BaseDemuxer{
			DataPipeline: &avformat.StreamsBuffer{},
			Name:         "flv",
			AutoFree:     autoFree,
		},
	}

	return demuxer
}
