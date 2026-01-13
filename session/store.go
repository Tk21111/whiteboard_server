package session

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
)

var (
	mu    sync.RWMutex
	store = make(map[string]string) // session -> userID
)

func Create(userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	token := base64.RawURLEncoding.EncodeToString(b)

	mu.Lock()
	store[token] = userID
	mu.Unlock()

	return token, nil
}

func Get(token string) (string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	userID, ok := store[token]
	return userID, ok
}

func Delete(token string) {
	mu.Lock()
	delete(store, token)
	mu.Unlock()
}
