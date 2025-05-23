package transport

import (
	"context"
	"net"
)

type TCPServer struct {
	ReuseServer
	listeners []*net.TCPListener
}

func (t *TCPServer) Bind(addr net.Addr) error {
	Assert(t.listeners == nil)

	// 监听地址为nil, 系统分配端口
	random := addr == nil
	if random {
		t.ConcurrentNumber = 1
		addr, _ = net.ResolveTCPAddr("tcp", ":0")
	}

	config := net.ListenConfig{
		Control: t.GetSetOptFunc(),
	}

	t.ctx, t.cancel = context.WithCancel(context.Background())
	number := t.ComputeConcurrentNumber()
	for i := 0; i < number; i++ {

		listen, err := config.Listen(t.ctx, "tcp", addr.String())
		if err != nil {
			t.Close()
			return err
		}

		t.listeners = append(t.listeners, listen.(*net.TCPListener))

		if random {
			t.setListenAddr(listen.Addr())
		} else {
			t.setListenAddr(addr)
		}
	}

	return nil
}

func (t *TCPServer) Accept() {
	Assert(t.handler != nil)
	Assert(len(t.listeners) > 0)

	for _, listener := range t.listeners {
		go t.doAccept(listener)
	}
}

func (t *TCPServer) doAccept(listener *net.TCPListener) {
	for t.ctx.Err() == nil {
		conn, err := listener.AcceptTCP()
		if err != nil {
			println(err.Error())
			continue
		}

		go readDataFromTCPConn(t.ctx, NewConn(conn), t.handler)
	}
}

func (t *TCPServer) Close() {
	for _, listener := range t.listeners {
		_ = listener.Close()
	}

	t.listeners = nil
	t.transport.Close()
}

func readDataFromTCPConn(ctx context.Context, conn net.Conn, handler Handler) {
	var receiveBuffer []byte
	if handler != nil {
		receiveBuffer = handler.OnConnected(conn)
	}

	if receiveBuffer == nil {
		receiveBuffer = make([]byte, DefaultTCPRecvBufferSize)
	}

	var n int
	var err error
	var buffer []byte
	for ctx.Err() == nil {
		if buffer == nil {
			buffer = receiveBuffer
		}

		n, err = conn.Read(buffer)
		if err != nil {
			if err.Error() != "EOF" {
				println(err.Error())
			}
			break
		}

		if n > 0 && handler != nil {
			buffer = handler.OnPacket(conn, buffer[:n])
		}
	}

	_ = conn.Close()

	if handler != nil {
		handler.OnDisConnected(conn, err)
	}
}
