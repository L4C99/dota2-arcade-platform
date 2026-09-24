package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	TokenBytes    = 32
	saltBytes     = 16
	argonTime     = 3
	argonMemory   = 64 * 1024
	argonThreads  = 4
	argonKeyBytes = 32
)

func NewToken() (string, [32]byte, error) {
	var raw [TokenBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, sha256.Sum256([]byte(token)), nil
}

func TokenHash(token string) ([32]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != TokenBytes || base64.RawURLEncoding.EncodeToString(raw) != token {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(token)), true
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", errors.New("admin password must contain 12 to 1024 bytes")
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyBytes)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, timeCost, threads uint64
	params := strings.Split(parts[3], ",")
	if len(params) != 3 || !strings.HasPrefix(params[0], "m=") || !strings.HasPrefix(params[1], "t=") || !strings.HasPrefix(params[2], "p=") {
		return false
	}
	var err error
	if memory, err = strconv.ParseUint(strings.TrimPrefix(params[0], "m="), 10, 32); err != nil {
		return false
	}
	if timeCost, err = strconv.ParseUint(strings.TrimPrefix(params[1], "t="), 10, 32); err != nil {
		return false
	}
	if threads, err = strconv.ParseUint(strings.TrimPrefix(params[2], "p="), 10, 8); err != nil {
		return false
	}
	// Only our bounded format is accepted, avoiding hostile hashes with excessive cost.
	if memory != argonMemory || timeCost != argonTime || threads != argonThreads {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltBytes {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != argonKeyBytes {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(timeCost), uint32(memory), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
