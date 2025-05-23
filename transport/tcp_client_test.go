package transport

import (
	"net"
	"testing"
	"time"
)

func TestTCPClient(t *testing.T) {
	serverAddr := "192.168.2.119:20000"
	addr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		panic(err)
	}

	client := TCPClient{}
	client.SetHandler2(func(conn net.Conn) []byte {
		println("Client:" + conn.LocalAddr().String() + " 链接成功")
		conn.Write([]byte("hello world!"))
		return nil
	}, func(conn net.Conn, data []byte) []byte {
		println("Client:" + conn.LocalAddr().String() + " 收到数据:" + string(data))
		return nil
	}, func(conn net.Conn, err error) {
		println("Client:" + conn.LocalAddr().String() + " 断开链接")
	})

	conn, err := client.Connect(nil, addr)
	if err != nil {
		panic(err)
	}

	go client.Receive()

	for {
		_, err := conn.Write([]byte("hello world!"))
		if err != nil {
			panic(err)
		}

		time.Sleep(3 * time.Second)

		break
	}

	client.Close()

	select {}
}
