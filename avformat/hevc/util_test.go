package hevc

import (
	"fmt"
	"os"
	"testing"

	"github.com/devpospicha/media-server/avformat/avc"
)

func TestUtil(t *testing.T) {
	file, err := os.ReadFile("../h265.hevc")
	if err != nil {
		panic(err)
	}

	var index int
	var lastKeyFrameIndex = -1
	avc.SplitNalU(file, func(nalu []byte) {
		index++
		if IsKeyFrame(nalu) {
			if lastKeyFrameIndex != -1 {
				println(fmt.Sprintf("关键帧间隔:%d", index-lastKeyFrameIndex))
			}
			lastKeyFrameIndex = index
		}
	})
}
