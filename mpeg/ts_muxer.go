package mpeg

import (
	"encoding/binary"
	"math"

	"github.com/devpospicha/media-server/avformat/utils"
)

// TSMuxer TS视频流，由多个固定大小的TS包组成, 一般多为188包长. 每个TS包都拥有一个TSHeader, 用于标识TS流数据类型和序号.
// TS包分为2种，PSI和PES. PSI(PAT/PMT...)用于描述音视频流信息. PES负载音视频流.
// PAT携带PMT的PID, PMT里面存储音视频的StreamType和StreamId
// PAT->PMT->DATA...PAT->PMT->DATA
type TSMuxer struct {
	tracks []*tsTrack
}

type tsTrack struct {
	streamType    int
	packet        *TSPacket
	pes           *PESHeader
	pesHeaderData []byte
	mediaType     utils.AVMediaType
	codecId       utils.AVCodecID
	extra         []byte
	audioConfig   *utils.MPEG4AudioConfig
	startTs       int64
	endTs         int64
}

func (t *TSMuxer) AddTrack(mediaType utils.AVMediaType, id utils.AVCodecID, extraData []byte) (int, error) {
	var pes *PESHeader
	if utils.AVMediaTypeAudio == mediaType {
		pes = NewPESHeader(StreamIDAudio)
	} else if utils.AVMediaTypeVideo == mediaType {
		pes = NewPESHeader(StreamIDVideo)
	} else {
		utils.Assert(false)
	}

	streamType, err := AVCodecID2StreamType(id)
	if err != nil {
		return -1, err
	}

	for _, track := range t.tracks {
		utils.Assert(track.streamType != streamType)
	}

	track := &tsTrack{streamType: streamType, pes: pes, pesHeaderData: make([]byte, 128),
		packet: &TSPacket{
			pid: TsPacketStartPid + len(t.tracks),
		},
		mediaType: mediaType, codecId: id,
		startTs: -1,
		endTs:   -1,
	}

	if length := len(extraData); length > 0 {
		track.extra = make([]byte, length)
		copy(track.extra, extraData)

		if utils.AVCodecIdAAC == id {
			var audioConfig *utils.MPEG4AudioConfig
			audioConfig, err = utils.ParseMpeg4AudioConfig(extraData)
			if err != nil {
				return -1, err
			}

			track.audioConfig = audioConfig
		}
	}

	t.tracks = append(t.tracks, track)

	return len(t.tracks) - 1, nil
}

func (t *TSMuxer) WriteHeader(dst []byte) (int, error) {
	utils.Assert(len(t.tracks) > 0)

	// 写PAT
	tsPacket := TSPacket{
		pid: PsiPat,
	}

	paSection := PASection{}
	paSection.programMapPID = append(paSection.programMapPID, PsiPmt)

	sectionData := make([]byte, 1024)
	n := paSection.Marshal(sectionData)
	utils.Assert(n == tsPacket.MarshalSection(dst, sectionData[:n]))

	// 写PMT
	pmSection := PMSection{}
	for _, track := range t.tracks {
		pmSection.streams = append(pmSection.streams, struct {
			streamType    int
			elementaryPID int
			esInfo        []byte
		}{streamType: track.streamType, elementaryPID: track.packet.pid})
	}

	n = pmSection.Marshal(sectionData)
	tsPacket.pid = PsiPmt
	utils.Assert(n == tsPacket.MarshalSection(dst[TsPacketSize:], sectionData[:n]))
	return TsPacketSize * 2, nil
}

func (t *TSMuxer) write(dst []byte, track *tsTrack, dts, pts int64, first bool, size int, data ...[]byte) int {
	var n int
	if n < size {
		// 首包加入pcr
		if first {
			if utils.AVMediaTypeVideo == track.mediaType && track.startTs == dts {
				// MPEG-2标准中, 时钟频率为27MHZ
				// PES中的DTS和PTS 90KHZ
				// 不能超过42位
				// utils.Assert(pcr <= 0x3FFFFFFFFF)
				// PCR(i)=PCR base(i)x300+ PCR ext(i)
				// PCR base(i) = ((system clock frequency x t(i))DIV 300) % 233
				// PCR ext(i) = ((system clock frequency x t(i)) DIV 1) % 300
				pcr := dts * 300
				track.packet.adaptationField.pcrBase = pcr / 300 % int64(math.Pow(2, 33))
				track.packet.adaptationField.pcrExt = int(pcr % 300)
				track.packet.adaptationField.PCRFlag = 1
				track.packet.adaptationFieldEnable = true
			}

			track.packet.payloadUnitStartIndicator = 1
		} else {
			track.packet.payloadUnitStartIndicator = 0
		}

		// 负载pes包
		var count int
		var offset int
		for i, datum := range data {
			// 找到偏移位置
			length := len(datum)
			count += length
			offset = count - n
			// 跳过已经写入的数据
			if offset < 1 {
				continue
			}

			tmp := data[i]
			data[i] = data[i][length-offset:]
			n += track.packet.Marshal(dst, data[i:]...)
			data[i] = tmp
			break
		}
	}

	track.packet.adaptationField.pcrBase = 0
	track.packet.adaptationField.pcrExt = 0
	track.packet.adaptationField.PCRFlag = 0
	track.packet.adaptationFieldEnable = false
	track.packet.increaseCounter()
	return n
}

func (t *TSMuxer) Input(dst []byte, index int, data []byte, totalSize int, dts, pts int64, key bool, first bool) int {
	track := t.tracks[index]
	var composeData [][]byte

	if first {
		if track.startTs == -1 {
			track.startTs = dts
		}

		if dts < track.startTs {
			track.endTs = track.startTs
			track.startTs = dts
		} else {
			track.endTs = dts
		}

		pts = pts % 0x1FFFFFFFF
		dts = dts % 0x1FFFFFFFF

		if utils.AVMediaTypeVideo == track.mediaType {
			track.pes.ptsDtsFlags = PesExistPtsDtsMark
		} else {
			track.pes.ptsDtsFlags = PesExistPtsMark
		}
		track.pes.dts = dts
		track.pes.pts = pts

		// 添加pes头
		pesHeaderLen := track.pes.Marshal(track.pesHeaderData)
		composeData = append(composeData, track.pesHeaderData[:pesHeaderLen])

		if utils.AVMediaTypeVideo == track.mediaType {
			// 视频帧添加aud
			// ios平台/vlc播放需要添加aud
			// 传入的nalu不要携带aud
			if !existAud(data, track.codecId) {
				if n := writeAud(track.pesHeaderData[pesHeaderLen:], track.codecId); n > 0 {
					composeData = append(composeData, track.pesHeaderData[pesHeaderLen:pesHeaderLen+n])
				}
			}

			// sps/pps
			if key && track.extra != nil {
				composeData = append(composeData, track.extra)
			}
		}

		// aac音频帧添加adts
		if utils.AVMediaTypeAudio == track.mediaType && utils.AVCodecIdAAC == track.codecId {
			if _, err := utils.ReadADtsFixedHeader(data); err != nil {
				utils.SetADtsHeader(track.pesHeaderData[pesHeaderLen:], 0, track.audioConfig.ObjectType-1, track.audioConfig.SamplingIndex, track.audioConfig.ChanConfig, 7+totalSize)
				composeData = append(composeData, track.pesHeaderData[pesHeaderLen:pesHeaderLen+7])
			}
		}
	}

	size := 0
	composeData = append(composeData, data)
	for _, bytes := range composeData {
		size += len(bytes)
	}

	// 设置pes包长度
	if first {
		pesPacketLength := size - 6
		if pesPacketLength > 65535 {
			pesPacketLength = 0
		}

		binary.BigEndian.PutUint16(composeData[0][4:], uint16(pesPacketLength))
	}

	n := t.write(dst, track, dts, pts, first, size, composeData...)
	if first {
		return n - (size - len(data))
	} else {
		return n
	}
}

func (t *TSMuxer) Reset() {
	for _, track := range t.tracks {
		track.startTs = -1
		track.endTs = -1
	}

	for _, track := range t.tracks {
		track.packet.payloadUnitStartIndicator = 1
		track.packet.continuityCounter = 0
	}
}

func (t *TSMuxer) Duration() int64 {
	var duration int64
	for _, track := range t.tracks {
		if i := track.endTs - track.startTs; i > duration {
			duration = i
		}
	}
	return duration
}

func (t *TSMuxer) Close() {

}

func NewTSMuxer() *TSMuxer {
	return &TSMuxer{}
}
