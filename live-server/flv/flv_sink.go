package flv

import (
	"net"

	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/transport"
)

type Sink struct {
	stream.BaseSink
	prevTagSize uint32
}

func (s *Sink) StopStreaming(stream stream.TransStream) {
	s.BaseSink.StopStreaming(stream)
	s.prevTagSize = stream.(*TransStream).Muxer.PrevTagSize()
}

func (s *Sink) Write(index int, data []*collections.ReferenceCounter[[]byte], ts int64, keyVideo bool) error {
	//	log.Print(" flv_sink.go -> Write")

	if s.prevTagSize > 0 {
		data = data[1:]
		s.prevTagSize = 0
	}
	return s.BaseSink.Write(index, data, ts, keyVideo)
}

func NewFLVSink(id stream.SinkID, sourceId string, conn net.Conn) stream.Sink {
	return &Sink{
		BaseSink: stream.BaseSink{
			ID:           id,
			SourceID:     sourceId,
			State:        stream.SessionStateCreated,
			Protocol:     stream.TransStreamFlv,
			Conn:         transport.NewConn(conn),
			TCPStreaming: true,
		},
	}
}
