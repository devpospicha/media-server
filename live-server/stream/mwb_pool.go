package stream

import (
	"fmt"
	"sync"

	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/live-server/log"
)

const (
	BlockBufferSize = 1024 * 1024 * 20 // 2MB block size for write buffering
)

var (
	MWBufferPool = sync.Pool{
		New: func() any {
			log.Sugar.Debug("create new merge writing buffer")
			return &mbBuffer{
				buffer:   collections.NewDirectBlockBuffer(BlockBufferSize),
				segments: collections.NewQueue[*collections.ReferenceCounter[[]byte]](32),
			}
		},
	}

	pendingReleaseBuffers = make(map[string]*collections.Queue[*mbBuffer])
	lock                  sync.Mutex
)

func AddMWBuffersToPending(sourceId string, transStreamId TransStreamID, buffers *collections.Queue[*mbBuffer]) {
	key := fmt.Sprintf("%s-%d", sourceId, transStreamId)

	lock.Lock()
	defer lock.Unlock()

	if oldBuffers, exists := pendingReleaseBuffers[key]; exists {
		log.Sugar.Warnf("force release last pending buffers of %s", key)
		for oldBuffers.Size() > 0 {
			b := oldBuffers.Pop()
			b.buffer.Clear()
			b.segments.Clear()
			MWBufferPool.Put(b)
		}
		delete(pendingReleaseBuffers, key)
	}

	pendingReleaseBuffers[key] = buffers
}

func ReleasePendingBuffers(sourceId string, transStreamId TransStreamID) {
	key := fmt.Sprintf("%s-%d", sourceId, transStreamId)

	lock.Lock()
	defer lock.Unlock()

	buffers, exists := pendingReleaseBuffers[key]
	if !exists || !release(buffers, buffers.Size()) {
		return
	}
	delete(pendingReleaseBuffers, key)
}

func release(buffers *collections.Queue[*mbBuffer], length int) bool {
	log.Sugar.Debug("func release")
	for i := 0; i < length; i++ {
		buffer := buffers.Peek(0)
		size := buffer.segments.Size()

		// Ensure the last reference is not in use before releasing
		if size == 0 || (size > 0 && buffer.segments.Peek(size-1).UseCount() < 2) {
			buffers.Pop()
			buffer.buffer.Clear()
			buffer.segments.Clear()
			MWBufferPool.Put(buffer)
		} else {
			break
		}
	}
	return buffers.Size() == 0
}
