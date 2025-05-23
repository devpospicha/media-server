package mpeg

import (
	"bufio"
	"os"
	"testing"

	"github.com/devpospicha/media-server/avformat"
	bufio2 "github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
)

type OnStreamHandler struct {
	avformat.OnUnpackStream2FileHandler
	muxer  *PSMuxer
	buffer []byte
	fos    *os.File
}

func (o OnStreamHandler) OnNewTrack(stream avformat.Track) {
	o.OnUnpackStreamLogger.OnNewTrack(stream)
	_, err := o.muxer.AddTrack(stream.GetStream().MediaType, stream.GetStream().CodecID)
	if err != nil {
		panic(err)
	}
}

func (o OnStreamHandler) OnTrackComplete() {
	o.OnUnpackStreamLogger.OnTrackComplete()
}

func (o OnStreamHandler) OnPacket(packet *avformat.AVPacket) {
	o.OnUnpackStreamLogger.OnPacket(packet)
	n := o.muxer.Input(o.buffer, packet.Index, packet.Key, packet.Data, &packet.Dts, &packet.Pts)
	_, err := o.fos.Write(o.buffer[:n])
	if err != nil {
		panic(err)
	}
}

func TestDecodePS(t *testing.T) {
	files := []string{
		//"test.ps",
		//"video8888881.ps",
		//"无法回调出H264数据.ps",
		//"hikvisionH264.ps",
		//"hikvisionH265.ps",
		"ps_demux.ps",
	}

	getSourceFilePath := func(file string) string {
		return "../source_files/" + file
	}

	unpack := func(path string, handler avformat.OnUnpackStreamHandler) {
		demuxer := NewPSDemuxer(true)
		demuxer.BaseDemuxer.ProbeDuration = 1000
		demuxer.SetHandler(handler)

		file, err := os.Open(getSourceFilePath(path))
		if err != nil {
			panic(err)
		}

		// 解析缓冲区
		bytes := make([]byte, 1024*1024)
		// 每次从文件最多读取1500字节(模拟网络收包)
		maxReadSize := 1500

		reader := bufio.NewReader(file)
		var offset int
		for n, err := reader.Read(bytes[offset:bufio2.MinInt(offset+maxReadSize, len(bytes))]); n > 0 && err == nil; n, err = reader.Read(bytes[offset:bufio2.MinInt(offset+maxReadSize, len(bytes))]) {
			end := offset + n
			consume, err := demuxer.Input(bytes[:end])
			if err != nil {
				panic(err)
			}

			offset = end - consume
			utils.Assert(offset < len(bytes))

			if offset > 0 {
				copy(bytes, bytes[end-offset:end])
			}
		}

		demuxer.Close()
	}

	t.Run("logger", func(t *testing.T) {
		for _, path := range files {
			unpack(path, &avformat.OnUnpackStreamLogger{})
		}
	})

	t.Run("demux", func(t *testing.T) {
		for _, file := range files {
			unpack(file, &avformat.OnUnpackStream2FileHandler{Path: getSourceFilePath(file)})
		}
	})

	t.Run("remux", func(t *testing.T) {
		for _, path := range files {

			out, err := os.OpenFile(getSourceFilePath(path)+".re_mux.ps", os.O_WRONLY|os.O_CREATE, 132)
			if err != nil {
				panic(err)
			}

			unpack(path, &OnStreamHandler{
				OnUnpackStream2FileHandler: avformat.OnUnpackStream2FileHandler{
					Path: getSourceFilePath(path),
				},
				muxer:  NewPsMuxer(),
				buffer: make([]byte, 1024*1024),
				fos:    out,
			})
		}
	})
}
