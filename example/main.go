package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/dendhi31/lazyhttp"
	"golang.org/x/net/context"
)

func main() {
	httpReq, err := lazyhttp.New(lazyhttp.Config{
		ExpiryTime:         10 * time.Minute,
		HTTPRequestTimeout: 1 * time.Second,
		MainTimeout:        1 * time.Second,
		StorageHostServer:  strings.Split("127.0.0.1:5000,127.0.0.1:7001,127.0.0.1:7002", ","),
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	httpCode, body, err := httpReq.SendRequest(context.Background(), "http://localhost:9096/foo", "GET", nil, nil, "test")
	fmt.Println("=====")
	if err != nil {
		fmt.Println("Error: ", err.Error())
	} else {
		fmt.Println("HTTP Code: ", httpCode)
		fmt.Println("Response Body: ", string(body))
	}
}