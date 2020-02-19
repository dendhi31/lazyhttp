package redis

import (
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
)

// Clienter is an interface implementation for redis Client()
type Clienter interface {
	Get(key string) (string, error)
	Set(key string, value interface{}, ttl time.Duration) error
	Remove(key string) error
}

// Client is struct representative for redis Client
type Client struct {
	clientMu sync.Mutex
	client   redis.Cmdable

	hosts       []string
	dbName      int
	readTimeout time.Duration
}

// Client is used to get existing redis client
// if client is nil. then create new client
func (c *Client) Client() (redis.Cmdable, error) {
	if c.client == nil {
		c.clientMu.Lock()
		defer c.clientMu.Unlock()

		var err error
		c.client, err = createClient(c.hosts, c.dbName, c.readTimeout)
		if err != nil {
			return nil, err
		}
	}

	return c.client, nil
}

// NewClient is an initialize redis client function
// this will create new client and store into a pointer
func NewClient(hosts []string, dbName int, readTimeout time.Duration) (*Client, error) {
	client, err := createClient(hosts, dbName, readTimeout)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:      client,
		hosts:       hosts,
		dbName:      dbName,
		readTimeout: readTimeout,
	}, nil
}

// createClient is function to create new redis client
// if hosts is 1 , then create client for single instance redis
// if hosts is more then 1 , create new redis cluster client
// if hosts is 0, then return error
func createClient(hosts []string, dbName int, readTimeout time.Duration) (redis.Cmdable, error) {
	var client redis.Cmdable

	if len(hosts) == 1 {
		h := hosts[0]
		client = redis.NewClient(&redis.Options{
			Addr:        h,
			DB:          dbName,
			ReadTimeout: readTimeout,
		})
	} else if len(hosts) > 1 {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:       hosts,
			ReadTimeout: readTimeout,
		})
	} else {
		return nil, errors.New("no host(s) found")
	}

	_, err := client.Ping().Result()
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (c *Client) checkConnection() error {
	if c.client == nil {
		c.clientMu.Lock()
		newClient, err := createClient(c.hosts, c.dbName, c.readTimeout)
		if err != nil {
			return err
		}
		c.client = newClient
		c.clientMu.Unlock()
	} else {
		if _, err := c.client.Ping().Result(); err != nil {
			c.clientMu.Lock()
			newClient, err := createClient(c.hosts, c.dbName, c.readTimeout)
			if err != nil {
				return err
			}
			c.client = newClient
			c.clientMu.Unlock()
		}
	}

	return nil
}
