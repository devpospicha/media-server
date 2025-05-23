package collections

import (
	"testing"

	"github.com/devpospicha/media-server/avformat/utils"
)

func TestReferenceCounter(t *testing.T) {
	r := ReferenceCounter[int]{}
	r.Refer()
	utils.Assert(r.UseCount() == 1)
	utils.Assert(r.Release())
}
