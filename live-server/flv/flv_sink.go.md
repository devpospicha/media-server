package flv

import (
	"fmt"
	"net"
	"net/http"

	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/live-server/log"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/transport"
)

type Sink struct {
	stream.BaseSink
	prevTagSize uint32
	closeChan   chan struct{}
	Stream      interface{}
}

func (s *Sink) StopStreaming(stream stream.TransStream) error {
	s.BaseSink.StopStreaming(stream)
	ts, ok := stream.(*TransStream)
	if !ok {
		return fmt.Errorf("expected *TransStream, got %T", stream)
	}
	s.prevTagSize = ts.Muxer.PrevTagSize()
	return nil
}

func (s *Sink) Write(index int, data []*collections.ReferenceCounter[[]byte], ts int64, keyVideo bool) error {
	if s.Stream == nil {
		log.Sugar.Warn("No stream available in Write, skipping sequence headers")
	}
	if s.prevTagSize == 0 {
		ts = 0
	}
	if keyVideo && s.Stream != nil {
		seqHeaders := s.getSequenceHeaders()
		if len(seqHeaders) > 0 {
			log.Sugar.Debugf("Writing sequence headers, size: %d", len(seqHeaders))
			_, err := s.Conn.Write(seqHeaders)
			if err != nil {
				log.Sugar.Errorf("Failed to write sequence headers: %s", err.Error())
				return err
			}
			if f, ok := s.Conn.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
	if s.prevTagSize > 0 {
		data = data[1:]
		s.prevTagSize = 0
	}
	return s.BaseSink.Write(index, data, ts, keyVideo)
}

func (s *Sink) CloseChannel() <-chan struct{} {
	return s.closeChan
}

func (s *Sink) Close() {
	close(s.closeChan)
	s.Stream = nil
	s.Conn.Close()
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
		prevTagSize: 0,
		closeChan:   make(chan struct{}),
	}
}
