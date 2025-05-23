package main

import (
	"encoding/json"
	"os"

	"net"
	"net/http"
	_ "net/http/pprof"
	"strconv"

	"github.com/devpospicha/media-server/live-server/flv"
	"github.com/devpospicha/media-server/live-server/gb28181"
	"github.com/devpospicha/media-server/live-server/hls"
	"github.com/devpospicha/media-server/live-server/jt1078"
	"github.com/devpospicha/media-server/live-server/log"
	"github.com/devpospicha/media-server/live-server/record"
	"github.com/devpospicha/media-server/live-server/rtc"
	"github.com/devpospicha/media-server/live-server/rtmp"
	"github.com/devpospicha/media-server/live-server/rtsp"
	"github.com/devpospicha/media-server/live-server/stream"
	"github.com/devpospicha/media-server/transport"

	"go.uber.org/zap/zapcore"
)

func init() {
	stream.RegisterTransStreamFactory(stream.TransStreamRtmp, rtmp.TransStreamFactory)
	stream.RegisterTransStreamFactory(stream.TransStreamHls, hls.TransStreamFactory)
	stream.RegisterTransStreamFactory(stream.TransStreamFlv, flv.TransStreamFactory)
	//stream.RegisterTransStreamFactory(stream.TransStreamRtsp, rtsp.TransStreamFactory)
	stream.RegisterTransStreamFactory(stream.TransStreamRtc, rtc.TransStreamFactory)
	//stream.RegisterTransStreamFactory(stream.TransStreamGBCascadedForward, gb28181.CascadedTransStreamFactory)
	//	stream.RegisterTransStreamFactory(stream.TransStreamGBTalkForward, gb28181.TalkTransStreamFactory)
	stream.SetRecordStreamFactory(record.NewFLVFileSink)
	stream.StreamEndInfoBride = NewStreamEndInfo

	config, err := stream.LoadConfigFile("./config.json")
	if err != nil {
		panic(err)
	}

	stream.SetDefaultConfig(config)

	options := map[string]stream.EnableConfig{
		"rtmp":    &config.Rtmp,
		"rtsp":    &config.Rtsp,
		"hls":     &config.Hls,
		"webrtc":  &config.WebRtc,
		"gb28181": &config.GB28181,
		"jt1078":  &config.JT1078,
		"hooks":   &config.Hooks,
		"record":  &config.Record,
	}

	// 读取运行参数
	disableOptions, enableOptions := readRunArgs()
	mergeArgs(options, disableOptions, enableOptions)

	stream.AppConfig = *config

	if stream.AppConfig.Hooks.Enable {
		stream.InitHookUrls()
	}

	if stream.AppConfig.WebRtc.Enable {
		// 设置公网IP和端口
		rtc.InitConfig()
	}

	// 初始化日志
	log.InitLogger(config.Log.FileLogging, zapcore.Level(stream.AppConfig.Log.Level), stream.AppConfig.Log.Name, stream.AppConfig.Log.MaxSize, stream.AppConfig.Log.MaxBackup, stream.AppConfig.Log.MaxAge, stream.AppConfig.Log.Compress)

	if stream.AppConfig.GB28181.Enable && stream.AppConfig.GB28181.IsMultiPort() {
		gb28181.TransportManger = transport.NewTransportManager(config.ListenIP, uint16(stream.AppConfig.GB28181.Port[0]), uint16(stream.AppConfig.GB28181.Port[1]))
	}

	if stream.AppConfig.Rtsp.Enable && stream.AppConfig.Rtsp.IsMultiPort() {
		rtsp.TransportManger = transport.NewTransportManager(config.ListenIP, uint16(stream.AppConfig.Rtsp.Port[1]), uint16(stream.AppConfig.Rtsp.Port[2]))
	}

	// 创建dump目录
	if stream.AppConfig.Debug {
		if err := os.MkdirAll("dump", 0755); err != nil {
			panic(err)
		}
	}

	// 打印配置信息
	indent, _ := json.MarshalIndent(stream.AppConfig, "", "\t")
	log.Sugar.Infof("server config:\r\n%s", indent)
}

func main() {
	if stream.AppConfig.Rtmp.Enable {
		rtmpAddr, err := net.ResolveTCPAddr("tcp", stream.ListenAddr(stream.AppConfig.Rtmp.Port))
		if err != nil {
			panic(err)
		}

		server := rtmp.NewServer()
		err = server.Start(rtmpAddr)
		if err != nil {
			panic(err)
		}

		log.Sugar.Info("Start rtmp service successfully addr:", rtmpAddr.String())
	}

	if stream.AppConfig.Rtsp.Enable {
		rtspAddr, err := net.ResolveTCPAddr("tcp", stream.ListenAddr(stream.AppConfig.Rtsp.Port[0]))
		if err != nil {
			panic(rtspAddr)
		}

		server := rtsp.NewServer(stream.AppConfig.Rtsp.Password)
		err = server.Start(rtspAddr)
		if err != nil {
			panic(err)
		}

		log.Sugar.Info("Start rtsp service successfully addr:", rtspAddr.String())
	}

	log.Sugar.Info("Start http service addr:", stream.ListenAddr(stream.AppConfig.Http.Port))
	go startApiServer(net.JoinHostPort(stream.AppConfig.ListenIP, strconv.Itoa(stream.AppConfig.Http.Port)))

	// 单端口模式下, 启动时就创建收流端口
	// 多端口模式下, 创建GBSource时才创建收流端口
	if stream.AppConfig.GB28181.Enable && !stream.AppConfig.GB28181.IsMultiPort() {
		if stream.AppConfig.GB28181.IsEnableUDP() {
			filter := gb28181.NewSSRCFilter(128)
			server, err := gb28181.NewUDPServer(filter)
			if err != nil {
				panic(err)
			}

			gb28181.SharedUDPServer = server
			log.Sugar.Info("Start GB28181 udp receiving port successfully:" + stream.ListenAddr(stream.AppConfig.GB28181.Port[0]))
			gb28181.SSRCFilters = append(gb28181.SSRCFilters, filter)
		}

		if stream.AppConfig.GB28181.IsEnableTCP() {
			filter := gb28181.NewSSRCFilter(128)
			server, err := gb28181.NewTCPServer(filter)
			if err != nil {
				panic(err)
			}

			gb28181.SharedTCPServer = server
			log.Sugar.Info("Start GB28181 TCP receiving port successfully:" + stream.ListenAddr(stream.AppConfig.GB28181.Port[0]))
			gb28181.SSRCFilters = append(gb28181.SSRCFilters, filter)
		}
	}

	if stream.AppConfig.JT1078.Enable {
		jtAddr, err := net.ResolveTCPAddr("tcp", stream.ListenAddr(stream.AppConfig.JT1078.Port))
		if err != nil {
			panic(err)
		}

		server := jt1078.NewServer()
		err = server.Start(jtAddr)
		if err != nil {
			panic(err)
		}

		log.Sugar.Info("Start jt1078 service successfully addr:", jtAddr.String())
	}

	if stream.AppConfig.Hooks.IsEnableOnStarted() {
		go func() {
			_, _ = stream.Hook(stream.HookEventStarted, "", nil)
		}()
	}

	// 开启pprof调试
	err := http.ListenAndServe(":19999", nil)
	if err != nil {
		println(err)
	}

	select {}
}
