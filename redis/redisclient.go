package redis

import (
	"time"

	"github.com/go-redis/redis"
)

// Get will return a certain value based on Key
func (c *Client) Get(key string) (string, error) {
	err := c.checkConnection()
	if err != nil {
		return "", err
	}

	result, err := c.client.Get(key).Result()

	if err == redis.Nil {
		err = nil
	}

	return result, err
}

// Set will store a key-value pair to Cache
func (c *Client) Set(key string, value interface{}, ttl time.Duration) error {
	err := c.checkConnection()
	if err != nil {
		return err
	}

	return c.client.Set(key, value, ttl).Err()
}

// Remove will remove a value from Cache based on desired key
func (c *Client) Remove(key string) error {
	err := c.checkConnection()
	if err != nil {
		return err
	}

	return c.client.Del(key).Err()
}

func (c *Client) Publish(channel string, value interface{}) error {
	err := c.checkConnection()
	if err != nil {
		return err
	}

	return c.client.Publish(channel, value).Err()
}

func (c *Client) Subscribe(channels ...string) *redis.PubSub {
	err := c.checkConnection()
	if err != nil {
		return nil
	}
	return &c.pubsub
}
