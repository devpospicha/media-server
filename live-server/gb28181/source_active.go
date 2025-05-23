package gb28181

import (
	"net"

	"github.com/devpospicha/media-server/transport"
)

type ActiveSource struct {
	PassiveSource

	port       int
	remoteAddr net.TCPAddr
	tcp        *TCPClient
}

func (a *ActiveSource) Connect(remoteAddr *net.TCPAddr) error {
	client, err := NewTCPClient(a.port, remoteAddr, a)
	if err != nil {
		return err
	}

	a.tcp = client
	return nil
}

func (a *ActiveSource) SetupType() SetupType {
	return SetupActive
}

func NewActiveSource() (*ActiveSource, int, error) {
	var port int
	TransportManger.AllocPort(true, func(port_ uint16) error {
		port = int(port_)
		return nil
	})

	return &ActiveSource{
		PassiveSource: PassiveSource{
			decoder: transport.NewLengthFieldFrameDecoder(0xFFFF, 2),
		},
		port: port,
	}, port, nil
}
