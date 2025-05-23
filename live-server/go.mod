module github.com/devpospicha/media-server/live-server

require (
	github.com/devpospicha/media-server/flv v0.0.0
	github.com/devpospicha/media-server/mpeg v0.0.0
	github.com/devpospicha/media-server/rtmp v0.0.0
	github.com/devpospicha/media-server/rtp v0.0.0
	github.com/devpospicha/media-server/transport v0.0.0
)

require (
	github.com/devpospicha/media-server/avformat v0.0.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.1
	github.com/natefinch/lumberjack v2.0.0+incompatible
	github.com/pion/interceptor v0.1.25
	github.com/pion/rtcp v1.2.12
	github.com/pion/rtp v1.8.5
	github.com/pion/sdp/v3 v3.0.8
	github.com/pion/webrtc/v3 v3.2.29
	github.com/sirupsen/logrus v1.9.3
	github.com/x-cray/logrus-prefixed-formatter v0.5.2
	go.uber.org/zap v1.27.0
)

require (
	github.com/BurntSushi/toml v1.5.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/pion/datachannel v1.5.5 // indirect
	github.com/pion/dtls/v2 v2.2.7 // indirect
	github.com/pion/ice/v2 v2.3.13 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.12 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.12 // indirect
	github.com/pion/srtp/v2 v2.0.18 // indirect
	github.com/pion/stun v0.6.1 // indirect
	github.com/pion/transport/v2 v2.2.3 // indirect
	github.com/pion/turn/v2 v2.1.3 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/crypto v0.18.0 // indirect
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/term v0.16.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/devpospicha/media-server/avformat => ../avformat

replace github.com/devpospicha/media-server/mpeg => ../mpeg

replace github.com/devpospicha/media-server/flv => ../flv

replace github.com/devpospicha/media-server/rtmp => ../rtmp

replace github.com/devpospicha/media-server/transport => ../transport

replace github.com/devpospicha/media-server/rtp => ../rtp

go 1.24.3
