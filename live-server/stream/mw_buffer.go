package stream

import (
	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
)

// MergeWritingBuffer 实现针对RTMP/FLV/HLS等基于TCP传输流的合并写缓存
type MergeWritingBuffer interface {
	TryAlloc(size int, ts int64, videoPkt, videoKey bool) ([]byte, bool)

	// TryFlushSegment 尝试生成切片, 如果时长不足, 返回nil
	TryFlushSegment() (*collections.ReferenceCounter[[]byte], bool)

	// FlushSegment 生成并返回当前切片, 以及是否是关键帧切片.
	FlushSegment() (*collections.ReferenceCounter[[]byte], bool)

	// ShouldFlush 当前切片是否已达到生成条件
	ShouldFlush(ts int64) bool

	// IsNewSegment 当前切片是否还未写数据
	IsNewSegment() bool

	// Reserve 从当前切片中预留指定长度数据
	Reserve(length int)

	// ReadSegmentsFromKeyFrameIndex 返回最近的关键帧切片
	ReadSegmentsFromKeyFrameIndex(cb func(*collections.ReferenceCounter[[]byte]))

	HasVideoDataInCurrentSegment() bool

	Close() *collections.Queue[*mbBuffer]
}

type mbBuffer struct {
	buffer   collections.BlockBuffer
	segments *collections.Queue[*collections.ReferenceCounter[[]byte]]
}

type mergeWritingBuffer struct {
	buffers                  *collections.Queue[*mbBuffer]
	lastKeyVideoDataSegments *collections.Queue[*collections.ReferenceCounter[[]byte]] // The nearest keyframe slice

	startTS  int64 // The start time of the current slice
	duration int   // Current slice duration

	hasKeyVideoDataInCurrentSegment bool // Does the current slice have a key video frame?
	hasVideoDataInCurrentSegment    bool // Does the current slice have a video frame?
	existVideo                      bool // Does the video exist?
}

func (m *mergeWritingBuffer) TryAlloc(size int, ts int64, videoPkt, videoKey bool) ([]byte, bool) {
	//log.Sugar.Debug(" func (m *mergeWritingBuffer) TryAlloc ")
	//log.Sugar.Warnf("func (m *mergeWritingBuffer) TryAlloc(size= %s", size)
	if m.buffers.IsEmpty() {
		m.buffers.Push(MWBufferPool.Get().(*mbBuffer))
	}

	buffer := m.buffers.Peek(m.buffers.Size() - 1).buffer
	bytes := buffer.AvailableBytes()
	if bytes < size {
		// Incomplete slice, save the slice first and then allocate new memory
		if buffer.PendingBlockSize() > 0 {
			return nil, false
		}

		// -1, the current memory pool is not released
		release(m.buffers, m.buffers.Size()-1)
		m.buffers.Push(MWBufferPool.Get().(*mbBuffer))
	}

	return m.alloc(size, ts, videoPkt, videoKey), true
}

func (m *mergeWritingBuffer) alloc(size int, ts int64, videoPkt, videoKey bool) []byte {
	utils.Assert(ts != -1)
	buffer := m.buffers.Peek(m.buffers.Size() - 1).buffer
	bytes := buffer.AvailableBytes()
	// The current slice must have enough space, otherwise call TryAlloc first
	utils.Assert(bytes >= size)

	// New slice
	if m.startTS == -1 {
		m.startTS = ts
	}

	if !m.hasVideoDataInCurrentSegment && videoPkt {
		m.hasVideoDataInCurrentSegment = true
	}

	if videoKey {
		m.hasKeyVideoDataInCurrentSegment = true
	}

	if ts < m.startTS {
		m.startTS = ts
	}

	m.duration = int(ts - m.startTS)
	return buffer.Alloc(size)
}

func (m *mergeWritingBuffer) FlushSegment() (*collections.ReferenceCounter[[]byte], bool) {
	buffer := m.buffers.Peek(m.buffers.Size() - 1)
	data := buffer.buffer.Feat()
	if len(data) == 0 {
		return nil, false
	}

	counter := collections.NewReferenceCounter(data)
	// When encountering a complete keyframe slice, replace the previous set
	// or only keep the most recent audio slice
	if m.hasKeyVideoDataInCurrentSegment || !m.existVideo {
		for m.lastKeyVideoDataSegments.Size() > 0 {
			segment := m.lastKeyVideoDataSegments.Pop()
			segment.Release()
		}
	}

	if AppConfig.GOPCache {
		counter.Refer()
		m.lastKeyVideoDataSegments.Push(counter)
	}

	buffer.segments.Push(counter)

	// 清空下一个切片的标记
	m.startTS = -1
	m.duration = 0
	m.hasVideoDataInCurrentSegment = false
	key := m.hasKeyVideoDataInCurrentSegment
	m.hasKeyVideoDataInCurrentSegment = false
	return counter, key
}

func (m *mergeWritingBuffer) TryFlushSegment() (*collections.ReferenceCounter[[]byte], bool) {
	if !AppConfig.GOPCache || m.duration >= AppConfig.MergeWriteLatency {
		return m.FlushSegment()
	}

	return nil, false
}

func (m *mergeWritingBuffer) ShouldFlush(ts int64) bool {
	if m.startTS == -1 {
		return false
	}

	return int(ts-m.startTS) >= AppConfig.MergeWriteLatency
}

func (m *mergeWritingBuffer) IsNewSegment() bool {
	size := m.buffers.Size()
	return size == 0 || m.buffers.Peek(size-1).buffer.PendingBlockSize() == 0
}

func (m *mergeWritingBuffer) Reserve(size int) {
	_ = m.buffers.Peek(m.buffers.Size() - 1).buffer.Alloc(size)
}

func (m *mergeWritingBuffer) ReadSegmentsFromKeyFrameIndex(cb func(*collections.ReferenceCounter[[]byte])) {
	if !AppConfig.GOPCache || m.lastKeyVideoDataSegments.Size() == 0 {
		return
	}

	size := m.lastKeyVideoDataSegments.Size()
	for i := 0; i < size; i++ {
		cb(m.lastKeyVideoDataSegments.Peek(i))
	}
}

func (m *mergeWritingBuffer) HasVideoDataInCurrentSegment() bool {
	return m.hasVideoDataInCurrentSegment
}

func (m *mergeWritingBuffer) Close() *collections.Queue[*mbBuffer] {
	for m.lastKeyVideoDataSegments.Size() > 0 {
		m.lastKeyVideoDataSegments.Pop().Release()
	}

	if m.buffers.Size() > 0 && !release(m.buffers, m.buffers.Size()) {
		return m.buffers
	}

	return nil
}

func NewMergeWritingBuffer(existVideo bool) MergeWritingBuffer {
	buffer := &mergeWritingBuffer{
		startTS:    -1,
		existVideo: existVideo,
		buffers:    collections.NewQueue[*mbBuffer](24),
	}

	if AppConfig.GOPCache {
		buffer.lastKeyVideoDataSegments = collections.NewQueue[*collections.ReferenceCounter[[]byte]](36)
	}

	return buffer
}
