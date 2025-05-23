package transport

import (
	"context"
	"net"
)

const (
	DefaultTCPRecvBufferSize = 4096
)

type Transport interface {
	// Bind binds the transport to a specific address.
	Bind(addr net.Addr) error

	Close()

	SetHandler(handler Handler)

	SetHandler2(onConnected func(conn net.Conn) []byte,
		onPacket func(conn net.Conn, data []byte) []byte,
		onDisConnected func(conn net.Conn, err error))

	// ListenIP returns the IP address the transport is listening on.
	ListenIP() string

	// ListenPort returns the port the transport is listening on.
	ListenPort() int
}

type transport struct {
	handler Handler
	ctx     context.Context
	cancel  context.CancelFunc

	listenIP   string
	listenPort int
}

func (t *transport) Bind(addr net.Addr) error {
	return nil
}

func (t *transport) setListenAddr(addr net.Addr) {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		t.listenIP = tcpAddr.IP.String()
		t.listenPort = tcpAddr.Port
	} else if udpAddr, ok := addr.(*net.UDPAddr); ok {
		t.listenIP = udpAddr.IP.String()
		t.listenPort = udpAddr.Port
	} else {
		panic(addr)
	}
}

func (t *transport) SetHandler(handler Handler) {
	t.handler = handler
}

func (t *transport) SetHandler2(onConnected func(conn net.Conn) []byte, onPacket func(conn net.Conn, data []byte) []byte, onDisConnected func(conn net.Conn, err error)) {
	Assert(t.handler == nil)
	t.SetHandler(&handler{
		onConnected,
		onPacket,
		onDisConnected,
	})
}

func (t *transport) Close() {
	t.handler = nil
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
}

func (t *transport) ListenPort() int {
	return t.listenPort
}

func (t *transport) ListenIP() string {
	return t.listenIP
}
