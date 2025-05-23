package flv

import (
	"encoding/binary"
	"fmt"

	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
)

type VideoCodecID int

type PacketType byte

const (
	FrameTypeKeyFrame             = iota + 1 // 关键帧
	FrameTypeInterFrame                      // 中间帧
	FrameTypeDisposableInterFrame            // 可丢弃的中间帧（仅H.263）
	FrameTypeGeneratedKeyFrame               // 生成的关键帧（保留供服务器使用）
	FrameTypeVideoInfoCommand                // 视频信息/命令帧

	VideoCodecNONE       = VideoCodecID(-1)
	VideoCodecIDH263     = VideoCodecID(2) // flv1
	VideoCodecIDSCREEN   = VideoCodecID(3)
	VideoCodecIDVP6      = VideoCodecID(4)
	VideoCodecIDVP6Alpha = VideoCodecID(5)
	VideoCodecIDScreenV2 = VideoCodecID(6)
	VideoCodecIDAVC      = VideoCodecID(7)
	VideoCodecIDAV1      = VideoCodecID(1635135537)
	VideoCodecIDVP9      = VideoCodecID(1987063865)
	VideoCodecIDHEVC     = VideoCodecID(1752589105)

	//VideoCodecIDAV1      = VideoCodecID(binary.BigEndian.Uint32([]byte("av01")))
	//VideoCodecIDVP9      = VideoCodecID(binary.BigEndian.Uint32([]byte("vp09")))
	//VideoCodecIDHEVC     = VideoCodecID(binary.BigEndian.Uint32([]byte("hvc1")))

	PacketTypeSequenceStart = PacketType(0) // sequence header
	PacketTypeCodedFrames   = PacketType(1) // 视频帧
	PacketTypeSequenceEnd   = PacketType(2) // 新的视频序列结束
	PacketTypeCodedFramesX  = PacketType(3) // CompositionTime为0, 不包含该字段
	PacketTypeMetaData      = PacketType(4) // 元数据,例如: HDR信息

	// PacketTypeMPEG2TSSequenceStart AV1 video descriptor
	// Reference av1-mpeg2-ts
	// https://code.videolan.org/videolan/av1-mapping-specs/blob/master/ts-carriage.md#41-av1-video-descriptor
	// https://aomediacodec.github.io/av1-mpeg2-ts/#av1-video-descriptor
	PacketTypeMPEG2TSSequenceStart = PacketType(5)

	AudioPacketTypeSequenceStart      = PacketType(0)
	AudioPacketTypeCodedFrames        = PacketType(1)
	AudioPacketTypeSequenceEnd        = PacketType(2)
	AudioPacketTypeMultichannelConfig = PacketType(4)
	AudioPacketTypeMultiTrack         = PacketType(5)
)

func AVCodecID2VideoCodecID(id utils.AVCodecID) (VideoCodecID, error) {
	switch id {
	case utils.AVCodecIdFLV1:
		return VideoCodecIDH263, nil
	case utils.AVCodecIdFLASHSV:
		return VideoCodecIDSCREEN, nil
	case utils.AVCodecIdVP6F:
		return VideoCodecIDVP6, nil
	case utils.AVCodecIdVP6A:
		return VideoCodecIDVP6Alpha, nil
	case utils.AVCodecIdFLASHSV2:
		return VideoCodecIDScreenV2, nil
	case utils.AVCodecIdH264:
		return VideoCodecIDAVC, nil
	case utils.AVCodecIdAV1:
		return VideoCodecIDAV1, nil
	case utils.AVCodecIdVP9:
		return VideoCodecIDVP9, nil
	case utils.AVCodecIdHEVC:
		return VideoCodecIDHEVC, nil
	default:
		return -1, fmt.Errorf("unsupported video codec: %v", id)
	}
}

func VideoCodecID2AVCodecID(id VideoCodecID) (utils.AVCodecID, error) {
	switch id {
	case VideoCodecIDH263:
		return utils.AVCodecIdFLV1, nil
	case VideoCodecIDSCREEN:
		return utils.AVCodecIdFLASHSV, nil
	case VideoCodecIDVP6:
		return utils.AVCodecIdVP6F, nil
	case VideoCodecIDVP6Alpha:
		return utils.AVCodecIdVP6A, nil
	case VideoCodecIDScreenV2:
		return utils.AVCodecIdFLASHSV2, nil
	case VideoCodecIDAVC:
		return utils.AVCodecIdH264, nil
	case VideoCodecIDAV1:
		return utils.AVCodecIdAV1, nil
	case VideoCodecIDVP9:
		return utils.AVCodecIdVP9, nil
	case VideoCodecIDHEVC:
		return utils.AVCodecIdHEVC, nil
	default:
		return utils.AVCodecIdNONE, fmt.Errorf("unknow video codec: %d", id)
	}
}

type VideoData struct {
	CodecID VideoCodecID
}

// Unmarshal 解析视频tag, 返回视频帧数据(AVCC格式), 是否是SequenceHeader, FrameType, CompositionTime
func (v *VideoData) Unmarshal(data []byte) ([]byte, bool, int, int, error) {
	reader := bufio.NewBytesReader(data)
	flags, err := reader.ReadUint8()
	if err != nil {
		return nil, false, -1, 0, err
	}

	enhancedFlv := flags>>7 == 1
	frameType := int(flags >> 4 & 0b0111)
	codecId := VideoCodecID(flags & 0x0F)
	var pktType = PacketType(0xFF)

	if enhancedFlv {
		// Signals to not interpret CodecID UB[4] as a codec identifier. Instead
		// these UB[4] bits are interpreted as PacketType which is then followed
		// by UI32 FourCC value.
		pktType = PacketType(codecId)
		fourcc, err := reader.ReadUint32()
		if err != nil {
			return nil, false, -1, 0, err
		}

		codecId = VideoCodecID(fourcc)
		if PacketTypeMetaData != pktType {
			frameType &= 0x7
		} else {
			// The body does not contain video data. The body is an AMF encoded metadata.
			// The metadata will be represented by a series of [name, value] pairs.
			// For now the only defined [name, value] pair is [“colorInfo”, Object]
			// See Metadata Frame section for more details of this object.
			//
			// For a deeper understanding of the encoding please see description
			// of SCRIPTDATA and SSCRIPTDATAVALUE in the FLV file spec.
			// DATA = [“colorInfo”, Object]
			// amf0 := flv.Data{}
			// err := amf0.Unmarshal(reader.RemainingBytes())
			// if err != nil {
			// 	panic(err)
			// }
		}

		// video info/command frame
		if frameType == 5 {
			return reader.RemainingBytes(), false, frameType, 0, nil
		}
	}

	if !enhancedFlv && VideoCodecIDAVC == codecId {
		type_, err := reader.ReadUint8()
		if err != nil {
			return nil, false, -1, 0, err
		}

		pktType = PacketType(type_)
	}

	// avc/hevc/mpeg4
	var ct uint32
	if VideoCodecIDAVC == codecId || (VideoCodecIDHEVC == codecId && PacketTypeCodedFrames == pktType) {
		if ct, err = reader.ReadUint24(); err != nil {
			return nil, false, -1, 0, err
		}
	}

	// sequence header
	var sequenceHeader bool
	if pktType == PacketTypeSequenceStart && codecId >= VideoCodecIDAVC {
		sequenceHeader = true
	}

	v.CodecID = codecId
	return reader.RemainingBytes(), sequenceHeader, frameType, int(ct), nil
}

func (v *VideoData) Marshal(dst []byte, ct uint32, frameType int, header bool) int {
	_ = dst[4]

	if header {
		frameType = FrameTypeKeyFrame
	}

	var enhancedFlv = v.CodecID > VideoCodecIDAVC
	var flags = (byte(frameType) & 0x7 << 4) | (byte(v.CodecID) & 0x0F)
	var pktType PacketType

	n := 1
	if enhancedFlv {
		flags |= 1 << 7
		binary.BigEndian.PutUint32(dst[n:], uint32(v.CodecID))
		n += 4

		if header {
			pktType = PacketTypeSequenceStart
		} else if VideoCodecIDHEVC == v.CodecID && ct != 0 {
			pktType = PacketTypeCodedFrames
		} else {
			pktType = PacketTypeCodedFramesX
		}

		// 7-5位frame type
		// 后4位包类型
		flags = flags&0xF0 | byte(pktType)
	} else if VideoCodecIDAVC == v.CodecID {
		if header {
			pktType = PacketTypeSequenceStart
			dst[n] = 0
		} else {
			pktType = PacketTypeCodedFrames
			dst[n] = 1
		}
		n++
	}

	dst[0] = flags
	if !enhancedFlv || PacketTypeCodedFrames == pktType {
		bufio.PutUint24(dst[n:], ct)
		n += 3
	}

	return n
}

func NewVideoData(id utils.AVCodecID) (*VideoData, error) {
	codecID, err := AVCodecID2VideoCodecID(id)
	if err != nil {
		return nil, err
	}

	data := &VideoData{CodecID: codecID}
	return data, nil
}
