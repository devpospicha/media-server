module github.com/devpospicha/media-server/rtmp

require (
	github.com/devpospicha/media-server/avformat v0.0.0
	github.com/devpospicha/media-server/flv v0.0.0
	github.com/devpospicha/media-server/transport v0.0.0
)

replace (
	github.com/devpospicha/media-server/avformat => ../avformat
	github.com/devpospicha/media-server/flv => ../flv
	github.com/devpospicha/media-server/transport => ../transport
)

go 1.24.3
