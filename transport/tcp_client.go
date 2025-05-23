package transport

import (
	"context"
	"net"
)

type TCPClient struct {
	transport
	conn net.Conn
}

func (t *TCPClient) Connect(local, remote *net.TCPAddr) (net.Conn, error) {
	dialer := net.Dialer{
		LocalAddr: local,
	}

	conn, err := dialer.Dial("tcp", remote.String())
	if err != nil {
		t.Close()
		return nil, err
	}

	t.conn = NewConn(conn)
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.setListenAddr(conn.LocalAddr())
	return t.conn, nil
}

func (t *TCPClient) Receive() {
	Assert(t.handler != nil)
	Assert(t.conn != nil)

	readDataFromTCPConn(t.ctx, t.conn, t.handler)
}

func (t *TCPClient) Write(data []byte) error {
	_, err := t.conn.Write(data)
	return err
}

func (t *TCPClient) Close() {
	if t.conn != nil {
		t.conn.Close()
		t.conn = nil
	}

	t.transport.Close()
}
