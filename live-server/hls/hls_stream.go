package hls

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/log"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/mpeg"
)

type TransStream struct {
	stream.BaseTransStream
	muxer *mpeg.TSMuxer
	ctx   struct {
		segmentSeq      int
		writeBuffer     []byte
		writeBufferSize int
		url             string
		path            string
		file            *os.File
	}

	M3U8Writer     stream.M3U8Writer
	m3u8Name       string
	m3u8File       *os.File
	dir            string
	tsUrl          string
	tsFormat       string
	duration       int
	playlistLength int

	m3u8Sinks      map[stream.SinkID]*M3U8Sink
	PlaylistFormat *string
}

func (t *TransStream) Input(packet *avformat.AVPacket) ([]*collections.ReferenceCounter[[]byte], int64, bool, error) {
	if (!t.ExistVideo || utils.AVMediaTypeVideo == packet.MediaType && packet.Key) && float32(t.muxer.Duration())/90000 >= float32(t.duration) {
		if t.ctx.file != nil {
			if err := t.flushSegment(false); err != nil {
				return nil, -1, false, err
			}
		}
		if err := t.createSegment(); err != nil {
			return nil, -1, false, err
		}
	}

	pts := packet.ConvertPts(90000)
	dts := packet.ConvertDts(90000)
	data := packet.Data
	if utils.AVMediaTypeVideo == packet.MediaType {
		data = avformat.AVCCPacket2AnnexB(t.BaseTransStream.Tracks[packet.Index].Stream, packet)
	}

	length := len(data)
	capacity := cap(t.ctx.writeBuffer)
	for i := 0; i < length; {
		if capacity-t.ctx.writeBufferSize < mpeg.TsPacketSize {
			_, _ = t.ctx.file.Write(t.ctx.writeBuffer[:t.ctx.writeBufferSize])
			t.ctx.writeBufferSize = 0
		}
		bytes := t.ctx.writeBuffer[t.ctx.writeBufferSize : t.ctx.writeBufferSize+mpeg.TsPacketSize]
		i += t.muxer.Input(bytes, packet.Index, data[i:], length, dts, pts, packet.Key, i == 0)
		t.ctx.writeBufferSize += mpeg.TsPacketSize
	}
	return nil, -1, true, nil
}

func (t *TransStream) AddTrack(track *stream.Track) error {
	if err := t.BaseTransStream.AddTrack(track); err != nil {
		return err
	}
	var err error
	if utils.AVMediaTypeVideo == track.Stream.MediaType {
		data := track.Stream.CodecParameters.AnnexBExtraData()
		_, err = t.muxer.AddTrack(track.Stream.MediaType, track.Stream.CodecID, data)
	} else {
		_, err = t.muxer.AddTrack(track.Stream.MediaType, track.Stream.CodecID, track.Stream.Data)
	}
	return err
}

func (t *TransStream) WriteHeader() error {
	return t.createSegment()
}

func (t *TransStream) flushSegment(end bool) error {
	if t.ctx.writeBufferSize > 0 {
		_, _ = t.ctx.file.Write(t.ctx.writeBuffer[:t.ctx.writeBufferSize])
		t.ctx.writeBufferSize = 0
	}
	if err := t.ctx.file.Close(); err != nil {
		return err
	}
	if t.M3U8Writer.Size() >= t.playlistLength {
		_ = os.Remove(t.M3U8Writer.Get(0).Path)
	}
	duration := float32(t.muxer.Duration()) / 90000
	t.M3U8Writer.AddSegment(duration, t.ctx.url, t.ctx.segmentSeq, t.ctx.path)
	m3u8Txt := t.M3U8Writer.String()
	*t.PlaylistFormat = m3u8Txt
	if t.m3u8File != nil {
		if _, err := t.m3u8File.Seek(0, 0); err != nil {
			return err
		} else if err = t.m3u8File.Truncate(0); err != nil {
			return err
		} else if _, err = t.m3u8File.Write([]byte(m3u8Txt)); err != nil {
			return err
		}
	}
	if len(t.m3u8Sinks) > 0 && t.M3U8Writer.Size() > 1 {
		for _, sink := range t.m3u8Sinks {
			sink.SendM3U8Data(t.PlaylistFormat)
		}
		t.m3u8Sinks = make(map[stream.SinkID]*M3U8Sink, 0)
	}
	return nil
}

func (t *TransStream) createSegment() error {
	t.muxer.Reset()
	var tsFile *os.File
	startSeq := t.ctx.segmentSeq + 1
	for {
		tsName := fmt.Sprintf(t.tsFormat, startSeq)
		t.ctx.path = fmt.Sprintf("%s/%s", t.dir, tsName)
		t.ctx.url = fmt.Sprintf("%s%s", t.tsUrl, tsName)
		file, err := os.OpenFile(t.ctx.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err == nil {
			tsFile = file
			break
		}
		log.Sugar.Errorf("Failed to create ts slice file err:%s path:%s", err.Error(), t.ctx.path)
		if os.IsPermission(err) || os.IsTimeout(err) || os.IsNotExist(err) {
			return err
		}
		startSeq++
	}
	t.ctx.segmentSeq = startSeq
	t.ctx.file = tsFile
	n, err := t.muxer.WriteHeader(t.ctx.writeBuffer)
	if err != nil {
		return err
	}
	t.ctx.writeBufferSize = n
	return nil
}

func (t *TransStream) Close() ([]*collections.ReferenceCounter[[]byte], int64, error) {
	var err error
	if t.ctx.file != nil {
		err = t.flushSegment(true)
		err = t.ctx.file.Close()
		t.ctx.file = nil
	}
	if t.muxer != nil {
		t.muxer.Close()
		t.muxer = nil
	}
	if t.m3u8File != nil {
		err = t.m3u8File.Close()
		t.m3u8File = nil
	}
	for _, sink := range t.m3u8Sinks {
		sink.cb(nil)
	}
	t.m3u8Sinks = nil
	return nil, 0, err
}

func DeleteOldSegments(id string) {
	var index int
	for ; ; index++ {
		path := stream.AppConfig.Hls.TSPath(id, strconv.Itoa(index))
		fileInfo, err := os.Stat(path)
		if err != nil && os.IsNotExist(err) {
			break
		} else if fileInfo.IsDir() {
			continue
		}
		_ = os.Remove(path)
	}
}

func NewTransStream(dir, m3u8Name, tsFormat, tsUrl string, segmentDuration, playlistLength int, seq int, playlistFormat *string, writer stream.M3U8Writer) (stream.TransStream, error) {
	m3u8Path := fmt.Sprintf("%s/%s", dir, m3u8Name)
	if err := os.MkdirAll(filepath.Dir(m3u8Path), 0755); err != nil {
		log.Sugar.Errorf("\u521b\u5efaHLS\u76ee\u5f55\u5931\u8d25 err: %s path: %s", err.Error(), m3u8Path)
		return nil, err
	}
	file, err := os.OpenFile(m3u8Path, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		log.Sugar.Errorf("\u521b\u5efam3u8\u6587\u4ef6\u5931\u8d25 err: %s path: %s", err.Error(), m3u8Path)
	}
	transStream := &TransStream{
		m3u8Name:       m3u8Name,
		tsFormat:       tsFormat,
		tsUrl:          tsUrl,
		dir:            dir,
		duration:       segmentDuration,
		playlistLength: playlistLength,
	}
	if writer != nil {
		transStream.M3U8Writer = writer
	} else {
		transStream.M3U8Writer = stream.NewM3U8Writer(playlistLength)
	}
	if playlistFormat != nil {
		transStream.PlaylistFormat = playlistFormat
	} else {
		transStream.PlaylistFormat = new(string)
	}
	transStream.muxer = mpeg.NewTSMuxer()
	transStream.ctx.segmentSeq = seq
	transStream.ctx.writeBuffer = make([]byte, 1024*1024)
	transStream.m3u8File = file
	transStream.m3u8Sinks = make(map[stream.SinkID]*M3U8Sink, 24)
	return transStream, nil
}

func TransStreamFactory(source stream.Source, protocol stream.TransStreamProtocol, tracks []*stream.Track) (stream.TransStream, error) {
	id := source.GetID()
	var writer stream.M3U8Writer
	var playlistFormat *string
	startSeq := -1
	endInfo := source.GetStreamEndInfo()
	if endInfo != nil && endInfo.M3U8Writer != nil {
		writer = endInfo.M3U8Writer
		playlistFormat = endInfo.PlaylistFormat
		startSeq = writer.Get(writer.Size() - 1).Sequence
	}
	return NewTransStream(stream.AppConfig.Hls.M3U8Dir(id), stream.AppConfig.Hls.M3U8Format(id), stream.AppConfig.Hls.TSFormat(id), "", stream.AppConfig.Hls.Duration, stream.AppConfig.Hls.PlaylistLength, startSeq, playlistFormat, writer)
}
