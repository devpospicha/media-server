package flv

import (
	"net"
	"time"

	"github.com/devpospicha/media-server/transport"
	"github.com/gorilla/websocket"
)

type WSConn struct {
	*websocket.Conn
}

func (w WSConn) Read(b []byte) (n int, err error) {
	panic("implement me")
}

func (w WSConn) Write(block []byte) (n int, err error) {
	// ws-flv负载的是flv tag
	return 0, w.WriteMessage(websocket.BinaryMessage, GetFLVTag(block))
}

func (w WSConn) SetDeadline(t time.Time) error {
	panic("implement me")
}

func NewWSConn(conn *websocket.Conn) net.Conn {
	return transport.NewConn(&WSConn{conn})
}
