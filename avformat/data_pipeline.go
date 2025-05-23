package avformat

import "github.com/devpospicha/media-server/avformat/utils"

type DataPipeline interface {
	Write(data []byte, index int, mediaType utils.AVMediaType) (int, error)

	Feat(index int) ([]byte, error)

	DiscardBackPacket(index int)

	DiscardHeadPacket(index int)

	PendingBlockSize(index int) int
}
