package transport

import "net"

// Handler 传输事件处理器，负责连接传输和断开连接的回调
type Handler interface {
	// OnConnected conn连接回调, 返回收流缓冲区, 下次将使用该缓冲区从网络读取数据
	OnConnected(conn net.Conn) []byte

	// OnPacket conn读取的数据回调, 返回收流缓冲区, 下次将使用该缓冲区从网络读取数据
	OnPacket(conn net.Conn, data []byte) []byte

	// OnDisConnected conn断开连接回调
	OnDisConnected(conn net.Conn, err error)
}

type handler struct {
	onConnected    func(conn net.Conn) []byte
	onPacket       func(conn net.Conn, data []byte) []byte
	onDisConnected func(conn net.Conn, err error)
}

func (h *handler) OnConnected(conn net.Conn) []byte {
	if h.onConnected != nil {
		return h.onConnected(conn)
	}

	return nil
}

func (h *handler) OnPacket(conn net.Conn, data []byte) []byte {
	if h.onPacket != nil {
		return h.onPacket(conn, data)
	}

	return nil
}

func (h *handler) OnDisConnected(conn net.Conn, err error) {
	if h.onDisConnected != nil {
		h.onDisConnected(conn, err)
	}
}
