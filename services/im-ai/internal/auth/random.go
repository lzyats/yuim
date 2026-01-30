package auth

import (
	"crypto/rand"
	"io"
)

// RandomAlphaNum returns a random alphanumeric string of length n.
func RandomAlphaNum(n int) (string, error) {
	const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	for i := 0; i < n; i++ {
		buf[i] = letters[int(buf[i])%len(letters)]
	}
	return string(buf), nil
}
