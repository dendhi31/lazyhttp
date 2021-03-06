package lazyhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dendhi31/lazyhttp/cache"
	"github.com/dendhi31/lazyhttp/logger"
)

// Config is a configuration that will be used when constructing a new instance of Requestor
type Config struct {
	MaxIdleConnection    int
	IdleConnTimeout      time.Duration
	MaxConnectionPerHost int

	// InsecureSkipVerify controls whether a client verifies the
	// server's certificate chain and host name.
	// This should be used only for testing.
	InsecureSkipVerify bool
	Certificate        *tls.Certificate

	MainTimeout        time.Duration
	WaitHttp           time.Duration
	WaitRedis          time.Duration
	HTTPRequestTimeout time.Duration

	RedisHost string

	StorageHostServer    []string
	StorageDB            int
	TempStorageKeyPrefix string
	ExpiryTime           time.Duration
	StorageTimeout       time.Duration
	Channel              string

	Debug bool
}

//type Requestor interface {
//	SendRequest(ctx context.Context, url, action string, payload []byte, header map[string]string, key string) (statusCode int, responseBody []byte, err error)
//}

// Client will handle http request response by extending go http.Client package
type Client struct {
	HTTPClient         *http.Client
	CacheClient        cache.Cacher
	PubsubClient       cache.Cacher
	ExpiryTime         time.Duration
	MainTimeOut        time.Duration
	WaitHttp           time.Duration
	WaitRedis          time.Duration
	HTTPRequestTimeout time.Duration
	Channel            string
	PubSubServer       string
	Logger             logger.Logger
}

type httpChannel struct {
	ResultChan []byte
	ErrorChan  error
}

type redisChannel struct {
	ResultChan string
	ErrorChan  error
}

type HTTPResponse struct {
	//StatusCode int    `json:"status_code"`
	Body string `json:"body"`
}

// New will construct a customized http client
func New(config Config) (*Client, error) {
	transport := &http.Transport{
		MaxIdleConns:    config.MaxIdleConnection,
		IdleConnTimeout: config.IdleConnTimeout * time.Millisecond,
		//MaxConnsPerHost: config.MaxConnectionPerHost,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
			Renegotiation:      tls.RenegotiateFreelyAsClient,
		},
	}
	if config.Certificate != nil {
		transport.TLSClientConfig.Certificates = []tls.Certificate{*config.Certificate}
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.HTTPRequestTimeout * time.Millisecond,
	}

	client := &Client{}
	client.HTTPClient = httpClient

	// initialize cache client and set prefix if not empty
	cacher, err := cache.NewCacheClient(config.StorageHostServer, config.StorageDB)
	if err != nil {
		return nil, fmt.Errorf("error create cacher: %v", err)
	}
	if config.TempStorageKeyPrefix != "" {
		cacher.SetPrefix(config.TempStorageKeyPrefix)
	}

	pubServer, err := cache.NewCacheClient([]string{config.RedisHost}, config.StorageDB)
	if err != nil {
		return nil, fmt.Errorf("error create cacher: %v", err)
	}
	if config.TempStorageKeyPrefix != "" {
		cacher.SetPrefix(config.TempStorageKeyPrefix)
	}
	client.CacheClient = cacher
	client.PubsubClient = pubServer

	client.ExpiryTime = config.ExpiryTime
	client.MainTimeOut = config.MainTimeout
	client.WaitHttp = config.WaitHttp
	if config.WaitRedis < 1 {
		config.WaitRedis = 500
	}
	client.WaitRedis = config.WaitRedis
	client.HTTPRequestTimeout = config.HTTPRequestTimeout
	client.Channel = config.Channel
	client.PubSubServer = config.RedisHost
	client.Logger = logger.New(logger.Config{Debug: config.Debug})
	log.SetOutput(os.Stdout)
	return client, nil
}

// getFromRedis Get value from redis based on described Key
func (httprequest *Client) getFromRedis(ctx context.Context, key string, redisChan chan redisChannel) {
	//GET FROM REDIS
	var redisChanStruct redisChannel
	httprequest.Logger.Debugln("Start request via redis")
	cacheBody, err := httprequest.CacheClient.Get(key)
	httprequest.Logger.Debugln("Done request via redis")
	if err != nil {
		redisChanStruct = redisChannel{
			ErrorChan:  err,
			ResultChan: "",
		}
	} else {
		httprequest.Logger.Debugln("Response via Redis", cacheBody)
		redisChanStruct = redisChannel{
			ErrorChan:  nil,
			ResultChan: cacheBody,
		}
	}
	redisChan <- redisChanStruct
	httprequest.Logger.Debugln("Done set redis channel value, ", redisChanStruct)
	close(redisChan)
}

// doRequest Do HTTP Request to get response from server
func (httprequest *Client) doRequest(ctx context.Context, httpRequest *http.Request, key string, httpChan chan httpChannel) {
	ctx, cancelHttp := context.WithTimeout(context.Background(), httprequest.HTTPRequestTimeout*time.Millisecond)
	defer cancelHttp()

	var httpChanStruct httpChannel

	response, err := httprequest.HTTPClient.Do(httpRequest.WithContext(ctx))
	httprequest.Logger.Debugln("Done request via HTTP: ", response)
	if err != nil {
		httprequest.Logger.Debugln("Error request via HTTP: ", err.Error())
		httpChanStruct.ErrorChan = err
		httpChan <- httpChanStruct
		close(httpChan)
		return
	}
	responseBody, _ := ioutil.ReadAll(response.Body)
	httprequest.Logger.Debugln("Response via HTTP", string(responseBody))
	if response.StatusCode == http.StatusOK {
		_ = httprequest.CacheClient.Set(key, string(responseBody), httprequest.ExpiryTime*time.Millisecond)
		httpChanStruct.ResultChan = responseBody
	}
	httpChan <- httpChanStruct
	httprequest.Logger.Debugln("done set http channel value")
	close(httpChan)
}

func (httprequest *Client) SendRequest(ctx context.Context, url string, action string, payload []byte, header map[string]string, key string, useCache bool) (code int, body []byte, err error) {
	if useCache {
		return httprequest.optimisticReq(ctx, url, action, payload, header, key)
	}
	return httprequest.pessimisticReq(ctx, url, action, payload, header, key)
}

// SendRequest will hit a defined endpoint and return a response body in byte format
func (httprequest *Client) pessimisticReq(ctx context.Context, url string, action string, payload []byte, header map[string]string, key string) (int, []byte, error) {

	var responseBody []byte

	mCtx, cancel := context.WithTimeout(context.Background(), httprequest.WaitHttp*time.Millisecond)
	defer cancel()

	body := bytes.NewBuffer(payload)
	httpRequest, err := http.NewRequest(action, url, body)
	if err != nil {
		return 0, responseBody, err
	}

	for k, v := range header {
		httpRequest.Header.Set(k, v)
	}

	httpChan := make(chan httpChannel, 1)
	redisChan := make(chan redisChannel, 1)

	go func() {
		httprequest.doRequest(mCtx, httpRequest, key, httpChan)
	}()

	go func() {
		httprequest.getFromRedis(mCtx, key, redisChan)
	}()

	var httpResult httpChannel
	var redisResult redisChannel
exit:
	for {
		select {
		case <-mCtx.Done():
			httprequest.Logger.Debugln("HTTP wait got timeout", int(httprequest.WaitHttp))
			httpResult.ErrorChan = errors.New("context timeout HTTP")
			httprequest.Logger.Debugln("Set error http")
			break exit
		case httpResult = <-httpChan:
			if httpResult.ErrorChan == nil {
				break exit
			} else {
				if redisResult.ErrorChan == nil {
					break exit
				}
			}
		case redisTempResult := <-redisChan:
			if (redisChannel{}) != redisTempResult {
				redisResult = redisTempResult
			}
			if httpResult.ErrorChan != nil {
				break exit
			}

		}
	}

	var code int

	if httpResult.ErrorChan == nil {
		responseBody = httpResult.ResultChan
		if len(responseBody) == 0 {
			err = errors.New("Response body is empty")
			code = http.StatusInternalServerError
		} else {
			code = http.StatusOK
		}
	} else {
		if (redisResult.ErrorChan == nil) && (redisResult.ResultChan != "") {
			responseBody = []byte(redisResult.ResultChan)
			err = nil
			code = http.StatusOK
		} else {
			err = httpResult.ErrorChan
			code = http.StatusInternalServerError
		}
	}

	return code, responseBody, err
}
