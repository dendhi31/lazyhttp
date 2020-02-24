package cache

import (
	"fmt"
	"time"

	"github.com/dendhi31/lazyhttp/redis"
)

// Cacher is a Cache handler that will handle all redis action related
type Cacher interface {
	SetPrefix(prefix string)
	Set(key string, value interface{}, ttl time.Duration) error
	Get(key string) (string, error)
	Remove(key string) error
}

// Client is a Cache Client, in this case we are using Redis
type Client struct {
	redisClient redis.Clienter
	prefix      string
}

// NewCacheClient will construct new client to be reused
func NewCacheClient(hosts []string, db int) (Cacher, error) {
	readTimeoutDuration := time.Duration(10) * time.Minute

	redisClient, err := redis.NewClient(hosts, db, readTimeoutDuration)

	if err != nil {
		return nil, err
	}

	return &Client{
		redisClient: redisClient,
	}, nil
}

// Set to store a key-value pair to Cache
func (c *Client) Set(key string, value interface{}, ttl time.Duration) error {
	key = c.addPrefix(key)

	return c.redisClient.Set(key, value, ttl)
}

// Get will retrieve a certain data by it's key
func (c *Client) Get(key string) (string, error) {
	key = c.addPrefix(key)

	val, err := c.redisClient.Get(key)
	if err != nil {
		return "", err
	}

	return val, nil
}

// Remove will delete a certain value by key
func (c *Client) Remove(key string) error {
	key = c.addPrefix(key)

	return c.redisClient.Remove(key)
}

// SetPrefix will append a prefix to this Cache Client
func (c *Client) SetPrefix(prefix string) {
	c.prefix = prefix
}

func (c *Client) addPrefix(key string) string {
	if c.prefix != "" {
		key = fmt.Sprintf("%s%s", c.prefix, key)
	}

	return key
}
