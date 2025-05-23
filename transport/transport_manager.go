package transport

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"sync"
)

type Manager interface {
	AllocPort(tcp bool, cb func(port uint16) error) error

	AllocPairPort(cb, cb2 func(port uint16) error) error

	NewTCPServer() (*TCPServer, error)

	NewUDPServer() (*UDPServer, error)

	NewUDPClient(remoteAddr *net.UDPAddr) (*UDPClient, error)
}

type transportManager struct {
	startPort uint16 // 起始端口
	endPort   uint16 // 结束端口
	nextPort  uint16
	listenIP  string
	lock      sync.Mutex
}

func (t *transportManager) NewTCPServer() (*TCPServer, error) {
	server := TCPServer{}
	err := t.AllocPort(true, func(port uint16) error {
		addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(t.listenIP, strconv.Itoa(int(port))))
		if err != nil {
			panic(err)
		}

		return server.Bind(addr)
	})

	return &server, err
}

func (t *transportManager) NewUDPServer() (*UDPServer, error) {
	server := UDPServer{}
	err := t.AllocPort(false, func(port uint16) error {
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(t.listenIP, strconv.Itoa(int(port))))
		if err != nil {
			panic(err)
		}

		return server.Bind(addr)
	})

	return &server, err
}

func (t *transportManager) NewUDPClient(remoteAddr *net.UDPAddr) (*UDPClient, error) {
	client := UDPClient{}
	err := t.AllocPort(false, func(port uint16) error {
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(t.listenIP, strconv.Itoa(int(port))))
		if err != nil {
			panic(err)
		}

		return client.Connect(addr, remoteAddr)
	})

	return &client, err
}

func (t *transportManager) AllocPort(tcp bool, cb func(port uint16) error) error {
	loop := func(start, end uint16, tcp bool) (uint16, error) {
		for i := start; i < end; i++ {
			if used := Used(int(i), tcp); !used {
				return i, cb(i)
			}
		}

		panic("")
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	_, err := loop(t.nextPort, t.endPort, tcp)
	// 分配端口失败, 第二次尝试从头部开始分配
	if err != nil {
		_, err = loop(t.startPort, t.nextPort, tcp)
	}

	if err != nil {
		return fmt.Errorf("no available ports in the [%d-%d] range", t.startPort, t.endPort)
	}

	t.nextPort = t.nextPort + 1%t.endPort
	t.nextPort = uint16(math.Max(float64(t.nextPort), float64(t.startPort)))
	return nil
}

func (t *transportManager) AllocPairPort(cb func(port uint16) error, cb2 func(port uint16) error) error {
	if err := t.AllocPort(false, cb); err != nil {
		return err
	}

	if err := t.AllocPort(false, cb2); err != nil {
		return err
	}

	return nil
}

// Used 端口是否被占用
func Used(port int, tcp bool) bool {
	if tcp {
		listener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: port})
		if err == nil {
			_ = listener.Close()
		}

		return err != nil
	} else {
		listener, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
		if err == nil {
			_ = listener.Close()
		}

		return err != nil
	}
}

func NewTransportManager(listenIP string, start, end uint16) Manager {
	Assert(end > start)

	return &transportManager{
		startPort: start,
		endPort:   end,
		nextPort:  start,
		listenIP:  listenIP,
	}
}
