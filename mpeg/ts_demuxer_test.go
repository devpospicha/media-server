package mpeg

import (
	"bufio"
	"os"
	"testing"

	"github.com/devpospicha/media-server/avformat"
)

type OnTSStreamHandler struct {
	avformat.OnUnpackStream2FileHandler
	muxer  *TSMuxer
	buffer []byte
	fos    *os.File
}

func (o OnTSStreamHandler) OnNewTrack(track avformat.Track) {
	o.OnUnpackStreamLogger.OnNewTrack(track)

	stream := track.GetStream()
	_, err := o.muxer.AddTrack(stream.MediaType, stream.CodecID, stream.Data)
	if err != nil {
		panic(err)
	}

	n, err := o.muxer.WriteHeader(o.buffer)
	if err != nil {
		panic(err)
	}

	_, err = o.fos.Write(o.buffer[:n])
}

func (o OnTSStreamHandler) OnTrackComplete() {
	o.OnUnpackStreamLogger.OnTrackComplete()
}

func (o OnTSStreamHandler) OnPacket(packet *avformat.AVPacket) {
	o.OnUnpackStreamLogger.OnPacket(packet)

	data := packet.Data
	length := len(data)

	for i := 0; i < length; {
		n := o.muxer.Input(o.buffer, packet.Index, packet.Data[i:], length, packet.Dts, packet.Pts, packet.Key, i == 0)
		_, err := o.fos.Write(o.buffer[:TsPacketSize])
		if err != nil {
			panic(err)
		}

		i += n
	}
}

func TestTSDeMuxer(t *testing.T) {
	files := []string{
		////"output.ts",
		//"00032_test_ts.ts",
		////"error.ts",
		//// "ok.ts",
		//// "test.mp4.ts",
		//// "test1.ts",
		//"hls.ts",
		"mystream_0.ts",
	}

	getSourceFilePath := func(file string) string {
		return "../source_files/" + file
	}

	unpack := func(name string, handler avformat.OnUnpackStreamHandler) {
		demuxer := NewTSDemuxer()
		demuxer.SetHandler(handler)

		bytes := make([]byte, 188)
		file, err := os.Open(getSourceFilePath(name))
		if err != nil {
			panic(err)
		}

		reader := bufio.NewReader(file)

		var size int
		var n int
		for n, err = reader.Read(bytes[size:]); err == nil; n, err = reader.Read(bytes[size:]) {
			size += n
			if size != 188 {
				continue
			}

			err = demuxer.Input(bytes)
			if err != nil {
				panic(err)
			}

			n = 0
			size = 0
		}

		demuxer.Close()
	}

	t.Run("logger", func(t *testing.T) {
		for _, file := range files {
			unpack(file, &avformat.OnUnpackStreamLogger{})
		}
	})

	t.Run("demux", func(t *testing.T) {

		for _, file := range files {
			unpack(file, &avformat.OnUnpackStream2FileHandler{Path: getSourceFilePath(file)})
		}
	})

	t.Run("remux", func(t *testing.T) {
		for _, file := range files {

			out, err := os.OpenFile(getSourceFilePath(file)+".re_mux.ts", os.O_WRONLY|os.O_CREATE, 132)
			if err != nil {
				panic(err)
			}

			muxer := NewTSMuxer()

			handler := &OnTSStreamHandler{
				OnUnpackStream2FileHandler: avformat.OnUnpackStream2FileHandler{
					Path: getSourceFilePath(file),
				},
				muxer:  muxer,
				buffer: make([]byte, 1024*1024),
				fos:    out,
			}

			unpack(file, handler)
		}
	})
}
