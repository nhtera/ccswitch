// Package cryptobox provides AES-256-GCM authenticated encryption with
// Argon2id key derivation. Used by both internal/secrets (file fallback)
// and internal/export (encrypted bundles).
//
// Format produced by Seal is a self-describing byte string consisting of:
//
//	magic(4) || version(1) || time(1) || memMB(2 BE) || threads(1)
//	|| salt(16) || nonce(12) || ciphertext(N) || tag(16)
//
// Open parses the same format. Different magics distinguish payloads from
// different callers (secrets vault vs export bundle).
package cryptobox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Params controls the Argon2id work factor. Recorded inside each payload so
// older payloads can still be decrypted after defaults change.
type Params struct {
	Time    uint8  // iterations
	MemMB   uint16 // megabytes of memory
	Threads uint8
}

// DefaultParams are conservative for modern laptops. Time=3 meets the
// OWASP 2023 recommendation for Argon2id at 64 MiB memory.
var DefaultParams = Params{Time: 3, MemMB: 64, Threads: 4}

const (
	saltLen  = 16
	nonceLen = 12
	keyLen   = 32 // AES-256

	headerLen = 4 /*magic*/ + 1 /*version*/ + 1 /*time*/ + 2 /*memMB*/ + 1 /*threads*/ + saltLen + nonceLen
	version1  = 0x01
)

// ErrAuth indicates a wrong passphrase, corrupted ciphertext, or tampered
// payload. We return a single error so attackers can't distinguish causes.
var ErrAuth = errors.New("cryptobox: wrong passphrase or corrupted payload")

// ErrFormat indicates the payload is not a recognized cryptobox blob.
var ErrFormat = errors.New("cryptobox: bad format or unsupported version")

// Magic is a 4-byte tag that namespaces payloads to a specific caller.
type Magic [4]byte

// Seal encrypts plaintext with a key derived from passphrase + a random salt
// using Argon2id and AES-256-GCM. Returns the wire format described above.
func Seal(magic Magic, passphrase, plaintext []byte, p Params) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("read salt: %w", err)
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	key := deriveKey(passphrase, salt, p)
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	header := make([]byte, headerLen)
	copy(header[0:4], magic[:])
	header[4] = version1
	header[5] = p.Time
	binary.BigEndian.PutUint16(header[6:8], p.MemMB)
	header[8] = p.Threads
	copy(header[9:9+saltLen], salt)
	copy(header[9+saltLen:9+saltLen+nonceLen], nonce)

	// Bind the header into the auth tag — anyone who tampers with params
	// or magic will fail GCM verification.
	out := gcm.Seal(nil, nonce, plaintext, header)
	full := make([]byte, 0, len(header)+len(out))
	full = append(full, header...)
	full = append(full, out...)
	return full, nil
}

// Open verifies and decrypts a payload produced by Seal. The magic is
// checked against the supplied value; pass nil to skip the check.
func Open(magic *Magic, passphrase, payload []byte) ([]byte, error) {
	if len(payload) < headerLen+16 {
		return nil, ErrFormat
	}
	if magic != nil {
		var got Magic
		copy(got[:], payload[0:4])
		if got != *magic {
			return nil, ErrFormat
		}
	}
	if payload[4] != version1 {
		return nil, ErrFormat
	}
	p := Params{
		Time:    payload[5],
		MemMB:   binary.BigEndian.Uint16(payload[6:8]),
		Threads: payload[8],
	}
	if p.Time == 0 || p.MemMB == 0 || p.Threads == 0 {
		return nil, ErrFormat
	}

	salt := payload[9 : 9+saltLen]
	nonce := payload[9+saltLen : headerLen]
	header := payload[:headerLen]
	ciphertext := payload[headerLen:]

	key := deriveKey(passphrase, salt, p)
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ciphertext, header)
	if err != nil {
		return nil, ErrAuth
	}
	return pt, nil
}

func deriveKey(passphrase, salt []byte, p Params) []byte {
	return argon2.IDKey(passphrase, salt, uint32(p.Time), uint32(p.MemMB)*1024, p.Threads, keyLen)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return gcm, nil
}
