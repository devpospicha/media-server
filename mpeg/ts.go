package mpeg

import (
	"encoding/binary"

	"github.com/devpospicha/media-server/avformat/utils"
)

// section 2.4.4
const (
	PsiPat = 0x0000

	// PsiPmt 在PAT中自定义
	PsiPmt = 0x1000

	// PsiCat 私有数据
	PsiCat = 0x0001 //私有数据

	// PsiTSdt 描述流
	PsiTSdt = 0x0002

	// PsiNit 网络信息
	PsiNit = 0x0000

	TableIdPAS = 0x00

	TableIdCAS = 0x01

	TableIdPMS = 0x2

	TsPacketSize = 188

	TsPacketStartPid = 0x100
)

var stuffingBytes [188]byte

func init() {
	for i := 0; i < len(stuffingBytes); i++ {
		stuffingBytes[i] = 0xFF
	}
}

func writeAud(data []byte, id utils.AVCodecID) int {
	if utils.AVCodecIdH264 == id {
		binary.BigEndian.PutUint32(data, 0x1)
		data[4] = 0x09
		data[5] = 0xF0
		return 6
	} else if utils.AVCodecIdH265 == id {
		binary.BigEndian.PutUint32(data, 0x1)
		data[4] = 0x46
		data[5] = 0x01
		data[6] = 0x50
		return 7
	}

	return 0
}

func existAud(data []byte, id utils.AVCodecID) bool {
	if utils.AVCodecIdH264 == id {
		return binary.BigEndian.Uint32(data) == 0x1 && data[4] == 0x09
	} else if utils.AVCodecIdH265 == id {
		return binary.BigEndian.Uint32(data) == 0x1 && data[4] == 0x46
	}

	return false
}
