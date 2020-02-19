package lazyhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/dendhi31/stale-request/lazyhttp/cache"
)

// Config is a configuration that will be used when constructing a new instance of Requestor
type Config struct {
	MaxIdleConnection    int
	IdleConnTimeout      time.Duration
	MaxConnectionPerHost int
	HTTPRequestTimeout   time.Duration

	// InsecureSkipVerify controls whether a client verifies the
	// server's certificate chain and host name.
	// This should be used only for testing.
	InsecureSkipVerify bool
	Certificate        *tls.Certificate

	MainTimeout time.Duration

	StorageHostServer    []string
	TempStorageKeyPrefix string
	ExpiryTime           time.Duration
	StorageTimeout       time.Duration
}

type Requestor interface {
	SendRequest(ctx context.Context, url, action string, payload []byte, header map[string]string, key string) (statusCode int, responseBody []byte, err error)
}

// Client will handle http request response by extending go http.Client package
type Client struct {
	HTTPClient  *http.Client
	CacheClient cache.Cacher
	ExpiryTime  time.Duration
	MainTimeOut time.Duration
}

type HTTPResponse struct {
	StatusCode int    `json:"status_code"`
	Body       []byte `json:"body"`
}

// New will construct a customized http client
func New(config Config) (*Client, error) {
	transport := &http.Transport{
		MaxIdleConns:    config.MaxIdleConnection,
		IdleConnTimeout: config.IdleConnTimeout * time.Second,
		MaxConnsPerHost: config.MaxConnectionPerHost,
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
		Timeout:   config.HTTPRequestTimeout * time.Second,
	}

	client := &Client{}
	client.HTTPClient = httpClient

	// initialize cache client and set prefix if not empty
	cacher, err := cache.NewCacheClient(config.StorageHostServer)
	if err != nil {
		return nil, fmt.Errorf("error create cacher: %v", err)
	}
	if config.TempStorageKeyPrefix != "" {
		cacher.SetPrefix(config.TempStorageKeyPrefix)
	}
	client.CacheClient = cacher
	client.ExpiryTime = config.ExpiryTime
	client.MainTimeOut = config.MainTimeout

	return client, nil
}

// getFromRedis Get value from redis based on described Key
func (httprequest *Client) getFromRedis(ctx context.Context, key string, redisChan chan bool, redisResChan chan []byte, redisErrChan chan error) {
	//GET FROM REDIS
	cacheBody, err := httprequest.CacheClient.Get(key)
	if err != nil {
		redisErrChan <- err
		redisChan <- true
		close(redisChan)
		close(redisResChan)
		close(redisErrChan)
		return
	}
	redisResChan <- []byte(cacheBody)
	redisChan <- true
	close(redisChan)
	close(redisResChan)
	close(redisErrChan)
}

// doRequest Do HTTP Request to get response from server
func (httprequest *Client) doRequest(ctx context.Context, httpRequest *http.Request, key string, httpChan chan bool, httpResChan chan []byte, httpErrChan chan error) {
	response, err := httprequest.HTTPClient.Do(httpRequest.WithContext(ctx))
	if err != nil {
		httpErrChan <- err
		httpChan <- true
		close(httpChan)
		close(httpResChan)
		close(httpErrChan)
		return
	}
	responseBody, _ := ioutil.ReadAll(response.Body)

	if response.StatusCode == http.StatusOK {
		httpResponse := HTTPResponse{
			StatusCode: response.StatusCode,
			Body:       responseBody,
		}
		result, err := json.Marshal(httpResponse)
		if err == nil {
			_ = httprequest.CacheClient.Set(key, string(result), httprequest.ExpiryTime)
			httpResChan <- result
		}
	}
	//httpErrChan <- errors.New("test")
	httpChan <- true
	close(httpChan)
	close(httpResChan)
	close(httpErrChan)
}

// SendRequest will hit a defined endpoint and return a response body in byte format
func (httprequest *Client) SendRequest(ctx context.Context, url string, action string, payload []byte, header map[string]string, key string) (int, []byte, error) {
	//codeSig := make(chan int)
	var statusCode int
	var responseBody []byte

	mCtx, cancel := context.WithTimeout(context.Background(), httprequest.MainTimeOut*time.Second)
	defer cancel()

	body := bytes.NewBuffer(payload)
	httpRequest, err := http.NewRequest(action, url, body)
	if err != nil {
		return statusCode, responseBody, err
	}

	for k, v := range header {
		httpRequest.Header.Set(k, v)
	}

	httpChan := make(chan bool)
	redisChan := make(chan bool)

	httpResChan := make(chan []byte)
	redisResChan := make(chan []byte)

	httpErrChan := make(chan error)
	redisErrChan := make(chan error)

	go func() {
		httprequest.doRequest(mCtx, httpRequest, key, httpChan, httpResChan, httpErrChan)
	}()
	go func() {
		httprequest.getFromRedis(mCtx, key, redisChan, redisResChan, redisErrChan)
	}()

	//var response HTTPResponse
	var errRedis error
	var errHttp error
	var redisBody []byte
	var httpBody []byte
	var httpStatus bool
	var redisStatus bool
exit:
	for {
		select {
		case httpBody = <-httpResChan:
			break exit
		case tempRedisBody := <-redisResChan:
			if (len(redisBody) < 1) && (len(tempRedisBody) > 0) {
				redisBody = tempRedisBody
			}
		case tempErrRedis := <-redisErrChan:
			if errRedis == nil {
				errRedis = tempErrRedis
			}
		case tempErrHttp := <-httpErrChan:
			if errHttp == nil {
				errHttp = tempErrHttp
			}
		case httpStatus = <-httpChan:
		case redisStatus = <-redisChan:

		}
		if (redisStatus == true) && (httpStatus == true) {
			break exit
		}
	}

	if errHttp == nil {
		responseBody = httpBody
		if len(responseBody) == 0 {
			err = errors.New("response body is empty")
		}
	} else {
		if errRedis == nil {
			responseBody = redisBody
			err = nil
		} else {
			err = errHttp
		}
	}

	return http.StatusOK, responseBody, err
}
