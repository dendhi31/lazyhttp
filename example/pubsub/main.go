package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/dendhi31/lazyhttp"
	"golang.org/x/net/context"
)

func main() {
	httpReq, err := lazyhttp.New(lazyhttp.Config{
		ExpiryTime:         600, //In second
		HTTPRequestTimeout: 1,   //In second
		WaitHttp:           1,   //In second
		StorageHostServer:  strings.Split("127.0.0.1:5000,127.0.0.1:7001,127.0.0.1:7002", ","),
		StorageDB:          1,
		Channel:            "first",
		RedisHost:          "localhost:6379",
		Debug:              true,
	})
	if err != nil {
		log.Fatal(err)
		fmt.Println(err.Error())
	}
	httpCode, body, err := httpReq.SendRequest(context.Background(), "http://localhost:9096/foo", "GET", nil, nil, "test", true)
	fmt.Println("=====")
	if err != nil {
		fmt.Println("Error: ", err.Error())
	} else {
		fmt.Println("HTTP Code: ", httpCode)
		fmt.Println("Response Body: ", string(body))
	}
}
