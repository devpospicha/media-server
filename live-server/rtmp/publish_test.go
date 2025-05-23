package rtmp

import (
	"encoding/binary"
	"net"
	"os"
	"testing"
	"time"

	"github.com/devpospicha/media-server/transport"
)

func TestName(t *testing.T) {
	path := "../dump/rtmp-127.0.0.1.6850"
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:1935")

	file, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	client := transport.TCPClient{}
	if _, err := client.Connect(nil, addr); err != nil {
		panic(err)
	}

	length := len(file)
	for i := 0; i < length; {
		size := int(binary.BigEndian.Uint32(file[i:]))
		if length-i < size {
			return
		}

		i += 4
		i += size
		client.Write(file[i-size : i])

		time.Sleep(10 * time.Millisecond)
	}

}
