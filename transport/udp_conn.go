package transport

import "net"

// UDPConn 使用net.Conn接口封装UDP连接句柄, 操作函数与TCPConn保持一致 /**
type UDPConn struct {
	net.PacketConn
	local  net.Addr
	remote net.Addr
}

func (c *UDPConn) Read(b []byte) (int, error) {
	if n, _, err := c.ReadFrom(b); err != nil {
		return n, err
	} else {
		return n, err
	}
}

func (c *UDPConn) Write(b []byte) (int, error) {
	return c.WriteTo(b, c.remote)
}

func (c *UDPConn) LocalAddr() net.Addr {
	if c.local == nil {
		return c.PacketConn.LocalAddr()
	}
	return c.local
}

func (c *UDPConn) Close() error {
	return nil
}

func (c *UDPConn) RemoteAddr() net.Addr {
	return c.remote
}
