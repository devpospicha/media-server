module github.com/devpospicha/media-server/rtp

require github.com/devpospicha/media-server/avformat v0.0.0

require github.com/devpospicha/media-server/transport v0.0.0

replace (
	github.com/devpospicha/media-server/avformat => ../avformat
	github.com/devpospicha/media-server/transport => ../transport
)

go 1.19
