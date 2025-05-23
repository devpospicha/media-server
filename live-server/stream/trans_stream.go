package stream

import (
	"github.com/devpospicha/media-server/avformat"
	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
)

// TransStream 将AVPacket封装成传输流
type TransStream interface {
	GetID() TransStreamID

	SetID(id TransStreamID)

	// Input 封装传输流, 返回合并写块、时间戳、合并写块是否包含视频关键帧
	Input(packet *avformat.AVPacket) ([]*collections.ReferenceCounter[[]byte], int64, bool, error)

	AddTrack(track *Track) error

	TrackSize() int

	GetTracks() []*Track

	// WriteHeader track添加完毕, 通过调用此函数告知
	WriteHeader() error

	// GetProtocol 返回输出流协议
	GetProtocol() TransStreamProtocol

	SetProtocol(protocol TransStreamProtocol)

	// ReadExtraData 读取传输流的编码器扩展数据
	ReadExtraData(timestamp int64) ([]*collections.ReferenceCounter[[]byte], int64, error)

	// ReadKeyFrameBuffer 读取最近的包含视频关键帧的合并写队列
	ReadKeyFrameBuffer() ([]*collections.ReferenceCounter[[]byte], int64, error)

	// Close 关闭传输流, 返回还未flush的合并写块
	Close() ([]*collections.ReferenceCounter[[]byte], int64, error)

	IsExistVideo() bool

	IsTCPStreaming() bool

	GetMWBuffer() MergeWritingBuffer
}

type BaseTransStream struct {
	ID         TransStreamID
	Tracks     []*Track
	Completed  bool
	ExistVideo bool
	Protocol   TransStreamProtocol

	OutBuffer     []*collections.ReferenceCounter[[]byte] // 传输流的合并写块队列
	OutBufferSize int                                     // 传输流返合并写块队列大小
}

func (t *BaseTransStream) GetID() TransStreamID {
	return t.ID
}

func (t *BaseTransStream) SetID(id TransStreamID) {
	t.ID = id
}

func (t *BaseTransStream) Input(packet *avformat.AVPacket) ([]*collections.ReferenceCounter[[]byte], int64, bool, error) {
	return nil, -1, false, nil
}

func (t *BaseTransStream) AddTrack(track *Track) error {
	t.Tracks = append(t.Tracks, track)
	if utils.AVMediaTypeVideo == track.Stream.MediaType {
		t.ExistVideo = true
	}
	return nil
}

func (t *BaseTransStream) Close() ([]*collections.ReferenceCounter[[]byte], int64, error) {
	return nil, 0, nil
}

func (t *BaseTransStream) GetProtocol() TransStreamProtocol {
	return t.Protocol
}

func (t *BaseTransStream) SetProtocol(protocol TransStreamProtocol) {
	t.Protocol = protocol
}

func (t *BaseTransStream) ClearOutStreamBuffer() {
	t.OutBufferSize = 0
}

func (t *BaseTransStream) AppendOutStreamBuffer(buffer *collections.ReferenceCounter[[]byte]) {
	if t.OutBufferSize+1 > len(t.OutBuffer) {
		// 扩容
		size := (t.OutBufferSize + 1) * 2
		newBuffer := make([]*collections.ReferenceCounter[[]byte], size)
		for i := 0; i < t.OutBufferSize; i++ {
			newBuffer[i] = t.OutBuffer[i]
		}

		t.OutBuffer = newBuffer
	}

	t.OutBuffer[t.OutBufferSize] = buffer
	t.OutBufferSize++
}

func (t *BaseTransStream) TrackSize() int {
	return len(t.Tracks)
}

func (t *BaseTransStream) GetTracks() []*Track {
	return t.Tracks
}

func (t *BaseTransStream) IsExistVideo() bool {
	return t.ExistVideo
}

func (t *BaseTransStream) ReadExtraData(timestamp int64) ([]*collections.ReferenceCounter[[]byte], int64, error) {
	return nil, 0, nil
}

func (t *BaseTransStream) ReadKeyFrameBuffer() ([]*collections.ReferenceCounter[[]byte], int64, error) {
	return nil, 0, nil
}

func (t *BaseTransStream) IsTCPStreaming() bool {
	return false
}

func (t *BaseTransStream) GetMWBuffer() MergeWritingBuffer {
	return nil
}

type TCPTransStream struct {
	BaseTransStream

	// 合并写内存泄露问题: 推流结束后, mwBuffer的data一直释放不掉, 只有拉流全部断开之后, 才会释放该内存.
	// 起初怀疑是代码层哪儿有问题, 但是测试发现如果将合并写切片再拷贝一次发送 给sink, 推流结束后，mwBuffer的data内存块释放没问题, 只有拷贝的内存块未释放. 所以排除了代码层造成内存泄露的可能性.
	// 看来是conn在write后还会持有data. 查阅代码发现, 的确如此. 向fd发送数据前buffer会引用data, 但是后续没有赋值为nil, 取消引用. https://github.com/golang/go/blob/d38f1d13fa413436d38d86fe86d6a146be44bb84/src/internal/poll/fd_windows.go#L694
	MWBuffer MergeWritingBuffer //合并写缓冲区, 同时作为用户态的发送缓冲区
}

func (t *TCPTransStream) IsTCPStreaming() bool {
	return true
}

func (t *TCPTransStream) GetMWBuffer() MergeWritingBuffer {
	return t.MWBuffer
}
