package mpeg

import "fmt"

type PSProbeBuffer struct {
	buffer   []byte
	offset   int
	capacity int
	tmp      []byte
}

func (p *PSProbeBuffer) Input(data []byte) ([]byte, error) {
	length := len(data)
	size := p.offset + length

	var tmp = data
	if size > p.capacity {
		return nil, fmt.Errorf("probe pesHeaderData overflow: current length %d exceeds capacity %d", size, p.capacity)
	} else if size != length {
		// 拷贝到缓冲区尾部
		copy(p.buffer[p.offset:], data)
		tmp = p.buffer[:size]
	}

	p.offset = size
	p.tmp = tmp
	return tmp, nil
}

func (p *PSProbeBuffer) Reset(n int) {
	if n > -1 {
		p.offset -= n

		// 拷贝未解析完的剩余数据到缓冲区头部
		if p.offset > 0 {
			copy(p.buffer, p.tmp[n:])
		}
	}
}

func NewProbeBuffer(capacity int) *PSProbeBuffer {
	return &PSProbeBuffer{
		buffer:   make([]byte, capacity),
		offset:   0,
		capacity: capacity,
	}
}
