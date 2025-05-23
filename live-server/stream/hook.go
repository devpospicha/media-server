package stream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/devpospicha/media-server/live-server/log"
)

// 每个通知事件都需要携带的字段
type eventInfo struct {
	Stream     string `json:"stream"`      //stream GetID
	Protocol   string `json:"protocol"`    //推拉流协议
	RemoteAddr string `json:"remote_addr"` //peer地址
}

func responseBodyToString(resp *http.Response) string {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return string(bodyBytes)
}

func SendHookEvent(url string, body []byte) (*http.Response, error) {
	client := &http.Client{
		Timeout: time.Duration(AppConfig.Hooks.Timeout),
	}
	request, err := http.NewRequest("post", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	return client.Do(request)
}

func Hook(event HookEvent, params string, body interface{}) (*http.Response, error) {
	url, ok := hookUrls[event]
	if url == "" || !ok {
		return nil, fmt.Errorf("the url for this %s event does not exist", event.ToString())
	}

	bytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	if "" != params {
		url += "?" + params
	}

	log.Sugar.Infof("sent a hook event for %s. url: %s body: %s", event.ToString(), url, bytes)
	response, err := SendHookEvent(url, bytes)
	if err != nil {
		log.Sugar.Errorf("failed to %s the hook event. err: %s", event.ToString(), err.Error())
	} else {
		log.Sugar.Infof("received response for hook %s event: status='%s', response body='%s'", event.ToString(), response.Status, responseBodyToString(response))
	}

	if err == nil && http.StatusOK != response.StatusCode {
		return response, fmt.Errorf("unexpected response status: %s for request %s", response.Status, url)
	}

	return response, err
}

func NewHookPlayEventInfo(sink Sink) eventInfo {
	return eventInfo{Stream: sink.GetSourceID(), Protocol: sink.GetProtocol().String(), RemoteAddr: sink.RemoteAddr()}
}

func NewHookPublishEventInfo(source Source) eventInfo {
	return eventInfo{Stream: source.GetID(), Protocol: source.GetType().String(), RemoteAddr: source.RemoteAddr()}
}

func NewRecordEventInfo(source Source, path string) interface{} {
	data := struct {
		eventInfo
		Path string `json:"path"`
	}{
		eventInfo: NewHookPublishEventInfo(source),
		Path:      path,
	}

	return data
}
