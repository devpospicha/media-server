package mpeg

import "github.com/devpospicha/media-server/avformat/bufio"

type Section struct {
	tableId                byte
	sectionSyntaxIndicator byte
	sectionLength          byte
	transportStreamId      uint16 // pat-transport_stream_id/pmt-program_number
	versionNumber          byte
	currentNextIndicator   byte
	sectionNumber          byte
	lastSectionNumber      byte
	crc32                  uint32
}

func (s *Section) Unmarshal(data []byte) int {
	reader := bufio.BitsReader{Data: data}
	n := int(reader.Read(8)) // pointer_field
	reader.Seek(n * 8)
	s.tableId = byte(reader.Read(8))
	s.sectionSyntaxIndicator = byte(reader.Read(1))
	reader.Seek(3)
	s.sectionLength = byte(reader.Read(12)) & 0x3F
	s.transportStreamId = uint16(reader.Read(16))
	reader.Seek(2)
	s.versionNumber = byte(reader.Read(5))
	s.currentNextIndicator = byte(reader.Read(1))
	s.sectionNumber = byte(reader.Read(8))
	s.lastSectionNumber = byte(reader.Read(8))
	return reader.Offset / 8
}

func (s *Section) Marshal(dst []byte) int {
	s.sectionSyntaxIndicator = 1 // 需要为1
	s.currentNextIndicator = 1   // 当前表有效
	writer := bufio.BitsWriter{Data: dst}
	writer.Write(8, 0) // pointer_field
	writer.Write(8, uint64(s.tableId))
	writer.Write(1, uint64(s.sectionSyntaxIndicator))
	writer.Write(3, 0)
	writer.Write(12, uint64(s.sectionLength))
	writer.Write(16, uint64(s.transportStreamId))
	writer.Write(2, 0)
	writer.Write(5, uint64(s.versionNumber))
	writer.Write(1, uint64(s.currentNextIndicator))
	writer.Write(8, uint64(s.sectionNumber))
	writer.Write(8, uint64(s.lastSectionNumber))
	return 9
}
