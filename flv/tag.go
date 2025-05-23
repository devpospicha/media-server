package flv

import (
	"encoding/binary"

	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
)

type TagType int

const (
	TagTypeAudioData  = TagType(8)
	TagTypeVideoData  = TagType(9)
	TagTypeScriptData = TagType(18)
)

type Tag struct {
	PrevTagSize uint32
	Type        TagType
	DataSize    int
	Timestamp   uint32
	StreamID    int
}

func UnmarshalTag(data []byte) Tag {
	timestamp := bufio.Uint24(data[8:])
	timestamp |= uint32(data[11]) << 24

	return Tag{
		PrevTagSize: binary.BigEndian.Uint32(data),
		Type:        TagType(data[4] & 0x1F),
		DataSize:    int(bufio.Uint24(data[5:])),
		Timestamp:   timestamp,
	}
}

func TagType2AVMediaType(tag TagType) utils.AVMediaType {
	if TagTypeAudioData == tag {
		return utils.AVMediaTypeAudio
	} else if TagTypeVideoData == tag {
		return utils.AVMediaTypeVideo
	} else if TagTypeScriptData == tag {
		return utils.AVMediaTypeData
	}

	return utils.AVMediaTypeVideo
}
