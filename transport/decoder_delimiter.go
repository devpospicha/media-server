package transport

import (
	"fmt"
)

// DelimiterFrameDecoder 分隔符解码器
type DelimiterFrameDecoder struct {
	DecoderBuffer

	maxFrameLength  int    // 最大帧长度
	delimiter       []byte // 分隔符
	delimiterLength int    // 分隔符长度
	foundCount      int    // 已经匹配到的分割符数量
}

func (d *DelimiterFrameDecoder) Input(data []byte) (int, []byte, error) {
	var bytes []byte
	var i int
	var v byte

	for i, v = range data {
		i++

		// 匹配分隔符
		if d.delimiter[d.foundCount] != v {
			d.foundCount = 0
			continue
		} else if d.foundCount++; d.foundCount < d.delimiterLength {
			continue
		}

		// 分隔符开始位置
		n := i - d.delimiterLength
		// 回调缓存数据
		if d.size > 0 {
			// 拷贝本次读取的数据
			if n > 0 {
				if d.maxFrameLength < d.size+n {
					return i, nil, fmt.Errorf("frame length exceeds %d", d.maxFrameLength)
				}

				copy(d.data[d.size:], data[:n])
			}

			d.size += n
			bytes = d.data[:d.size]
		} else /* if n > 0 */ {
			// 免拷贝回调当前包数据
			bytes = data[:n]
		}

		break
	}

	if bytes != nil {
		d.size = 0
		d.foundCount = 0

		if 0 != len(bytes) {
			return i, bytes, nil
		} else if i < len(data) {
			// 处理空字符串的情况
			n, bytes, err := d.Input(data[i:])
			if err != nil {
				return -1, nil, err
			}

			return i + n, bytes, nil
		}
	} else {
		// 未匹配到分隔符, 缓存数据
		length := len(data)
		if d.maxFrameLength < d.size+length {
			return -1, nil, fmt.Errorf("frame length exceeds %d", d.maxFrameLength)
		}

		if d.data == nil {
			d.data = make([]byte, d.maxFrameLength)
		}

		copy(d.data[d.size:], data)
		d.size += length

		return length, nil, nil
	}

	return i, nil, nil
}

// NewDelimiterFrameDecoder 创建分隔符解码器
// @maxFrameLength 最大帧长, 如果在maxFrameLength范围内没解析完, 解析失败
func NewDelimiterFrameDecoder(maxFrameLength int, delimiter []byte) *DelimiterFrameDecoder {
	Assert(maxFrameLength > len(delimiter))
	return &DelimiterFrameDecoder{
		delimiter:       delimiter,
		delimiterLength: len(delimiter),
		maxFrameLength:  maxFrameLength,
	}
}
