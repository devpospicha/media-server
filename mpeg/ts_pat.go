package mpeg

import (
	"github.com/devpospicha/media-server/avformat/bufio"
)

type PASection struct {
	Section

	networkPID    []int
	programMapPID []int
}

func (p *PASection) Unmarshal(data []byte) int {
	n := p.Section.Unmarshal(data)

	reader := bufio.BitsReader{Data: data, Offset: n * 8}
	length := int(p.Section.sectionLength - 9)
	for i := 0; i < length; i += 4 {
		programNumber := reader.Read(16)
		reader.Seek(3)
		if programNumber == 0 {
			p.networkPID = append(p.networkPID, int(reader.Read(13)))
		} else {
			p.programMapPID = append(p.programMapPID, int(reader.Read(13)))
		}
	}

	p.crc32 = uint32(reader.Read(32))
	return reader.Offset / 8
}

func (p *PASection) Marshal(dst []byte) int {
	n := 9
	writer := bufio.BitsWriter{Data: dst, Offset: n * 8}
	for _, pid := range p.networkPID {
		writer.Write(16, 0)
		writer.Write(3, 0)
		writer.Write(13, uint64(pid))
	}

	for _, pid := range p.programMapPID {
		writer.Write(16, 1)
		writer.Write(3, 0)
		writer.Write(13, uint64(pid))
	}

	p.Section.sectionLength = byte(writer.Offset / 8)
	p.Section.tableId = TableIdPAS
	_ = p.Section.Marshal(dst)

	crc32 := CalculateCrcMpeg2(dst[1 : writer.Offset/8])
	writer.Write(32, uint64(crc32))
	return writer.Offset / 8
}
