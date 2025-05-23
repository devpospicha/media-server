package rtmp

import (
	"net"
	"testing"

	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/rtmp"
)

func CreateTransStream(source stream.Source, protocol stream.TransStreamProtocol, streams []*avformat.AVStream) stream.TransStream {
	if stream.TransStreamRtmp == protocol {
		return NewTransStream(rtmp.MaxChunkSize, nil)
	}

	return nil
}

func init() {
	//stream.TransStreamFactory = CreateTransStream
}

func TestServer(t *testing.T) {
	stream.AppConfig.GOPCache = true
	stream.AppConfig.MergeWriteLatency = 350
	impl := server{}
	addr := "0.0.0.0:1935"
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}

	err = impl.Start(tcpAddr)
	if err != nil {
		panic(err)
	}

	println("启动rtmp服务成功:" + addr)
	select {}
}
