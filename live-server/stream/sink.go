package stream

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/log"
)

const (
	EnableFastForward         = false           // 发送超时, 开始追帧
	EnableCloseOnWriteTimeout = false           // 发送超时, 直接关闭Sink
	WriteTimeout              = 2000            // 发送超时时间, 单位毫秒. 如果发送超时, 开始追帧/关闭Sink
	MaxPendingDataSize        = 1024 * 1024 * 5 // 最大等待发送数据大小, 超过该大小, 开始追帧/关闭Sink
)

// Sink 对拉流端的封装
type Sink interface {
	GetID() SinkID

	SetID(sink SinkID)

	GetSourceID() string

	Write(index int, data []*collections.ReferenceCounter[[]byte], ts int64, keyVideo bool) error

	GetTransStreamID() TransStreamID

	SetTransStreamID(id TransStreamID)

	GetProtocol() TransStreamProtocol

	// GetState 获取Sink状态, 调用前外部必须手动加锁
	GetState() SessionState

	// SetState 设置Sink状态, 调用前外部必须手动加锁
	SetState(state SessionState)

	EnableVideo() bool

	// SetEnableVideo 设置是否拉取视频流, 允许客户端只拉取音频流
	SetEnableVideo(enable bool)

	// DesiredAudioCodecId 允许客户端拉取指定的音频流
	DesiredAudioCodecId() utils.AVCodecID

	// DesiredVideoCodecId DescribeVideoCodecId 允许客户端拉取指定的视频流
	DesiredVideoCodecId() utils.AVCodecID

	// Close 关闭释放Sink, 从传输流或等待队列中删除sink
	Close()

	String() string

	RemoteAddr() string

	// Lock Sink请求拉流->Source推流->Sink断开整个阶段, 是无锁线程安全
	// 如果Sink在等待队列-Sink断开, 这个过程是非线程安全的
	// 所以Source在AddSink时, SessionStateWait状态时, 需要加锁保护.
	Lock()

	UnLock()

	UrlValues() url.Values

	SetUrlValues(values url.Values)

	// StartStreaming Source向Sink开始推流时调用
	StartStreaming(stream TransStream) error

	// StopStreaming Source向Sink停止推流时调用
	StopStreaming(stream TransStream)

	GetConn() net.Conn

	IsTCPStreaming() bool

	GetSentPacketCount() int

	SetSentPacketCount(int)

	IncreaseSentPacketCount()

	IsReady() bool

	SetReady(ok bool)

	CreateTime() time.Time

	SetCreateTime(time time.Time)

	// EnableAsyncWriteMode 开启异步发送
	EnableAsyncWriteMode(queueSize int)

	PendingSendQueueSize() int
}

type BaseSink struct {
	ID            SinkID
	SourceID      string
	Protocol      TransStreamProtocol
	State         SessionState
	TransStreamID TransStreamID
	disableVideo  bool

	lock sync.RWMutex

	DesiredAudioCodecId_ utils.AVCodecID
	DesiredVideoCodecId_ utils.AVCodecID

	Conn         net.Conn   // 拉流信令链路
	TCPStreaming bool       // 是否是TCP流式拉流
	urlValues    url.Values // 拉流时携带的Url参数

	SentPacketCount         int  // 发包计数
	Ready                   bool // 是否准备好推流. Sink可以通过控制该变量, 达到触发Source推流, 但不立即拉流的目的. 比如rtsp拉流端在信令交互阶段,需要先获取媒体信息,再拉流.
	createTime              time.Time
	totalDataSize           atomic.Uint64
	writtenDataSize         atomic.Uint64
	lastKeyVideoDataSegment *collections.ReferenceCounter[[]byte]

	pendingSendQueue  chan *collections.ReferenceCounter[[]byte]                     // 等待发送的数据队列
	blockedBufferList *collections.LinkedList[*collections.ReferenceCounter[[]byte]] // 异步队列阻塞后的切片数据

	cancelFunc func()
	cancelCtx  context.Context
}

func (s *BaseSink) GetID() SinkID {
	return s.ID
}

func (s *BaseSink) SetID(id SinkID) {
	s.ID = id
}

func (s *BaseSink) fastForward(firstSegment *collections.ReferenceCounter[[]byte]) (*collections.ReferenceCounter[[]byte], bool) {
	if s.lastKeyVideoDataSegment == firstSegment {
		return firstSegment, true
	}
	log.Sugar.Debug("func (s *BaseSink) fastForward")
	firstSegment.Release()
	s.writtenDataSize.Add(uint64(len(firstSegment.Get())))

	for len(s.pendingSendQueue) > 0 {
		buffer := <-s.pendingSendQueue
		// 不存在视频, 清空队列
		// 还没有追到最近的关键帧, 继续追帧
		if s.lastKeyVideoDataSegment == nil || buffer != s.lastKeyVideoDataSegment {
			buffer.Release()
			s.writtenDataSize.Add(uint64(len(buffer.Get())))
		} else {
			// else if TransStreamFlv == s.Protocol {
			// 	// 重置第一个flv tag的pre tag size
			// 	if data == s.lastKeyVideoDataSegment {
			// 		binary.BigEndian.PutUint32(GetFLVTag(data.Get()), s.flvExtraDataPreTagSize)
			// 	}
			// }
			return buffer, true
		}
	}

	// 不存在视频, 清空队列后, 等待下次继续推流
	return nil, s.lastKeyVideoDataSegment == nil
}

func (s *BaseSink) doAsyncWrite() {
	defer func() {
		// 释放未发送的数据
		for len(s.pendingSendQueue) > 0 {
			buffer := <-s.pendingSendQueue
			buffer.Release()
		}

		for s.blockedBufferList.Size() > 0 {
			buffer := s.blockedBufferList.Remove(0)
			buffer.Release()
		}

		ReleasePendingBuffers(s.SourceID, s.TransStreamID)
	}()

	var fastForward bool
	for {
		select {
		case <-s.cancelCtx.Done():
			return
		case data := <-s.pendingSendQueue:
			// 追帧到最近的关键帧
			if fastForward {
				var ok bool
				data, ok = s.fastForward(data)
				if fastForward = !ok; !ok || data == nil {
					break
				}
			}

			l := time.Now().UnixMilli()
			_, err := s.Conn.Write(data.Get())
			duration := time.Now().UnixMilli() - l
			if err != nil {
				log.Sugar.Errorf(err.Error())
				<-s.cancelCtx.Done()
				return
			}

			data.Release()
			s.writtenDataSize.Add(uint64(len(data.Get())))

			if (EnableFastForward || EnableCloseOnWriteTimeout) && duration > WriteTimeout {
				// 等待发送的数据大小超过最大等待发送数据大小, 开始追帧
				// 如果extra data没有发送完成, 拉流端会有问题. 给个最低128k限制, 当然也可以统计真实的extra data大小
				// timeout := s.writtenDataSize.Load() > 128*1024 && s.totalDataSize.Load()-s.writtenDataSize.Load() > MaxPendingDataSize
				timeout := s.totalDataSize.Load()-s.writtenDataSize.Load() > MaxPendingDataSize
				if !timeout {
					break
				}

				if EnableCloseOnWriteTimeout {
					log.Sugar.Errorf("write timeout, closing sink. writtenDataSize: %d, totalDataSize: %d sink: %s, source: %s", s.writtenDataSize.Load(), s.totalDataSize.Load(), s.ID, s.SourceID)
					s.Conn.Close()
					// 不直接return, 从连接处最外层逐步关闭Sink
					// 如果直接return,Sink未从Source中删除, 执行defer func函数操作blockedBufferList与write非线程安全, 可能会panic, 以及管道清理不干净, buffer不释放等问题
					// return

					<-s.cancelCtx.Done()
					return
				}

				if EnableFastForward {
					fastForward = true
					log.Sugar.Errorf("write timeout, fast forward. writtenDataSize: %d, totalDataSize: %d sink: %s, source: %s", s.writtenDataSize.Load(), s.totalDataSize.Load(), s.ID, s.SourceID)
				}
			}
			break
		}
	}
}

func (s *BaseSink) EnableAsyncWriteMode(queueSize int) {
	utils.Assert(s.Conn != nil)
	s.pendingSendQueue = make(chan *collections.ReferenceCounter[[]byte], queueSize)
	s.blockedBufferList = &collections.LinkedList[*collections.ReferenceCounter[[]byte]]{}
	s.cancelCtx, s.cancelFunc = context.WithCancel(context.Background())
	go s.doAsyncWrite()
}

func (s *BaseSink) Write(index int, data []*collections.ReferenceCounter[[]byte], ts int64, keyVideo bool) error {
	if s.Conn == nil {
		return nil
	}

	if keyVideo {
		s.lastKeyVideoDataSegment = data[0]
	}

	// 统计发送的数据大小
	for _, datum := range data {
		s.totalDataSize.Add(uint64(len(datum.Get())))
	}

	// 发送被阻塞的数据
	for s.blockedBufferList.Size() > 0 {
		bytes := s.blockedBufferList.Get(0)
		select {
		case s.pendingSendQueue <- bytes:
			s.blockedBufferList.Remove(0)
			break
		default:
			// 发送被阻塞的数据失败, 将本次发送的数据加入阻塞队列
			for _, datum := range data {
				s.blockedBufferList.Add(datum)
				datum.Refer()
			}
			return nil
		}
	}

	for _, bytes := range data {
		if s.cancelCtx != nil {
			bytes.Refer()
			select {
			case s.pendingSendQueue <- bytes:
				break
			default:
				// 将本次发送的数据加入阻塞队列
				s.blockedBufferList.Add(bytes)
				//return transport.ZeroWindowSizeError{}
				return nil
			}
		} else {
			_, err := s.Conn.Write(bytes.Get())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *BaseSink) PendingSendQueueSize() int {
	return len(s.pendingSendQueue)
}

func (s *BaseSink) GetSourceID() string {
	return s.SourceID
}

func (s *BaseSink) GetTransStreamID() TransStreamID {
	return s.TransStreamID
}

func (s *BaseSink) SetTransStreamID(id TransStreamID) {
	s.TransStreamID = id
}

func (s *BaseSink) GetProtocol() TransStreamProtocol {
	return s.Protocol
}

func (s *BaseSink) Lock() {
	s.lock.Lock()
}

func (s *BaseSink) UnLock() {
	s.lock.Unlock()
}

func (s *BaseSink) GetState() SessionState {
	//utils.Assert(!s.lock.TryLock())

	return s.State
}

func (s *BaseSink) SetState(state SessionState) {
	utils.Assert(!s.lock.TryLock())

	s.State = state
}

func (s *BaseSink) EnableVideo() bool {
	return !s.disableVideo
}

func (s *BaseSink) SetEnableVideo(enable bool) {
	s.disableVideo = !enable
}

func (s *BaseSink) DesiredAudioCodecId() utils.AVCodecID {
	return s.DesiredAudioCodecId_
}

func (s *BaseSink) DesiredVideoCodecId() utils.AVCodecID {
	return s.DesiredVideoCodecId_
}

// Close 做如下事情:
// 1. Sink如果正在拉流, 删除任务交给Source处理, 否则直接从等待队列删除Sink.
// 2. 发送PlayDoneHook事件
func (s *BaseSink) Close() {
	log.Sugar.Debugf("closing the %s sink. id: %s. current session state: %s", s.Protocol, SinkId2String(s.ID), s.State)

	s.Lock()
	defer func() {
		// 此时Sink已经从Source或等待队列中删除
		closed := s.State == SessionStateClosed
		s.State = SessionStateClosed
		s.UnLock()

		if !closed {
			if s.cancelCtx != nil {
				s.cancelFunc()
			}

			// 最后断开网络连接, 确保从source删除sink之前, 推流是安全的.
			if s.Conn != nil {
				s.Conn.Close()
			}
		}
	}()

	// 已经关闭或Sink未添加到任何队列, 不做处理
	if SessionStateClosed == s.State || s.State < SessionStateWaiting {
		return
	} else if s.State == SessionStateTransferring {
		// 从source中删除sink, 如果source为nil, 已经结束推流.
		if source := SourceManager.Find(s.SourceID); source != nil {
			source.RemoveSink(s)
		}
	} else if s.State == SessionStateWaiting {
		// 从等待队列中删除sink
		RemoveSinkFromWaitingQueue(s.SourceID, s.ID)
		go HookPlayDoneEvent(s)

		// 等待队列为空, 不再保留推流源信息
		if !ExistSourceInWaitingQueue(s.SourceID) {
			streamEndInfoManager.Remove(s.SourceID)
		}
	}
}

func (s *BaseSink) String() string {
	return fmt.Sprintf("%s-%v source:%s", s.GetProtocol().String(), s.ID, s.SourceID)
}

func (s *BaseSink) RemoteAddr() string {
	if s.Conn == nil {
		return ""
	}

	return s.Conn.RemoteAddr().String()
}

func (s *BaseSink) UrlValues() url.Values {
	return s.urlValues
}

func (s *BaseSink) SetUrlValues(values url.Values) {
	s.urlValues = values
}

func (s *BaseSink) StartStreaming(stream TransStream) error {
	return nil
}

func (s *BaseSink) StopStreaming(stream TransStream) {
	s.SentPacketCount = 0
}

func (s *BaseSink) GetConn() net.Conn {
	return s.Conn
}

func (s *BaseSink) IsTCPStreaming() bool {
	return s.TCPStreaming
}

func (s *BaseSink) GetSentPacketCount() int {
	return s.SentPacketCount
}

func (s *BaseSink) SetSentPacketCount(count int) {
	s.SentPacketCount = count
}

func (s *BaseSink) IncreaseSentPacketCount() {
	s.SentPacketCount++
}

func (s *BaseSink) IsReady() bool {
	return s.Ready
}

func (s *BaseSink) SetReady(ok bool) {
	s.Ready = ok
	if ok {
		s.SetCreateTime(time.Now())
	}
}

func (s *BaseSink) CreateTime() time.Time {
	return s.createTime
}

func (s *BaseSink) SetCreateTime(time time.Time) {
	s.createTime = time
}
