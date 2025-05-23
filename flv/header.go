package flv

import (
	"encoding/binary"
	"fmt"

	"github.com/devpospicha/media-server/avformat/bufio"
)

var (
	Signature = bufio.Uint24([]byte("FLV"))
)

type TypeFlag byte

func (t *TypeFlag) ExistAudio() bool {
	return *t>>2&0x1 == 1
}
func (t *TypeFlag) ExistVideo() bool {
	return *t&0x1 == 1
}

func (t *TypeFlag) Marshal(audio, video bool) {
	var flag byte
	if audio {
		flag = 1 << 2
	}

	if video {
		flag |= 1
	}

	*t = TypeFlag(flag)
}

type Header byte

func UnmarshalHeader(data []byte) (*TypeFlag, error) {
	if signature := bufio.Uint24(data); Signature != signature {
		return nil, fmt.Errorf("unknow signature %x", signature)
	} else if 0x1 != data[3] {
		return nil, fmt.Errorf("only supports FLV version 1")
	} else if 0x9 != binary.BigEndian.Uint32(data[5:]) {
		return nil, fmt.Errorf("only supports FLV version 1")
	}

	flag := TypeFlag(data[4])
	return &flag, nil
}
func MarshalHeader(data []byte, audio, video bool) int {
	_ = data[8]
	bufio.PutUint24(data, Signature)
	data[3] = 0x1

	flag := TypeFlag(0)
	flag.Marshal(audio, video)
	data[4] = byte(flag)

	binary.BigEndian.PutUint32(data[5:], 0x9)
	return 9
}
