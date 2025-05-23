package mpeg

import (
	"github.com/devpospicha/media-server/avformat/bufio"
)

type TSPacket struct {
	syncByte                   byte // 1byte fixed 0x47
	transportErrorIndicator    byte // 1bit
	payloadUnitStartIndicator  byte // 1bit //1-PSI(PAT/PMT/...)和PES开始包
	transportPriority          byte // 1bit
	pid                        int  // 13bits //0x0000-PAT/0x001-CAT/0x002-TSDT/0x0004-0x000F reserved/0x1FFF null packet
	transportScramblingControl byte // 2bits 加密使用
	adaptationFieldControl     byte // 2bits 01-无自适应字段,仅有负载数据/10-仅有自适应字段数据/11-自适应字段后跟随负载数据
	continuityCounter          byte // 4bits TS包计数器, 随着相同PID的每个包增加, 最大值16, 超过后回环. 自适应字段标记为‘00’/'10'时, 不增加.
	adaptationField            AdaptationField
	adaptationFieldEnable      bool
	data                       []byte
}

func (t *TSPacket) MarshalHeader(dst []byte) int {
	dst[0] = 0x47
	dst[1] = (t.transportErrorIndicator & 0x1 << 7) | (t.payloadUnitStartIndicator & 0x1 << 6) | (t.transportPriority & 0x1 << 5) | byte(t.pid>>8&0x1F)
	dst[2] = byte(t.pid)
	dst[3] = (t.transportScramblingControl & 0x3 << 6) | (t.adaptationFieldControl & 0x3 << 4) | (t.continuityCounter & 0xF)
	return 4
}

func (t *TSPacket) Marshal(dst []byte, data ...[]byte) int {
	size := 0
	for _, bytes := range data {
		size += len(bytes)
	}

	var pktSize = 4
	// 负载数据不足以写满整个TS包, 需要用0xFF填充满, 开启自适应字段
	if !t.adaptationFieldEnable && TsPacketSize-pktSize > size {
		t.adaptationFieldEnable = true
	}

	if t.adaptationFieldEnable {
		// 写自适应字段
		pktSize += t.adaptationField.Marshal(dst[4:], size)
	}

	payloadLength := TsPacketSize - pktSize

	// 写TS包头
	if payloadLength == TsPacketSize-4 {
		t.adaptationFieldControl = 0b01
	} else if payloadLength > 0 {
		t.adaptationFieldControl = 0b11
	} else {
		t.adaptationFieldControl = 0b10
	}

	_ = t.MarshalHeader(dst)

	for _, bytes := range data {
		n := bufio.MinInt(len(bytes), TsPacketSize-pktSize)
		copy(dst[pktSize:], bytes[:n])

		pktSize += n
		if TsPacketSize == pktSize {
			break
		}
	}

	return payloadLength
}

func (t *TSPacket) MarshalSection(dst []byte, data []byte) int {
	t.adaptationFieldControl = 0b01
	t.payloadUnitStartIndicator = 1

	pktSize := t.MarshalHeader(dst)
	copy(dst[pktSize:], data)

	length := pktSize + len(data)
	if n := TsPacketSize - length; n > 0 {
		copy(dst[length:], stuffingBytes[:n])
	}

	return len(data)
}

func (t *TSPacket) increaseCounter() {
	t.continuityCounter = (t.continuityCounter + 1) % 16
}

func (t *TSPacket) Unmarshal(data []byte) int {
	reader := bufio.BitsReader{Data: data}
	t.syncByte = byte(reader.Read(8))
	t.transportErrorIndicator = byte(reader.Read(1))
	t.payloadUnitStartIndicator = byte(reader.Read(1))
	t.transportPriority = byte(reader.Read(1))
	t.pid = int(reader.Read(13))
	t.transportScramblingControl = byte(reader.Read(2))
	t.adaptationFieldControl = byte(reader.Read(2))
	t.continuityCounter = byte(reader.Read(4))

	switch t.adaptationFieldControl {
	case 0x00:
		// discard
		break
	case 0x01:
		// payload data
		t.data = data[4:]
		break
	case 0x02:
		// adaptation field only,no payload
		t.adaptationField.Unmarshal(data[4:])
		break
	case 0x03:
		// 2.4.3.4 adaptation_field
		n := t.adaptationField.Unmarshal(data[4:])
		t.data = data[4+n:]
		break
	}

	return len(data)
}
