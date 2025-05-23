package stream

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/devpospicha/media-server/avformat/utils"
	"github.com/devpospicha/media-server/live-server/log"
)

func PreparePublishSource(source Source, hook bool) (*http.Response, utils.HookState) {
	var response *http.Response

	if hook && AppConfig.Hooks.IsEnablePublishEvent() {
		rep, state := HookPublishEvent(source)
		if utils.HookStateOK != state {
			return rep, state
		}

		response = rep
	}

	if err := SourceManager.Add(source); err != nil {
		return nil, utils.HookStateOccupy
	}

	source.SetCreateTime(time.Now())

	urls := GetStreamPlayUrls(source.GetID())
	indent, _ := json.MarshalIndent(urls, "", "\t")
	log.Sugar.Infof("%Ready to push stream source:%s pull address:\r\n%s", source.GetType().String(), source.GetID(), indent)

	return response, utils.HookStateOK
}

func HookPublishEvent(source Source) (*http.Response, utils.HookState) {
	var response *http.Response

	if AppConfig.Hooks.IsEnablePublishEvent() {
		hook, err := Hook(HookEventPublish, source.UrlValues().Encode(), NewHookPublishEventInfo(source))
		if err != nil {
			return hook, utils.HookStateFailure
		}

		response = hook
	}

	return response, utils.HookStateOK
}

func HookPublishDoneEvent(source Source) {
	if AppConfig.Hooks.IsEnablePublishEvent() {
		_, _ = Hook(HookEventPublishDone, source.UrlValues().Encode(), NewHookPublishEventInfo(source))
	}
}

func HookReceiveTimeoutEvent(source Source) (*http.Response, error) {
	utils.Assert(AppConfig.Hooks.IsEnableOnReceiveTimeout())
	return Hook(HookEventReceiveTimeout, source.UrlValues().Encode(), NewHookPublishEventInfo(source))
}

func HookIdleTimeoutEvent(source Source) (*http.Response, error) {
	utils.Assert(AppConfig.Hooks.IsEnableOnIdleTimeout())
	return Hook(HookEventIdleTimeout, source.UrlValues().Encode(), NewHookPublishEventInfo(source))
}

func HookRecordEvent(source Source, path string) {
	if AppConfig.Hooks.IsEnableOnRecord() {
		_, _ = Hook(HookEventRecord, "", NewRecordEventInfo(source, path))
	}
}
