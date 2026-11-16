package storage

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var client *redis.Client

// SessionExists is the statically visible Redis adapter boundary.
func SessionExists(token string) bool {
	return client.Get(context.Background(), token).Err() == nil
}
