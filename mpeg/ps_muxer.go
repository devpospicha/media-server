package mpeg

import (
	"encoding/binary"
	"fmt"

	"github.com/devpospicha/media-server/avformat/bufio"
	"github.com/devpospicha/media-server/avformat/utils"
)

const (
	MaxPesPacketLength = 65535 - 100
)

type PSMuxer struct {
	streams []struct {
		streamType int
		pesHeader  PESHeader
		mediaType  utils.AVMediaType
	}

	packHeader   PackHeader
	systemHeader SystemHeader
	psm          ProgramStreamMap
}

func (r *PSMuxer) AddTrack(mediaType utils.AVMediaType, id utils.AVCodecID) (int, error) {
	streamType, err := AVCodecID2StreamType(id)
	if err != nil {
		return -1, nil
	}

	for _, s := range r.streams {
		utils.Assert(s.streamType != streamType)
	}

	var streamId int
	if utils.AVMediaTypeAudio == mediaType {
		streamId = StreamIDAudio

		r.systemHeader.streams = append(r.systemHeader.streams,
			StreamHeader{
				streamId:         StreamIDAudio,
				bufferBoundScale: 0,
				bufferSizeBound:  32,
			},
		)

		r.psm.elementaryStreams = append(r.psm.elementaryStreams,
			ElementaryStream{
				streamType: byte(streamType),
				streamId:   StreamIDAudio,
				info:       nil,
			})

		r.systemHeader.audioBound++
	} else if utils.AVMediaTypeVideo == mediaType {
		streamId = StreamIDVideo

		r.systemHeader.streams = append(r.systemHeader.streams,
			StreamHeader{
				streamId:         StreamIDVideo,
				bufferBoundScale: 1,
				bufferSizeBound:  400,
			},
		)

		r.systemHeader.videoBound++
	} else {
		panic(fmt.Sprintf("unsupported type: %s", mediaType))
	}

	r.psm.elementaryStreams = append(r.psm.elementaryStreams,
		ElementaryStream{
			streamType: byte(streamType),
			streamId:   byte(streamId),
			info:       nil,
		})

	r.streams = append(r.streams, struct {
		streamType int
		pesHeader  PESHeader
		mediaType  utils.AVMediaType
	}{streamType: streamType, mediaType: mediaType},
	)

	r.streams[len(r.streams)-1].pesHeader.streamId = byte(streamId)
	r.streams[len(r.streams)-1].pesHeader.dataAlignmentIndicator = 1
	return len(r.streams) - 1, nil
}

// Input
// @pts must be >= 0
// @dts if the DTS does not exist. DTS must be < 0
func (r *PSMuxer) Input(dst []byte, index int, key bool, data []byte, pts, dts *int64) int {
	utils.Assert(pts != nil)

	var i int
	s := r.streams[index]
	s.pesHeader.pts = *pts
	s.pesHeader.ptsDtsFlags = 0x2
	if dts != nil {
		s.pesHeader.ptsDtsFlags = 0x3
		s.pesHeader.dts = *dts

		if *dts >= 3600 {
			r.packHeader.systemClockReferenceBase = *dts - 3600
		}
	} else {
		r.packHeader.systemClockReferenceBase = 0
	}

	// add pack header
	i += r.packHeader.Marshal(dst[:])

	// add system header and psm
	if key {
		i += r.systemHeader.Marshal(dst[i:])
		i += r.psm.Marshal(dst[i:])
	}

	length := len(data)
	var payloadSize int
	for j := 0; j < length; j += payloadSize {
		payloadSize = bufio.MinInt(length-j, MaxPesPacketLength)
		n := s.pesHeader.Marshal(dst[i:])
		binary.BigEndian.PutUint16(dst[i+4:], uint16(payloadSize+n-6))

		i += n
		copy(dst[i:], data[j:j+payloadSize])
		i += payloadSize
		// Only the first packet contains PTS and DTS.
		s.pesHeader.ptsDtsFlags = 0x00
	}

	return i
}

func NewPsMuxer() *PSMuxer {
	m := &PSMuxer{}
	m.packHeader.programMuxRate = 6106
	m.systemHeader.rateBound = 26234
	return m
}
