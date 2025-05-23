package mpeg

import (
	"os"
	"testing"

	"github.com/devpospicha/media-server/avformat/bufio"
)

func TestTSPacket_Marshal(t *testing.T) {
	files := []string{
		"output.ts",
	}

	for _, path := range files {
		file, err := os.ReadFile("../source_files/" + path)
		if err != nil {
			panic(err)
		}

		pat := PASection{}
		pmt := PMSection{}
		for i := 0; i < len(file); {
			packet := TSPacket{}
			n := bufio.MinInt(i+188, len(file))
			bytes := file[i:n]
			packet.Unmarshal(bytes)
			i = n

			if PsiPat == packet.pid {
				_ = pat.Unmarshal(packet.data)
			} else {
				for _, pmtPID := range pat.programMapPID {
					if packet.pid == pmtPID {
						pmt.Unmarshal(packet.data)
					}
				}
			}
		}
	}
}
