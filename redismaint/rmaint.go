package redismaint

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gomodule/redigo/redis"
)

type (
	SendRequestWithPubSub func(ctx context.Context, url string, action string, payload []byte, header map[string]string, key string) (int, []byte, error)
)

//EventMessage as a message
type EventMessage struct {
	ID string `json:"id"`
}

type RequestRequirement struct {
	Url     string            `json:"url"`
	Action  string            `json:"action"`
	Payload []byte            `json:"payload"`
	Header  map[string]string `json:"header"`
	Key     string            `json:"key"`
}

//Consumer structure
type Consumer struct {
	rclt    *redisc
	hkey    string
	echan   chan error
	schan   chan bool
	debug   bool
	handler SendRequestWithPubSub

	sleepDuration time.Duration
}

//Configuration as consumer preferences
type Configuration struct {
	RedisURL      string
	ContexName    string
	Debug         bool
	SleepDuration time.Duration
	Handler       SendRequestWithPubSub
}

//New creates new redis maintenance
func New(config Configuration) (*Consumer, error) {
	rclt, err := dial(config.RedisURL)
	if err != nil {
		return nil, err
	}
	return &Consumer{
		rclt:          rclt,
		hkey:          config.ContexName,
		echan:         make(chan error, 1),
		schan:         make(chan bool, 1),
		debug:         config.Debug,
		sleepDuration: config.SleepDuration,
		handler:       config.Handler,
	}, nil
}

//Run runs the consumer
func (m *Consumer) Run() {
	rc := m.rclt.gconn()
	psc := redis.PubSubConn{
		Conn: rc,
	}
	key := fmt.Sprintf("%s", m.hkey)
	if err := psc.PSubscribe(key); err != nil {
		m.echan <- err
		return
	}
	for {
		select {
		case <-m.schan:
			err := rc.Close()
			if err != nil {
				m.debugln("err", err)
			}
			err = psc.Close()
			if err != nil {
				m.debugln("err", err)
			}
			m.echan <- err
			return
		default:
			switch msg := psc.Receive().(type) {
			case redis.Message:
				m.process(msg.Data)
				time.Sleep(1 * time.Second)
			}
		}
	}
}

//Err returns error channel
func (m *Consumer) Err() <-chan error {
	return m.echan
}

//Stop set stop flag
func (m *Consumer) Stop() {
	m.schan <- true
}

func (m *Consumer) process(bytes []byte) {
	var req RequestRequirement
	fmt.Println(string(bytes))
	err := json.Unmarshal(bytes, &req)
	if err != nil {
		log.Println("err", err)
		return
	}
	_, _, err = m.handler(context.Background(), req.Url, req.Action, req.Payload, req.Header, req.Key)
	if err != nil {
		log.Println("err", err)
		return
	}
	return
}

func (m *Consumer) debugln(args ...interface{}) {
	if m.debug {
		log.Println(args...)
	}
}
