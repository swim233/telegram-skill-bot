package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/swim233/chat_bot/lib"
	"github.com/swim233/logger"
	"resty.dev/v3"
)

var Models lib.Models

func InitRestyClient() error {
	route, err := resolveRouteByScene("skill")
	if err != nil {
		logger.Warn("resolve default route failed, skip models preload: %s", err.Error())
		return nil
	}
	if err := initModelsList(&Models, route); err != nil {
		logger.Warn("preload models failed, will retry on demand: %s", err.Error())
	}
	return nil
}

func initModelsList(models *lib.Models, route RequestRoute) error {
	if models == nil {
		return nil
	}
	*models = lib.Models{}

	loaded := lib.Models{}
	base := strings.TrimRight(strings.TrimSpace(route.Endpoint), "/")
	url := base + "/models"
	start := time.Now()
	logger.Info("HTTP request start: method=GET url=%s", url)

	client := resty.New().SetAuthToken(route.Token)
	rsp, err := client.R().
		SetContentType("application/json").
		SetResult(&loaded).
		Get(url)

	if err != nil {
		logger.Error("HTTP request failed: method=GET url=%s elapsed=%s err=%s", url, time.Since(start), err.Error())
		return err
	}
	logger.Info("HTTP request done: method=GET url=%s status=%d elapsed=%s", url, rsp.StatusCode(), time.Since(start))
	if rsp.StatusCode() != http.StatusOK {
		return fmt.Errorf("get models failed, status=%d, body=%s", rsp.StatusCode(), rsp.String())
	}
	*models = loaded

	logger.Info("models loaded, count=%d", len(models.Data))

	return nil
}
