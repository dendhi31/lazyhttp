package lazyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dendhi31/lazyhttp/redismaint"
)

func (httprequest *Client) Consumer() error {
	config := redismaint.Configuration{
		RedisURL:   httprequest.PubSubServer,
		ContexName: "first",
		Logger:     httprequest.Logger,
		Handler:    httprequest.optimisticReq,
	}

	rmaint, err := redismaint.New(config)
	if err != nil {
		return err
	}

	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func(r *redismaint.Consumer) {
		httprequest.Logger.Debugln("lazyhttp consumer started")
		r.Run()
	}(rmaint)
	//send sample schedule
	select {
	case <-rmaint.Err():
		return err
	case <-term:
		httprequest.Logger.Debugln("lazyhttp consumer stopped")
		rmaint.Stop()
		return nil
	}
}

func (httprequest *Client) optimisticReq(ctx context.Context, url string, action string, payload []byte, header map[string]string, key string) (int, []byte, error) {
	var responseBody []byte
	var err error
	var code int

	mCtx, cancel := context.WithTimeout(context.Background(), httprequest.WaitHttp*time.Millisecond)
	defer cancel()

	redisCtx, cancelRedis := context.WithTimeout(context.Background(), httprequest.WaitRedis*time.Millisecond)
	defer cancelRedis()

	redisChan := make(chan redisChannel, 1)
	httpChan := make(chan httpChannel, 1)

	go func(ctx context.Context, client *Client, key string, channel chan redisChannel) {
		client.getFromRedis(ctx, key, channel)
	}(redisCtx, httprequest, key, redisChan)

	var redisResult redisChannel
	var httpResult httpChannel
	select {
	case <-redisCtx.Done():
		httprequest.Logger.Debugln("Redis wait got timeout", int(httprequest.WaitRedis))
		err = errors.New("context timeout redis")
		redisResult.ErrorChan = err
		break
	case redisTempResult := <-redisChan:
		if (redisChannel{}) != redisTempResult {
			redisResult = redisTempResult
		}
		break
	}

	if (redisResult.ErrorChan == nil) && (redisResult.ResultChan != "") {
		return http.StatusOK, []byte(redisResult.ResultChan), nil
	}

	body := bytes.NewBuffer(payload)
	httpRequest, err := http.NewRequest(action, url, body)
	if err != nil {
		return 0, responseBody, err
	}
	for k, v := range header {
		httpRequest.Header.Set(k, v)
	}
	go func(ctx context.Context, http *http.Request, key string, channel chan httpChannel) {
		httprequest.doRequest(ctx, http, key, channel)
	}(mCtx, httpRequest, key, httpChan)
exit:
	for {
		select {
		case <-mCtx.Done():
			httprequest.Logger.Debugln("HTTP wait got timeout", int(httprequest.WaitHttp))
			err = errors.New("context timeout HTTP")
			break exit
		case httpResult = <-httpChan:
			responseBody = httpResult.ResultChan
			code = http.StatusOK
			err = httpResult.ErrorChan
			break exit
		}
	}
	if err != nil {
		//publish to redis
		reqRequirement := redismaint.RequestRequirement{
			Url:     url,
			Action:  action,
			Payload: payload,
			Header:  header,
			Key:     key,
		}
		reqJson, err := json.Marshal(reqRequirement)
		if err != nil {
			httprequest.Logger.Debugln("Error encode json: ", err.Error())
			return 0, responseBody, err
		}
		err2 := httprequest.PubsubClient.Publish(httprequest.Channel, reqJson)
		if err2 != nil {
			httprequest.Logger.Debugln("Error publish message: ", err2.Error())
		}
		return 0, responseBody, err
	}
	return code, responseBody, err
}
