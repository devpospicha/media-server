package transport

import (
	"fmt"
)

type DecoderBuffer struct {
	data []byte // 解析缓冲区
	size int    // 缓存区大小
}

// LengthFieldFrameDecoder 帧长解码器
type LengthFieldFrameDecoder struct {
	DecoderBuffer

	maxFrameLength   int // 最大帧长
	frameLength      int // 当前帧长
	fieldLength      int // 几个字节描述帧长
	fieldLengthCount int // 已经读取到几个字节帧长
}

func (d *LengthFieldFrameDecoder) Input(data []byte) (int, []byte, error) {
	var index int
	length := len(data)
	var bytes []byte

	for index < length {
		// 读取帧长度
		for ; d.fieldLengthCount < d.fieldLength && index < length; index++ {
			d.frameLength = d.frameLength<<8 | int(data[index])
			d.fieldLengthCount++
		}

		// 不够帧长
		if d.fieldLengthCount < d.fieldLength {
			break
		}

		if d.frameLength > d.maxFrameLength {
			return index, nil, fmt.Errorf("frame length exceeds %d", d.maxFrameLength)
		}

		// 剩余数据长度
		n := length - index

		// 有缓存数据或者数据不够,缓存起来
		if d.size > 0 || n < d.frameLength {
			if d.data == nil {
				d.data = make([]byte, d.maxFrameLength)
			}

			consume := MinInt(d.frameLength-d.size, n)

			copy(d.data[d.size:], data[index:index+consume])
			d.size += consume
			index += consume
		}

		if d.size >= d.frameLength {
			// 回调缓存数据
			bytes = d.data[:d.frameLength]
			break
		} else if n >= d.frameLength {
			// 免拷贝回调
			index += d.frameLength
			bytes = data[index-d.frameLength : index]
			break
		}
	}

	// 清空标记
	if bytes != nil {
		d.size = 0
		d.frameLength = 0
		d.fieldLengthCount = 0
	}

	return index, bytes, nil
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}

// NewLengthFieldFrameDecoder 创建帧长解码器
// @maxFrameLength 最大帧长, 如果在maxFrameLength范围内没解析完, 解析失败
// @fieldLength 几个字节描述帧长
func NewLengthFieldFrameDecoder(maxFrameLength, fieldLength int) *LengthFieldFrameDecoder {
	Assert(maxFrameLength > 0)
	Assert(fieldLength > 0 && fieldLength < 5)

	return &LengthFieldFrameDecoder{
		maxFrameLength: maxFrameLength, fieldLength: fieldLength}
}
