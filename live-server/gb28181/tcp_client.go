package gb28181

import (
	"net"

	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/transport"
)

// TCPClient GB28181TCP主动收流
type TCPClient struct {
	TCPServer
}

func NewTCPClient(listenPort int, remoteAddr *net.TCPAddr, source GBSource) (*TCPClient, error) {
	client := &TCPClient{
		TCPServer{filter: NewSingleFilter(source)},
	}
	tcp := transport.TCPClient{}
	tcp.SetHandler(client)

	addr, err := net.ResolveTCPAddr("tcp", stream.ListenAddr(listenPort))
	if err != nil {
		return client, err
	}

	_, err = tcp.Connect(addr, remoteAddr)
	return client, err
}
