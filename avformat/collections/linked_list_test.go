package collections

import (
	"testing"

	"github.com/devpospicha/media-server/avformat/utils"
)

func TestLinkedList(t *testing.T) {
	l := LinkedList[int]{}

	for i := 0; i < 100; i++ {
		l.Add(i)
	}

	for i := 0; i < 100; i++ {
		utils.Assert(l.Get(i) == i)
	}

	for i := 0; i < 100; i++ {
		utils.Assert(l.Remove(0) == i)
	}

	utils.Assert(l.Size() == 0)
}
