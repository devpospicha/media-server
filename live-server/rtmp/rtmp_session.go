package rtmp

import (
	"net"

	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/log"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/rtmp"
)

// Session RTMP会话, 解析处理Message
type Session struct {
	conn        net.Conn
	stack       *rtmp.ServerStack // rtmp协议栈, 解析message
	handle      interface{}       // 持有具体会话句柄(推流端/拉流端)， 在@see OnPublish @see OnPlay回调中赋值
	isPublisher bool              // 是否是推流会话
}

func (s *Session) generateSourceID(app, stream string) string {
	if len(app) == 0 {
		return stream
	} else if len(stream) == 0 {
		return app
	} else {
		return app + "/" + stream
	}
}

func (s *Session) OnPublish(app, stream_ string) utils.HookState {
	log.Sugar.Infof("rtmp onpublish app: %s stream: %s conn: %s", app, stream_, s.conn.RemoteAddr().String())

	streamName, values := stream.ParseUrl(stream_)

	sourceId := s.generateSourceID(app, streamName)
	source := NewPublisher(sourceId, s.stack, s.conn)

	// 初始化放在add source前面, 以防add后再init, 空窗期拉流队列空指针.
	source.Init(stream.TCPReceiveBufferQueueSize)
	source.SetUrlValues(values)

	// 统一处理source推流事件, source是否已经存在, hook回调....
	_, state := stream.PreparePublishSource(source, true)
	if utils.HookStateOK != state {
		log.Sugar.Errorf("rtmp推流失败 source: %s", sourceId)
	} else {
		s.handle = source
		s.isPublisher = true

		go stream.LoopEvent(source)
	}

	return state
}

func (s *Session) OnPlay(app, stream_ string) utils.HookState {
	streamName, values := stream.ParseUrl(stream_)

	sourceId := s.generateSourceID(app, streamName)
	sink := NewSink(stream.NetAddr2SinkId(s.conn.RemoteAddr()), sourceId, s.conn, s.stack)
	sink.SetUrlValues(values)

	log.Sugar.Infof("rtmp onplay app: %s stream: %s sink: %v conn: %s", app, stream_, sink.GetID(), s.conn.RemoteAddr().String())

	_, state := stream.PreparePlaySink(sink)
	if utils.HookStateOK != state {
		log.Sugar.Errorf("rtmp拉流失败 source: %s sink: %s", sourceId, sink.GetID())
	} else {
		s.handle = sink
	}

	return state
}

func (s *Session) Input(data []byte) error {
	// 推流会话, 收到的包都将交由主协程处理
	if s.isPublisher {
		return s.handle.(*Publisher).PublishSource.Input(data)
	} else {
		return s.stack.Input(s.conn, data)
	}
}

func (s *Session) Close() {
	// session/conn/stack相互引用, gc回收不了...手动赋值为nil
	s.conn = nil

	defer func() {
		if s.stack != nil {
			s.stack.Close()
			s.stack = nil
		}
	}()

	// 还未确定会话类型, 无需处理
	if s.handle == nil {
		return
	}

	publisher, ok := s.handle.(*Publisher)
	if ok {
		log.Sugar.Infof("rtmp推流结束 %s", publisher.String())

		if s.isPublisher {
			publisher.Close()
		}
	} else {
		sink := s.handle.(*Sink)

		log.Sugar.Infof("rtmp拉流结束 %s", sink.String())
		sink.Close()
	}
}

func NewSession(conn net.Conn) *Session {
	session := &Session{}

	stackServer := rtmp.NewStackServer(false)
	stackServer.SetOnStreamHandler(session)
	session.stack = stackServer
	session.conn = conn
	return session
}
