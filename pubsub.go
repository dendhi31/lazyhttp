package lazyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dendhi31/lazyhttp/redismaint"
)

func (httprequest *Client) Consumer() {
	config := redismaint.Configuration{
		RedisURL:   httprequest.PubSubServer,
		ContexName: "first",
		Debug:      true,
	}

	rmaint, err := redismaint.New(config)
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Println("starting the lazyhttp consumer")
		rmaint.Run()
	}()
	//send sample schedule
	select {
	case <-rmaint.Err():
		log.Println("err", err)
	case <-term:
		rmaint.Stop()
		log.Println("signal terminated detected")
	}
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
			log.Println("Error encode json: ", err.Error())
			return http.StatusInternalServerError, responseBody, err
		}
		err2 := httprequest.PubsubClient.Publish(httprequest.Channel, reqJson)
		if err2 != nil {
			log.Println("Error publish message: ", err2.Error())
		}
		return http.StatusInternalServerError, responseBody, err
	}
	return code, responseBody, err
}
