package main

import (
	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/flv"
	"github.com/devpospicha/media-server/live-server/hls"
	"github.com/devpospicha/media-server/live-server/rtsp"
	stream "github.com/devpospicha/media-server/live-server/stream"
)

// 处理不同包不能相互引用的需求

func NewStreamEndInfo(source stream.Source) *stream.StreamEndInfo {
	tracks := source.OriginTracks()
	streams := source.GetTransStreams()

	if len(tracks) < 1 || len(streams) < 1 {
		return nil
	}

	info := stream.StreamEndInfo{
		ID:         source.GetID(),
		Timestamps: make(map[utils.AVCodecID][2]int64, len(tracks)),
	}

	for _, track := range tracks {
		var timestamp [2]int64
		timestamp[0] = track.Dts + int64(track.FrameDuration)
		timestamp[1] = track.Pts + int64(track.FrameDuration)

		info.Timestamps[track.Stream.CodecID] = timestamp
	}

	for _, transStream := range streams {
		// 获取ts切片序号
		if stream.TransStreamHls == transStream.GetProtocol() {
			if hls := transStream.(*hls.TransStream); hls.M3U8Writer.Size() > 0 {
				info.M3U8Writer = hls.M3U8Writer
				info.PlaylistFormat = hls.PlaylistFormat
			}
		} else if stream.TransStreamRtsp == transStream.GetProtocol() {
			if rtsp := transStream.(*rtsp.TransStream); len(rtsp.Tracks) > 0 {
				info.RtspTracks = make(map[byte]uint16, len(tracks))
				for _, track := range rtsp.RtspTracks {
					info.RtspTracks[track.PT] = track.EndSeq
				}
			}
		} else if stream.TransStreamFlv == transStream.GetProtocol() {
			stream := transStream.(*flv.TransStream)
			info.FLVPrevTagSize = stream.Muxer.PrevTagSize()
		}
	}

	return &info
}
