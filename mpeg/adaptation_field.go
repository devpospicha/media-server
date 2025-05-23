package mpeg

import "github.com/devpospicha/media-server/avformat/bufio"

type AdaptationField struct {
	length                            byte
	discontinuityIndicator            byte // 1
	randomAccessIndicator             byte // 1
	elementaryStreamPriorityIndicator byte // 1
	PCRFlag                           byte // 1
	OPCRFlag                          byte // 1
	splicingPointFlag                 byte // 1
	transportPrivateDataFlag          byte // 1
	adaptationFieldExtensionFlag      byte // 1
	pcrBase                           int64
	pcrExt                            int
	opcrBase                          int64
	opcrExt                           int
	spliceCountdown                   byte
	privateData                       []byte
	ltwFlag                           byte // extension
	piecewiseRateFlag                 byte
	seamlessSpliceFlag                byte
	ltwValidFlag                      byte
	ltwOffset                         uint16
	piecewiseRate                     int
	spliceType                        byte
	dtsNextAU                         int64
	ExtReservedData                   []byte
	stuffingBytes                     []byte
}

func (a *AdaptationField) Unmarshal(data []byte) int {
	reader := bufio.BitsReader{Data: data}
	length := reader.Read(8)
	if length < 1 {
		return 1
	}

	a.discontinuityIndicator = byte(reader.Read(1))
	a.randomAccessIndicator = byte(reader.Read(1))
	a.elementaryStreamPriorityIndicator = byte(reader.Read(1))
	a.PCRFlag = byte(reader.Read(1))
	a.OPCRFlag = byte(reader.Read(1))
	a.splicingPointFlag = byte(reader.Read(1))
	a.transportPrivateDataFlag = byte(reader.Read(1))
	a.adaptationFieldExtensionFlag = byte(reader.Read(1))

	// pcr
	if a.PCRFlag == 1 {
		a.pcrBase = int64(reader.Read(33))
		reader.Seek(6)
		a.pcrExt = int(reader.Read(9))
	}

	// ocr
	if a.OPCRFlag == 1 {
		a.opcrBase = int64(reader.Read(33))
		reader.Seek(6)
		a.opcrExt = int(reader.Read(9))
	}

	// splice_countdown
	if a.splicingPointFlag == 1 {
		a.spliceCountdown = byte(reader.Read(8))
	}

	// private data
	if a.transportPrivateDataFlag == 1 {
		length := int(reader.Read(8))
		index := reader.Offset / 8
		a.privateData = data[index : index+length]
	}

	// extension
	if a.adaptationFieldExtensionFlag == 1 {
		extLength := int(reader.Read(8))
		start := reader.Offset

		a.ltwFlag = byte(reader.Read(1))
		a.piecewiseRateFlag = byte(reader.Read(1))
		a.seamlessSpliceFlag = byte(reader.Read(1))

		reader.Seek(5)

		if a.ltwFlag == 1 {
			a.ltwValidFlag = byte(reader.Read(1))
			a.ltwOffset = uint16(reader.Read(15))
		}

		if a.piecewiseRateFlag == 1 {
			reader.Seek(2)
			a.piecewiseRate = int(reader.Read(22))
		}

		if a.seamlessSpliceFlag == 1 {
			a.spliceType = byte(reader.Read(4))
			dtsNextAU := reader.Read(3)
			dtsNextAU <<= 30
			reader.Seek(1)
			dtsNextAU |= reader.Read(15) << 15
			reader.Seek(1)
			dtsNextAU |= reader.Read(15) << 15
			reader.Seek(1)
		}

		if n := extLength - ((reader.Offset - start) / 8); n > 0 {
			a.ExtReservedData = data[reader.Offset/8 : reader.Offset/8+n]
			reader.Seek(n * 8)
		}
	}

	// stuffing
	offset := (reader.Offset / 8) - 1
	if n := int(length) - offset; n > 0 {
		a.stuffingBytes = data[offset : offset+n]
		reader.Seek(n * 8)
	}

	return reader.Offset / 8
}

func (a *AdaptationField) Marshal(data []byte, payloadDataSize int) int {
	if len(a.privateData) > 0 {
		a.transportPrivateDataFlag = 1
	} else {
		a.transportPrivateDataFlag = 0
	}

	writer := bufio.BitsWriter{Data: data, Offset: 8}
	writer.Write(1, uint64(a.discontinuityIndicator))
	writer.Write(1, uint64(a.randomAccessIndicator))
	writer.Write(1, uint64(a.elementaryStreamPriorityIndicator))
	writer.Write(1, uint64(a.PCRFlag))
	writer.Write(1, uint64(a.OPCRFlag))
	writer.Write(1, uint64(a.splicingPointFlag))
	writer.Write(1, uint64(a.transportPrivateDataFlag))
	writer.Write(1, uint64(a.adaptationFieldExtensionFlag))

	if a.PCRFlag == 1 {
		writer.Write(33, uint64(a.pcrBase))
		writer.Write(6, 0)
		writer.Write(9, uint64(a.pcrExt))
	}

	if a.OPCRFlag == 1 {
		writer.Write(33, uint64(a.opcrBase))
		writer.Write(6, 0)
		writer.Write(9, uint64(a.opcrExt))
	}

	if a.splicingPointFlag == 1 {
		writer.Write(8, uint64(a.spliceCountdown))
	}

	if a.transportPrivateDataFlag == 1 {
		length := len(a.privateData)
		writer.Write(8, uint64(length))
		for i := 0; i < length; i++ {
			writer.Write(8, uint64(a.privateData[i]))
		}
	}

	if a.adaptationFieldExtensionFlag == 1 {
		offset := writer.Offset
		writer.Seek(8)
		writer.Write(1, uint64(a.ltwFlag))
		writer.Write(1, uint64(a.piecewiseRateFlag))
		writer.Write(1, uint64(a.seamlessSpliceFlag))
		writer.Write(5, 0)

		if a.ltwFlag == 1 {
			writer.Write(1, uint64(a.ltwValidFlag))
			writer.Write(15, uint64(a.ltwOffset))
		}

		if a.piecewiseRateFlag == 1 {
			writer.Write(2, 0)
			writer.Write(22, uint64(a.ltwOffset))
		}

		if a.seamlessSpliceFlag == 1 {
			writer.Write(4, uint64(a.spliceType))
			writer.Write(3, uint64(a.dtsNextAU>>30&0x7))
			writer.Write(1, 0)
			writer.Write(15, uint64(a.dtsNextAU>>15&0x7FFF))
			writer.Write(1, 0)
			writer.Write(15, uint64(a.dtsNextAU&0x7FFF))
		}

		length := len(a.ExtReservedData)
		if length == 0 {
			for i := 0; i < length; i++ {
				writer.Write(8, uint64(a.ExtReservedData[i]))
			}
		}

		data[offset/8] = byte((writer.Offset - offset) / 8)
	}

	stuffingBytesSize := TsPacketSize - 4 - writer.Offset/8 - payloadDataSize
	if stuffingBytesSize > 0 {
		writer.WriteBytes(stuffingBytes[:stuffingBytesSize])
	}

	data[0] = byte(writer.Offset/8) - 1
	return int(data[0]) + 1
}
