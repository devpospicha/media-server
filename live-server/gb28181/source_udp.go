package gb28181

import (
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/pion/rtp"
)

// UDPSource 国标UDP推流源
type UDPSource struct {
	BaseGBSource

	jitterBuffer *stream.JitterBuffer[*rtp.Packet]
}

func (u *UDPSource) SetupType() SetupType {
	return SetupUDP
}

// OnOrderedRtp 有序RTP包回调
func (u *UDPSource) OnOrderedRtp(packet *rtp.Packet) {
	// 此时还在网络收流携程, 交给Source的主协程处理
	u.PublishSource.Input(packet.Raw)
}

// InputRtpPacket 将RTP包排序后，交给Source的主协程处理
func (u *UDPSource) InputRtpPacket(pkt *rtp.Packet) error {
	block := stream.UDPReceiveBufferPool.Get().([]byte)
	copy(block, pkt.Raw)

	pkt.Raw = block[:len(pkt.Raw)]
	u.jitterBuffer.Push(pkt.SequenceNumber, pkt)
	for pop := u.jitterBuffer.Pop(true); pop != nil; pop = u.jitterBuffer.Pop(true) {
		u.OnOrderedRtp(pop)
	}
	return nil
}

func (u *UDPSource) Close() {
	// 清空剩余的包
	for pop := u.jitterBuffer.Pop(false); pop != nil; pop = u.jitterBuffer.Pop(false) {
		u.OnOrderedRtp(pop)
	}

	u.BaseGBSource.Close()
}

func NewUDPSource() *UDPSource {
	return &UDPSource{
		jitterBuffer: stream.NewJitterBuffer[*rtp.Packet](),
	}
}
