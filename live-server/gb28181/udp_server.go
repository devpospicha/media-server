package gb28181

import (
	"net"
	"runtime"

	"github.com/devpospicha/media-server/live-server/log"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/transport"
	"github.com/pion/rtp"
)

// UDPServer GB28181UDP收流
type UDPServer struct {
	stream.StreamServer[*UDPSource]
	udp    *transport.UDPServer
	filter Filter
}

func (U *UDPServer) OnNewSession(conn net.Conn) *UDPSource {
	return nil
}

func (U *UDPServer) OnCloseSession(session *UDPSource) {
	U.filter.RemoveSource(session.SSRC())
	session.Close()

	if stream.AppConfig.GB28181.IsMultiPort() {
		U.udp.Close()
		U.Handler = nil
	}
}

func (U *UDPServer) OnPacket(conn net.Conn, data []byte) []byte {
	U.StreamServer.OnPacket(conn, data)

	packet := rtp.Packet{}
	err := packet.Unmarshal(data)
	if err != nil {
		log.Sugar.Errorf("解析rtp失败 err:%s conn:%s", err.Error(), conn.RemoteAddr().String())
		return nil
	}

	source := U.filter.FindSource(packet.SSRC)
	if source == nil {
		log.Sugar.Errorf("ssrc匹配source失败 ssrc:%x conn:%s", packet.SSRC, conn.RemoteAddr().String())
		return nil
	}

	if stream.SessionStateHandshakeSuccess == source.State() {
		conn.(*transport.Conn).Data = source
		source.PreparePublish(conn, packet.SSRC, source)
	}

	packet.Raw = data
	source.(*UDPSource).InputRtpPacket(&packet)
	return nil
}

func NewUDPServer(filter Filter) (*UDPServer, error) {
	server := &UDPServer{
		filter: filter,
	}

	var udp *transport.UDPServer
	var err error
	if stream.AppConfig.GB28181.IsMultiPort() {
		udp, err = TransportManger.NewUDPServer()
		if err != nil {
			return nil, err
		}
	} else {
		udp = &transport.UDPServer{
			ReuseServer: transport.ReuseServer{
				EnableReuse:      true,
				ConcurrentNumber: runtime.NumCPU(),
			},
		}

		var gbAddr *net.UDPAddr
		gbAddr, err = net.ResolveUDPAddr("udp", stream.ListenAddr(stream.AppConfig.GB28181.Port[0]))
		if err != nil {
			return nil, err
		}

		if err = udp.Bind(gbAddr); err != nil {
			return server, err
		}
	}

	udp.SetHandler(server)
	udp.Receive()
	server.udp = udp
	server.StreamServer = stream.StreamServer[*UDPSource]{
		SourceType: stream.SourceType28181,
		Handler:    server,
	}
	return server, nil
}
