package transport

import (
	"encoding/hex"
	"testing"
)

func TestLengthFieldFrameDecoder_Input(t *testing.T) {
	decoder := NewLengthFieldFrameDecoder(0xFFFF, 2)
	decoder.Input([]byte{0x00, 0x64})

	for i := 0; i < 100; i++ {
		n, bytes, err := decoder.Input([]byte{byte(i)})
		Assert(n == 1)

		if err != nil {
			panic(err)
		} else if bytes != nil {
			println(hex.EncodeToString(bytes))
		}
	}
}

func TestFixedLengthFrameDecoder_Input(t *testing.T) {
	decoder := NewFixedLengthFrameDecoder(10)
	for i := 0; i < 100; i++ {
		n, bytes := decoder.Input([]byte{byte(i)})
		Assert(n == 1)

		if bytes != nil {
			println(hex.EncodeToString(bytes))
		}
	}
}

func TestNewDelimiterFrameDecoder(t *testing.T) {
	data := "abc123456abc789abchello worldabctest"

	t.Run("split", func(t *testing.T) {
		decoder := NewDelimiterFrameDecoder(1024*1024*2, []byte("abc"))
		var n int
		for {
			i, bytes, err := decoder.Input([]byte(data[n:]))
			if err != nil {
				panic(err)
			}
			n += i
			if bytes != nil {
				println(string(bytes))
			} else {
				break
			}
		}
	})

	t.Run("order", func(t *testing.T) {
		decoder := NewDelimiterFrameDecoder(1024*1024*2, []byte("abc"))

		for i := 0; i < len(data); i++ {
			n, bytes, err := decoder.Input([]byte(data[i : i+1]))
			Assert(n == 1)
			if err != nil {
				panic(err)
			}

			if bytes != nil {
				println(string(bytes))
			}

		}
	})

}
