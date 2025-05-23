package rtsp

import (
	"net"
	"time"

	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/log"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/rtp"
	"github.com/devpospicha/media-server/transport"
	"github.com/pion/rtcp"
)

var (
	TransportManger transport.Manager
)

// Sink rtsp拉流sink
// 对于udp而言, 每个sink维护多个transport
// tcp使用信令链路传输
type Sink struct {
	stream.BaseSink

	senders []*rtp.RtpSender // 一个rtsp源, 可能存在多个流, 每个流都需要拉取
	cb      func(sdp string) // sdp回调, 响应describe
}

func (s *Sink) StartStreaming(transStream stream.TransStream) error {
	utils.Assert(transStream.TrackSize() > 0)
	if s.senders != nil {
		return nil
	}

	s.senders = make([]*rtp.RtpSender, transStream.TrackSize())
	// sdp回调给sink, sink应答给describe请求
	if s.cb != nil {
		s.cb(transStream.(*TransStream).sdp)
	}
	return nil
}

func (s *Sink) AddSender(index int, tcp bool, ssrc uint32) (uint16, uint16, error) {
	utils.Assert(index < cap(s.senders))
	utils.Assert(s.senders[index] == nil)

	var err error
	var rtpPort uint16
	var rtcpPort uint16

	sender := rtp.RtpSender{
		SSRC: ssrc,
	}

	if tcp {
		s.TCPStreaming = true
		s.BaseSink.EnableAsyncWriteMode(512)
	} else {
		sender.Rtp, err = TransportManger.NewUDPServer()
		if err != nil {
			return 0, 0, err
		}

		sender.Rtcp, err = TransportManger.NewUDPServer()
		if err != nil {
			sender.Rtp.Close()
			sender.Rtp = nil
			return 0, 0, err
		}

		sender.Rtp.SetHandler2(nil, sender.OnRTPPacket, nil)
		sender.Rtcp.SetHandler2(nil, sender.OnRTCPPacket, nil)
		sender.Rtp.(*transport.UDPServer).Receive()
		sender.Rtcp.(*transport.UDPServer).Receive()

		rtpPort = uint16(sender.Rtp.ListenPort())
		rtcpPort = uint16(sender.Rtcp.ListenPort())
	}

	s.senders[index] = &sender
	return rtpPort, rtcpPort, err
}

func (s *Sink) Write(index int, data []*collections.ReferenceCounter[[]byte], rtpTime int64, keyVideo bool) error {
	// 拉流方还没有连接上来
	if index >= cap(s.senders) || s.senders[index] == nil {
		return nil
	}

	for i, bytes := range data {
		sender := s.senders[index]
		sender.PktCount++
		sender.OctetCount += len(bytes.Get())
		if s.TCPStreaming {
			// 一次发送会花屏?
			// return s.BaseSink.Write(index, data, rtpTime)
			s.BaseSink.Write(index, data[i:i+1], rtpTime, keyVideo)
			//s.Conn.Write(bytes.Get())
		} else {
			// 发送rtcp sr包
			sender.RtpConn.Write(bytes.Get()[OverTcpHeaderSize:])

			if sender.RtcpConn == nil || sender.PktCount%100 != 0 {
				continue
			}

			nano := uint64(time.Now().UnixNano())
			ntp := (nano/1000000000 + 2208988800<<32) | (nano % 1000000000)
			sr := rtcp.SenderReport{
				SSRC:        sender.SSRC,
				NTPTime:     ntp,
				RTPTime:     uint32(rtpTime),
				PacketCount: uint32(sender.PktCount),
				OctetCount:  uint32(sender.OctetCount),
			}

			marshal, err := sr.Marshal()
			if err != nil {
				log.Sugar.Errorf("创建rtcp sr消息失败 err:%s msg:%v", err.Error(), sr)
			}

			sender.RtcpConn.Write(marshal)
		}
	}

	return nil
}

// 拉流链路是否已经连接上
// 拉流测发送了play请求, 并且对于udp而言, 还需要收到nat穿透包
func (s *Sink) isConnected(index int) bool {
	return s.TCPStreaming || (s.senders[index] != nil && s.senders[index].RtpConn != nil)
}

func (s *Sink) Close() {
	s.BaseSink.Close()

	for _, sender := range s.senders {
		if sender == nil {
			continue
		}

		if sender.Rtp != nil {
			sender.Rtp.Close()
		}

		if sender.Rtcp != nil {
			sender.Rtcp.Close()
		}
	}
}

func NewSink(id stream.SinkID, sourceId string, conn net.Conn, cb func(sdp string)) stream.Sink {
	return &Sink{
		stream.BaseSink{ID: id, SourceID: sourceId, State: stream.SessionStateCreated, Protocol: stream.TransStreamRtsp, Conn: conn},
		nil,
		cb,
	}
}
