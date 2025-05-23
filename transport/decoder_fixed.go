package transport

// FixedLengthFrameDecoder 固定长度解码器
type FixedLengthFrameDecoder struct {
	DecoderBuffer
	frameLength int
}

func (d *FixedLengthFrameDecoder) Input(data []byte) (int, []byte) {
	var bytes []byte
	var n int
	length := len(data)

	if d.size > 0 {
		n = MinInt(d.frameLength-d.size, length)
		copy(d.data[d.size:], data[:n])

		d.size += n
		if d.size == d.frameLength {
			bytes = d.data
		}
	} else if length < d.frameLength {
		if d.data == nil {
			d.data = make([]byte, d.frameLength)
		}

		copy(d.data, data)
		d.size = length
		n = length
	} else {
		bytes = data[:d.frameLength]
		n = d.frameLength
	}

	if bytes != nil {
		d.size = 0
	}

	return n, bytes
}

func NewFixedLengthFrameDecoder(frameLength int) *FixedLengthFrameDecoder {
	Assert(frameLength > 0)

	return &FixedLengthFrameDecoder{
		frameLength: frameLength,
	}
}
