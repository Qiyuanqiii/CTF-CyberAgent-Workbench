package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

func New(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "id"
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405")
	}
	return prefix + "-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(random[:])
}
