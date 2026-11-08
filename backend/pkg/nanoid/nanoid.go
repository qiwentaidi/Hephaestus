package nanoid

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// Generate returns a compact unique identifier suitable for persisted IDs.
func Generate() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	timestamp := time.Now().UnixNano()
	return base64.URLEncoding.EncodeToString(b) + fmt.Sprintf("%d", timestamp)[:10]
}
