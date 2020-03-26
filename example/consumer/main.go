package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/dendhi31/lazyhttp"
)

func main() {
	httpReq, err := lazyhttp.New(lazyhttp.Config{
		ExpiryTime:         600, //In second
		HTTPRequestTimeout: 10,  //In second
		WaitHttp:           10,  //In second
		StorageHostServer:  strings.Split("127.0.0.1:5000,127.0.0.1:7001,127.0.0.1:7002", ","),
		StorageDB:          1,
		Channel:            "first",
		PubSubServer:       "localhost:6379",
	})
	if err != nil {
		log.Fatal(err)
		fmt.Println(err.Error())
	}
	httpReq.Consumer()
}
