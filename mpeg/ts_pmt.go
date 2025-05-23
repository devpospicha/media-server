package mpeg

import "github.com/devpospicha/media-server/avformat/bufio"

type PMSection struct {
	Section
	pcrPID      int
	programInfo []byte
	streams     []struct {
		streamType    int
		elementaryPID int // TS包PID
		esInfo        []byte
	}
}

func (p *PMSection) Unmarshal(data []byte) int {
	n := p.Section.Unmarshal(data)

	reader := bufio.BitsReader{Data: data, Offset: n * 8}
	reader.Seek(3)
	p.pcrPID = int(reader.Read(13))
	reader.Seek(4)

	programInfoLength := int(reader.Read(12)) & 0x3FF
	if programInfoLength > 0 {
		p.programInfo = reader.ReadBytes(programInfoLength)
	}

	length := int(p.Section.sectionLength) - 13 + programInfoLength
	for i := 0; i < length; i += 5 {
		streamType := reader.Read(8)
		reader.Seek(3)
		elementaryPID := reader.Read(13)
		reader.Seek(4)

		var esInfo []byte
		esInfoLength := int(reader.Read(12))
		if esInfoLength > 0 {
			esInfo = reader.ReadBytes(esInfoLength)
		}

		p.streams = append(p.streams, struct {
			streamType    int
			elementaryPID int
			esInfo        []byte
		}{streamType: int(streamType), elementaryPID: int(elementaryPID), esInfo: esInfo})

		i += esInfoLength
	}

	p.crc32 = uint32(reader.Read(32))
	return reader.Offset / 8
}

func (p *PMSection) Marshal(dst []byte) int {
	n := 9
	writer := bufio.BitsWriter{Data: dst, Offset: n * 8}
	writer.Write(3, 0)
	writer.Write(13, uint64(p.streams[0].elementaryPID))
	writer.Write(4, 0)

	length := len(p.programInfo)
	writer.Write(12, uint64(length))
	if len(p.programInfo) > 0 {
		writer.WriteBytes(p.programInfo)
	}

	for _, stream := range p.streams {
		writer.Write(8, uint64(stream.streamType))
		writer.Write(3, 0)
		writer.Write(13, uint64(stream.elementaryPID))
		writer.Write(4, 0)
		esInfoLength := len(stream.esInfo)
		writer.Write(12, uint64(esInfoLength))
		if esInfoLength > 0 {
			writer.WriteBytes(stream.esInfo)
		}
	}

	p.Section.sectionLength = byte(writer.Offset / 8)
	p.Section.tableId = TableIdPMS
	p.Section.transportStreamId = 1
	_ = p.Section.Marshal(dst[:])

	crc32 := CalculateCrcMpeg2(dst[1 : writer.Offset/8])
	writer.Write(32, uint64(crc32))
	return writer.Offset / 8
}
