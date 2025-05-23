package flv

import (
	"os"
	"testing"

	"github.com/devpospicha/media-server/avformat"
)

type RemuxHandler struct {
	avformat.OnUnpackStreamLogger
	muxer              *Muxer
	file               *os.File
	tagData            []byte
	firstOfVideoPacket bool
}

func (s *RemuxHandler) OnNewTrack(stream avformat.Track) {
	s.OnUnpackStreamLogger.OnNewTrack(stream)

	_, err := s.muxer.AddTrack(stream.GetStream())
	if err != nil {
		panic(err)
	}
}

func (s *RemuxHandler) OnTrackComplete() {
	s.OnUnpackStreamLogger.OnTrackComplete()

	n := s.muxer.WriteHeader(s.tagData)
	_, err := s.file.Write(s.tagData[:n])
	if err != nil {
		panic(err)
	}
}

func (s *RemuxHandler) OnPacket(packet *avformat.AVPacket) {
	s.OnUnpackStreamLogger.OnPacket(packet)

	track := s.muxer.Tracks.Get(packet.Index)
	if s.firstOfVideoPacket {
		if track.GetStream().Colors != nil {
			n := s.muxer.Input(s.tagData, packet.MediaType, len(track.GetStream().Colors), packet.Dts, packet.Pts, false, FrameTypeVideoInfoCommand)
			_, err := s.file.Write(s.tagData[:n])
			_, err = s.file.Write(track.GetStream().Colors)
			if err != nil {
				panic(err)
			}

		}
	}

	pktType := FrameTypeInterFrame
	if packet.Key {
		pktType = FrameTypeKeyFrame
	}

	n := s.muxer.Input(s.tagData, packet.MediaType, len(packet.Data), packet.Dts, packet.Pts, false, pktType)
	_, err := s.file.Write(s.tagData[:n])
	if err != nil {
		panic(err)
	}

	_, err = s.file.Write(packet.Data)
	if err != nil {
		panic(err)
	}
}

func TestDeMuxer(t *testing.T) {
	files := []string{
		//"live.flv",
		//"out.flv",
		//"生死罗布泊--超长片花.flv",
		//"daniulivetestflv.flv",
		//"20190108154338062.flv",
		//"h265.flv",
		//"vp6_mp3.flv",
		//"4K_60fps_VP9_OPUS_HDR.flv",
		//"av1.flv",
		//"14-49-02.flv",
		"live.flv",
	}

	getSourceFilePath := func(file string) string {
		return "../source_files/" + file
	}

	unpack := func(file string, handler avformat.OnUnpackStreamHandler) {
		demuxer := NewDemuxer(true)
		demuxer.SetHandler(handler)

		data, err := os.ReadFile(getSourceFilePath(file))
		if err != nil {
			panic(err)
		}

		_, err = demuxer.Input(data)
		if err != nil {
			panic(err)
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
			outfile, err := os.OpenFile(getSourceFilePath(file)+".muxer.flv", os.O_WRONLY|os.O_CREATE, 132)
			if err != nil {
				panic(err)
			}

			handler := &RemuxHandler{
				tagData: make([]byte, 2048),
			}
			handler.muxer = NewMuxer(nil)
			handler.file = outfile
			unpack(file, handler)
			handler.file.Close()
		}
	})
}
