package rtc

import (
	"fmt"
	"io"
	"time"

	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/log"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

type Sink struct {
	stream.BaseSink

	offer  string
	answer string

	peer   *webrtc.PeerConnection
	tracks []*webrtc.TrackLocalStaticSample
	state  webrtc.ICEConnectionState

	cb func(sdp string)
}

func (s *Sink) StartStreaming(transStream stream.TransStream) error {
	if s.peer != nil {
		return nil
	}

	// 创建PeerConnection
	var remoteTrack *webrtc.TrackLocalStaticSample
	s.tracks = make([]*webrtc.TrackLocalStaticSample, transStream.TrackSize())

	connection, err := webrtcApi.NewPeerConnection(webrtc.Configuration{})
	connection.OnICECandidate(func(candidate *webrtc.ICECandidate) {

	})

	tracks := transStream.GetTracks()
	for index, track := range tracks {
		var mimeType string
		var id string
		codecId := track.Stream.CodecID
		if utils.AVCodecIdH264 == codecId {
			mimeType = webrtc.MimeTypeH264
		} else if utils.AVCodecIdH265 == codecId {
			mimeType = webrtc.MimeTypeH265
		} else if utils.AVCodecIdAV1 == codecId {
			mimeType = webrtc.MimeTypeAV1
		} else if utils.AVCodecIdVP8 == codecId {
			mimeType = webrtc.MimeTypeVP8
		} else if utils.AVCodecIdVP9 == codecId {
			mimeType = webrtc.MimeTypeVP9
		} else if utils.AVCodecIdOPUS == codecId {
			mimeType = webrtc.MimeTypeOpus
		} else if utils.AVCodecIdPCMALAW == codecId {
			mimeType = webrtc.MimeTypePCMA
		} else if utils.AVCodecIdPCMMULAW == codecId {
			mimeType = webrtc.MimeTypePCMU
		} else {
			log.Sugar.Errorf("codec %s not compatible with webrtc", codecId)
			continue
		}

		if utils.AVMediaTypeAudio == track.Stream.MediaType {
			id = "audio"
		} else {
			id = "video"
		}

		remoteTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: mimeType}, id, "pion")
		if err != nil {
			return err
		}

		transceiver, err := connection.AddTransceiverFromTrack(remoteTrack, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
		if err != nil {
			return err
		}

		// pion需要在外部读取rtcp包,才会处理nack包丢包重传. https://github.com/pion/interceptor/blob/e1874104865b23ba465ecc505f959daf156cc2a5/pkg/nack/responder_interceptor.go#L82
		go func() {
			for {
				_, _, err := transceiver.Sender().ReadRTCP()
				if err == nil {
					continue
				} else if io.EOF == err || io.ErrClosedPipe == err {
					break
				} else {
					println(err.Error())
				}
			}
		}()

		s.tracks[index] = remoteTrack
	}

	if len(connection.GetTransceivers()) == 0 {
		return fmt.Errorf("no track added")
	} else if err = connection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: s.offer}); err != nil {
		return err
	}

	complete := webrtc.GatheringCompletePromise(connection)
	answer, err := connection.CreateAnswer(nil)
	if err != nil {
		return err
	} else if err = connection.SetLocalDescription(answer); err != nil {
		return err
	}

	<-complete
	connection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		s.state = state
		log.Sugar.Infof("ice state: %v sink: %d source: %s", state.String(), s.GetID(), s.SourceID)

		if state > webrtc.ICEConnectionStateDisconnected {
			log.Sugar.Errorf("webrtc peer断开连接 sink: %v source: %s", s.GetID(), s.SourceID)
			s.Close()
		}
	})

	s.peer = connection

	// offer的sdp, 应答给http请求
	if s.cb != nil {
		s.cb(connection.LocalDescription().SDP)
	}
	return nil
}

func (s *Sink) Close() {
	s.BaseSink.Close()

	if s.peer != nil {
		s.peer.Close()
		s.peer = nil
	}
}

func (s *Sink) Write(index int, data []*collections.ReferenceCounter[[]byte], ts int64, keyVideo bool) error {
	if s.tracks[index] == nil {
		return nil
	}

	for _, bytes := range data {
		err := s.tracks[index].WriteSample(media.Sample{
			Data:     bytes.Get(),
			Duration: time.Duration(ts) * time.Millisecond,
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func NewSink(id stream.SinkID, sourceId string, offer string, cb func(sdp string)) stream.Sink {
	return &Sink{stream.BaseSink{ID: id, SourceID: sourceId, State: stream.SessionStateCreated, Protocol: stream.TransStreamRtc, TCPStreaming: false}, offer, "", nil, nil, webrtc.ICEConnectionStateNew, cb}
}
