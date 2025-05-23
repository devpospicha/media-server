package rtmp

import (
	"net"

	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/rtmp"
)

type Sink struct {
	stream.BaseSink
	stack *rtmp.ServerStack
}

func (s *Sink) StartStreaming(_ stream.TransStream) error {
	return s.stack.SendStreamBeginChunk(s.Conn)
}

func (s *Sink) StopStreaming(stream stream.TransStream) {
	_ = s.stack.SendStreamEOFChunk(s.Conn)
	s.BaseSink.StopStreaming(stream)
}

func (s *Sink) Close() {
	s.BaseSink.Close()
	s.stack = nil
}

func NewSink(id stream.SinkID, sourceId string, conn net.Conn, stack *rtmp.ServerStack) stream.Sink {
	return &Sink{
		BaseSink: stream.BaseSink{ID: id, SourceID: sourceId, State: stream.SessionStateCreated, Protocol: stream.TransStreamRtmp, Conn: conn, DesiredAudioCodecId_: utils.AVCodecIdNONE, DesiredVideoCodecId_: utils.AVCodecIdNONE, TCPStreaming: true},
		stack:    stack,
	}
}
