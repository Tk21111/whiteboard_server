package middleware

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func ColorFromUserID(userID string) string {
	h := fnv.New32a()
	h.Write([]byte(userID))
	hash := h.Sum32()

	hue := int(hash % 360)
	return fmt.Sprintf("hsl(%d, 70%%, 55%%)", hue)
}
