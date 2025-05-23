package mpeg

import (
	"fmt"

	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/avc"
	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
)

// PSDemuxer PS流解复用器
type PSDemuxer struct {
	avformat.BaseDemuxer

	packHeader       *PackHeader
	systemHeader     *SystemHeader
	programStreamMap *ProgramStreamMap
	pesHeader        *PESHeader
	reader           bufio.BytesReader

	ctx struct {
		readESLength      uint16 // 已经读取到的ES流长度
		esPacketTotalSize int
		codecId           utils.AVCodecID
		mediaType         utils.AVMediaType
		dts               int64
		pts               int64
	}
}

// 读取并解析非pes包, 返回是否定位到pes包
func (d *PSDemuxer) locatePESHeader(reader bufio.BytesReader) bool {
	for {
		startCode := avc.FindStartCodeWithReader(reader)
		if startCode < 0 {
			return false
		}

		// 回退4个字节, 以0x00000001开头
		_ = reader.SeekBack(4)
		var n int
		if startCode == 0xBA {
			header := PackHeader{}
			if n = header.Unmarshal(reader.RemainingBytes()); n > 0 {
				*d.packHeader = header
			}
		} else if startCode == 0xBB {
			header := SystemHeader{}
			if n = header.Unmarshal(reader.RemainingBytes()); n > 0 {
				*d.systemHeader = header
			}
		} else if startCode == 0xBC {
			psm := ProgramStreamMap{}
			if n = psm.Unmarshal(reader.RemainingBytes()); n > 0 {
				*d.programStreamMap = psm
			}
		} else if StreamIDPrivateStream1 == startCode || StreamIDPaddingStream == startCode || StreamIDPrivateStream2 == startCode {
			// 跳过PrivateStream
			// PrivateStream1解析可参考https://github.com/FFmpeg/FFmpeg/blob/release/7.0/libavformat/mpeg.c#L361
			// PrivateStream2解析可以参考https://github.com/FFmpeg/FFmpeg/blob/release/7.0/libavformat/mpeg.c#L266
			_ = reader.Seek(4)

			skipCount, err := reader.ReadUint16()
			if err != nil {
				// 需要更多数据
				_ = reader.SeekBack(4)
				return false
			} else if reader.Seek(int(skipCount)) != nil {
				// 需要更多数据
				_ = reader.SeekBack(6)
				return false
			}
		} else if !((startCode >= 0xc0 && startCode <= 0xdf) || (startCode >= 0xe0 && startCode <= 0xef)) {
			// 跳过非pes包
			_ = reader.Seek(4)
		} else {
			// 找到pes包
			break
		}

		// 读取到非Pes Header
		if startCode < 0xBD {
			// 解析Header失败, 需要更多数据
			if n < 1 {
				return false
			}

			_ = reader.Seek(n)
		}
	}

	return true
}

// Input 确保输入流的连续性, 比如一个视频帧有多个PES包, 多个PES包必须是连续的, 不允许插入非当前帧PES包.
func (d *PSDemuxer) Input(data []byte) (int, error) {
	d.reader.Reset(data)

	codec := d.ctx.codecId
	mediaType := d.ctx.mediaType

	for d.reader.ReadableBytes() > 0 {
		// 回调es数据
		need := d.pesHeader.esLength - d.ctx.readESLength
		if need > 0 {
			consume := bufio.MinInt(int(need), d.reader.ReadableBytes())
			bytes, _ := d.reader.ReadBytes(consume)

			utils.Assert(len(bytes) > 0)
			if err := d.callbackES(bytes, mediaType, codec); err != nil {
				return d.reader.Offset(), err
			}

			continue
		}

		ok := d.locatePESHeader(d.reader)
		if len(d.programStreamMap.elementaryStreams) < 1 {
			fmt.Printf("skipped %d invalid data bytes\r\n", len(data))
			return len(data), nil
		} else if !ok {
			break
		}

		pesHeader := PESHeader{}
		n := pesHeader.Unmarshal(d.reader.RemainingBytes())
		if n < 1 {
			break
		} else {
			_ = d.reader.Seek(n)
			*d.pesHeader = pesHeader
		}

		elementaryStream, b := d.programStreamMap.findElementaryStream(d.pesHeader.streamId)
		if !b {
			fmt.Printf("unknow stream id:%x \r\n", d.pesHeader.streamId)
		}

		var err error
		codec, mediaType, err = StreamType2AVCodecID(int(elementaryStream.streamType))
		if err != nil {
			return -1, err
		}
	}

	return d.reader.Offset(), nil
}

func (d *PSDemuxer) callbackES(data []byte, mediaType utils.AVMediaType, codecId utils.AVCodecID) error {
	// 完善时间戳, 回调同时包含dts和pts
	dts := d.pesHeader.dts
	pts := d.pesHeader.pts

	// 不包含pts使用dts
	// 解析pes头时, 已经确保至少包含一个时间戳
	if d.pesHeader.ptsDtsFlags>>1 == 0 {
		pts = dts
	}

	// 不包含dts使用pts
	if d.pesHeader.ptsDtsFlags&0x1 == 0 {
		dts = pts
	}

	err := d.onEsPacket(data, int(d.pesHeader.esLength), d.ctx.readESLength == 0, mediaType, codecId, dts, pts)
	d.ctx.readESLength += uint16(len(data))

	// 读取到完整pes包, 重置标记
	if d.ctx.readESLength == d.pesHeader.esLength {
		d.ctx.readESLength = 0
		d.pesHeader.esLength = 0
	}

	return err
}

func (d *PSDemuxer) onEsPacket(data []byte, total int, first bool, mediaType utils.AVMediaType, id utils.AVCodecID, dts int64, pts int64) error {
	// 处理前一个es包. 根据时间戳和类型的变化, 决定丢弃或者回调完整包
	index := d.BaseDemuxer.FindBufferIndexByMediaType(d.ctx.mediaType)
	if size := d.DataPipeline.PendingBlockSize(index); size > 0 && first && ((dts > d.ctx.dts || pts > d.ctx.pts) || d.ctx.mediaType != mediaType) {
		// 丢包造成数据不足, 释放之前的缓存数据, 丢弃帧
		if size < d.ctx.esPacketTotalSize {
			fmt.Printf("loss packet data size: %d\r\n", size)
			d.DataPipeline.DiscardBackPacket(index)
		} else {
			pktData, err := d.DataPipeline.Feat(index)
			if err != nil {
				return err
			}

			if utils.AVMediaTypeAudio == d.ctx.mediaType {
				d.OnAudioPacket(index, d.ctx.codecId, pktData, d.ctx.dts)
			} else if utils.AVMediaTypeVideo == d.ctx.mediaType {
				d.OnVideoPacket(index, d.ctx.codecId, pktData, avformat.IsKeyFrame(d.ctx.codecId, pktData), d.ctx.dts, d.ctx.pts, avformat.PacketTypeAnnexB)
			}

			if !d.Completed && d.Tracks.Size() > 1 {
				d.ProbeComplete()
			}
		}
	}

	// 第一包, 重置标记
	writeIndex := d.BaseDemuxer.FindBufferIndexByMediaType(mediaType)
	if d.DataPipeline.PendingBlockSize(writeIndex) == 0 {
		d.ctx.mediaType = mediaType
		d.ctx.codecId = id
		d.ctx.dts = dts
		d.ctx.pts = pts
		d.ctx.esPacketTotalSize = total
	}

	// 回调部分es数据
	_, err := d.DataPipeline.Write(data, writeIndex, mediaType)
	if err != nil {
		return err
	}

	return nil
}

func NewPSDemuxer(autoFree bool) *PSDemuxer {
	return &PSDemuxer{
		BaseDemuxer: avformat.BaseDemuxer{
			DataPipeline: &avformat.StreamsBuffer{},
			Name:         "ps", // vob
			AutoFree:     autoFree,
		},

		packHeader:       &PackHeader{},
		systemHeader:     &SystemHeader{},
		programStreamMap: &ProgramStreamMap{},
		pesHeader:        &PESHeader{},
		reader:           bufio.NewBytesReader(nil),
	}
}
