module github.com/devpospicha/media-server/mpeg

require github.com/devpospicha/media-server/avformat v0.0.0

replace (
	github.com/devpospicha/media-server/avformat => ../avformat
)

go 1.24.3
