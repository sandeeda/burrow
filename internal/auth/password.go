// Package auth handles Burrow's password, login sessions, CSRF tokens and
// login rate limiting.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// MinPasswordLen is the shortest password accepted, in characters.
const MinPasswordLen = 8

// MaxPasswordLen caps hashing cost for absurd inputs.
const MaxPasswordLen = 1024

// argon2id parameters (RFC 9106 / OWASP guidance, tuned for a home PC:
// ~50–100 ms per hash).
type params struct {
	memory  uint32 // KiB
	time    uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

var defaultParams = params{memory: 64 * 1024, time: 3, threads: 2, keyLen: 32, saltLen: 16}

var (
	// ErrWeakPassword means the password is too short (or too long).
	ErrWeakPassword = fmt.Errorf("password must be %d–%d characters", MinPasswordLen, MaxPasswordLen)
	errBadHash      = errors.New("auth: malformed password hash")
)

// CheckPasswordStrength enforces the length rules.
func CheckPasswordStrength(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen || len(pw) > MaxPasswordLen {
		return ErrWeakPassword
	}
	return nil
}

// HashPassword returns an encoded argon2id hash in the standard PHC format:
// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
func HashPassword(pw string) (string, error) {
	if err := CheckPasswordStrength(pw); err != nil {
		return "", err
	}
	p := defaultParams
	salt := make([]byte, p.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, p.time, p.memory, p.threads, p.keyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches the encoded hash, in constant
// time with respect to the hash contents.
func VerifyPassword(pw, encoded string) (bool, error) {
	if len(pw) > MaxPasswordLen {
		return false, nil
	}
	p, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(pw), salt, p.time, p.memory, p.threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func decodeHash(encoded string) (params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return params{}, nil, nil, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return params{}, nil, nil, errBadHash
	}
	var p params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return params{}, nil, nil, errBadHash
	}
	// Refuse parameters that would make verification absurdly expensive.
	if p.memory < 8*1024 || p.memory > 1024*1024 || p.time < 1 || p.time > 20 || p.threads < 1 {
		return params{}, nil, nil, errBadHash
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return params{}, nil, nil, errBadHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) < 16 {
		return params{}, nil, nil, errBadHash
	}
	return p, salt, key, nil
}
