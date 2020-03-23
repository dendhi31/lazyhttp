package lazyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type RequestRequirement struct {
	Url     string            `json:"url"`
	Action  string            `json:"action"`
	Payload []byte            `json:"payload"`
	Header  map[string]string `json:"header"`
	Key     string            `json:"key"`
}

func (httprequest *Client) Consumer() {
	//psc := httprequest.CacheClient.Publish()
}

func (httprequest *Client) SendRequestWithPubSub(ctx context.Context, url string, action string, payload []byte, header map[string]string, key string) (int, []byte, error) {
	var responseBody []byte
	var err error
	var code int

	mCtx, cancel := context.WithTimeout(context.Background(), httprequest.WaitHttp*time.Second)
	defer cancel()

	redisChan := make(chan redisChannel, 1)
	httpChan := make(chan httpChannel, 1)

	go func() {
		httprequest.getFromRedis(mCtx, key, redisChan)
	}()

	var redisResult redisChannel
	var httpResult httpChannel
	select {
	case redisTempResult := <-redisChan:
		if (redisChannel{}) != redisTempResult {
			redisResult = redisTempResult
		}
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
	go func() {
		httprequest.doRequest(mCtx, httpRequest, key, httpChan)
	}()
exit:
	for {
		select {
		case <-mCtx.Done():
			log.Println("HTTP wait got timeout", int(httprequest.WaitHttp))
			err = errors.New("context timeout HTTP")
			fmt.Println("Set error http")
			break exit
		case httpResult = <-httpChan:
			responseBody = httpResult.ResultChan
			code = http.StatusOK
			err = httpResult.ErrorChan
			break exit
		}
	}
	fmt.Println("kampret")
	if err != nil {
		//publish to redis
		fmt.Println("asu")
		reqRequirement := RequestRequirement{
			Url:     url,
			Action:  action,
			Payload: payload,
			Header:  header,
			Key:     key,
		}
		reqJson, err := json.Marshal(reqRequirement)
		if err != nil {
			log.Println("Error encode json: ", err.Error())
			return http.StatusInternalServerError, responseBody, err
		}
		fmt.Println("masuk nyet")
		err2 := httprequest.CacheClient.Publish(httprequest.Channel, reqJson)
		fmt.Println(err2)
		return http.StatusInternalServerError, responseBody, err
	}
	return code, responseBody, err
}
