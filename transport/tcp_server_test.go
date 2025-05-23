package transport

import (
	"fmt"
	"net"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

type TCPServerHandler struct {
}

func (T *TCPServerHandler) OnConnected(conn net.Conn) []byte {
	println("客户端连接: " + conn.RemoteAddr().String())

	go func() {
		bytes := make([]byte, 1024*1024*1)
		for {
			milli := time.Now().UnixMilli()
			conn.Write(bytes)
			fmt.Printf("发送耗时:%d\r\n"+
				""+
				""+
				""+
				""+
				""+
				"", time.Now().UnixMilli()-milli)

		}
	}()

	return nil
}

func (T *TCPServerHandler) OnPacket(conn net.Conn, data []byte) []byte {
	if _, err := conn.Write(data); err != nil {
		panic(err)
	}
	return nil
}

func (T *TCPServerHandler) OnDisConnected(conn net.Conn, err error) {
	println("客户端断开连接: " + conn.RemoteAddr().String())
}

func TestTCPServer(t *testing.T) {
	t.Run("reuse", func(t *testing.T) {
		server := TCPServer{
			ReuseServer: ReuseServer{
				EnableReuse:      true,
				ConcurrentNumber: runtime.NumCPU(),
			},
		}

		server.SetHandler(&TCPServerHandler{})
		addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:8000")

		if err := server.Bind(addr); err != nil {
			panic(err)
		}

		server.Accept()
		println("成功监听:" + addr.String())
	})

	t.Run("weak", func(t *testing.T) {
		// tcp server 间隔发送
		// tcp client 缓慢读取
		server := TCPServer{}
		server.SetHandler(&TCPServerHandler{})
		addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:8000")

		if err := server.Bind(addr); err != nil {
			panic(err)
		}

		server.Accept()
		println("成功监听:" + addr.String())

		conn, err2 := net.DialTCP("tcp", nil, addr)
		if err2 != nil {
			panic(err2)
		}

		go func() {
			bytes := make([]byte, 1024)
			for {
				_, err := conn.Read(bytes)
				if err != nil {
					panic(err)
				}

				time.Sleep(1 * time.Millisecond)
			}
		}()

		defer conn.Close()

		select {}
	})
}
