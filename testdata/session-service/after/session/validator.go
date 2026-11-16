package session

import "example.test/session-service/storage"

func Validate(token string) bool {
	return storage.SessionExists(token)
}
