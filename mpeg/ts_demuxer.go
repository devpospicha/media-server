package mpeg

import (
	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/utils"
)

type TSDemuxer struct {
	avformat.BaseDemuxer
	pat *PASection
	pmt *PMSection
	//programs []*PMSection

	ctx struct {
		mediaType   utils.AVMediaType
		codec       utils.AVCodecID
		bufferIndex int
	}
}

func (d *TSDemuxer) isPMT(pid int) bool {
	for _, pmtPID := range d.pat.programMapPID {
		if pmtPID != pid {
			continue
		}

		return true
	}

	return false
}

func (d *TSDemuxer) findStream(pid int) (int, bool) {
	if d.pmt != nil {
		for _, stream := range d.pmt.streams {
			if stream.elementaryPID == pid {
				return stream.streamType, true
			}
		}
	}

	return -1, false
}

func mergePMT(c1, c2 *PMSection) {
	for _, s2 := range c2.streams {
		found := false

		for i := range c1.streams {
			if c1.streams[i].streamType == s2.streamType &&
				c1.streams[i].elementaryPID == s2.elementaryPID {
				found = true
				break
			}
		}

		if !found {
			c1.streams = append(c1.streams, s2)
		}
	}
}

func (d *TSDemuxer) Input(data []byte) error {
	packet := TSPacket{}
	_ = packet.Unmarshal(data)

	var err error
	if PsiPat == packet.pid {
		pat := &PASection{}
		_ = pat.Unmarshal(packet.data)
		d.pat = pat
	} else if d.pat != nil && d.isPMT(packet.pid) {
		pmt := &PMSection{}
		_ = pmt.Unmarshal(packet.data)

		if d.pmt != nil {
			mergePMT(d.pmt, pmt)
		} else {
			d.pmt = pmt
		}
	} else if streamType, ok := d.findStream(packet.pid); ok {
		// pes开始包, 处理前一个pes包
		if packet.payloadUnitStartIndicator == 1 && utils.AVCodecIdNONE != d.ctx.codec {
			if d.DataPipeline.PendingBlockSize(d.ctx.bufferIndex) != 0 {
				pesPkt, _ := d.DataPipeline.Feat(d.ctx.bufferIndex)
				header := PESHeader{}
				n := header.Unmarshal(pesPkt)
				if n > len(pesPkt) {
					return nil
				}

				// 回调同时包含dts和pts
				dts := header.dts
				pts := header.pts

				// 不包含pts使用dts
				// 解析pes头时, 已经确保至少包含一个时间戳
				if header.ptsDtsFlags>>1 == 0 {
					pts = dts
				}

				// 不包含dts使用pts
				if header.ptsDtsFlags&0x1 == 0 {
					dts = pts
				}

				bytes := pesPkt[n:]
				if utils.AVMediaTypeAudio == d.ctx.mediaType {
					d.OnAudioPacket(d.ctx.bufferIndex, d.ctx.codec, bytes, header.pts)
				} else {
					d.OnVideoPacket(d.ctx.bufferIndex, d.ctx.codec, bytes, avformat.IsKeyFrame(d.ctx.codec, bytes), dts, pts, avformat.PacketTypeAnnexB)
				}
			}
		}

		var codec utils.AVCodecID
		var mediaType utils.AVMediaType
		codec, mediaType, err = StreamType2AVCodecID(streamType)
		if err != nil {
			return err
		}

		d.ctx.mediaType = mediaType
		d.ctx.codec = codec
		d.ctx.bufferIndex = d.FindBufferIndex(packet.pid + streamType)

		// pes负载数据
		_, err = d.DataPipeline.Write(packet.data, d.ctx.bufferIndex, d.ctx.mediaType)
	} else if packet.payloadUnitStartIndicator == 1 {
		// section
	}

	return err
}

func NewTSDemuxer() *TSDemuxer {
	demuxer := &TSDemuxer{BaseDemuxer: avformat.BaseDemuxer{
		DataPipeline:  &avformat.StreamsBuffer{},
		Name:          "ts",
		ProbeDuration: 5000,
		AutoFree:      true,
	}}

	return demuxer
}
