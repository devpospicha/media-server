package transport

import (
	"math"
	"runtime"
	"syscall"
)

type ReuseServer struct {
	transport
	ConcurrentNumber int
	EnableReuse      bool
}

func (r *ReuseServer) GetSetOptFunc() func(network, address string, c syscall.RawConn) error {
	if r.ComputeConcurrentNumber() > 1 {
		return SetReuseOpt
	}

	return nil
}

func (r *ReuseServer) ComputeConcurrentNumber() int {
	// macos或未开启端口复用, 只使用一个协程监听端口
	if runtime.GOOS == "darwin" || !r.EnableReuse {
		r.ConcurrentNumber = 1
	}

	r.ConcurrentNumber = int(math.Max(float64(r.ConcurrentNumber), 1))
	return r.ConcurrentNumber
}
