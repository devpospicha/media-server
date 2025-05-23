package record

import (
	"os"
	"path/filepath"
	"time"

	"github.com/devpospicha/media-server/avformat/collections"
	"github.com/devpospicha/media-server/live-server/stream"
)

type FLVFileSink struct {
	stream.BaseSink
	file *os.File
	fail bool
}

// Input 输入http-flv数据
func (f *FLVFileSink) Write(index int, blocks []*collections.ReferenceCounter[[]byte], ts int64, keyVideo bool) error {
	if f.fail {
		return nil
	}

	for _, data := range blocks {
		// 去掉不需要的换行符
		var offset int
		bytes := data.Get()
		for i := 2; i < len(bytes); i++ {
			if bytes[i-2] == 0x0D && bytes[i-1] == 0x0A {
				offset = i
				break
			}
		}

		_, err := f.file.Write(bytes[offset : len(bytes)-2])
		if err != nil {
			// 只要写入失败一次，后续不再允许写入, 不影响拉流
			f.fail = true
			return err
		}
	}

	return nil
}

func (f *FLVFileSink) Close() {
	if f.file != nil {
		f.file.Close()
		f.file = nil
	}
}

// NewFLVFileSink 创建FLV文件录制流Sink
// 保存path: dir/sourceId/yyyy-MM-dd/HH-mm-ss.flv
func NewFLVFileSink(sourceId string) (stream.Sink, string, error) {
	now := time.Now().Format("2006-01-02/15-04-05")
	path := filepath.Join(stream.AppConfig.Record.Dir, sourceId, now+".flv")

	// 创建目录
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0666); err != nil {
		return nil, "", err
	}

	// 创建flv文件
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, "", err
	}

	return &FLVFileSink{
		BaseSink: stream.BaseSink{ID: "record-sink-flv", SourceID: sourceId, Protocol: stream.TransStreamFlv, Ready: true},
		file:     file,
	}, path, nil
}
