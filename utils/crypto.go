package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-GCM with a key.
// The key must be 16, 24, or 32 bytes long (AES-128, AES-192, or AES-256).
func Encrypt(plaintext []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cannot create new cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cannot create GCM: %w", err)
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("cannot create nonce: %w", err)
	}

	// Seal encrypts and authenticates plaintext, authenticates the
	// additional data and appends the result to dst, returning the updated
	// slice. The nonce must be NonceSize() bytes long and unique for all
	// time, for a given key.
	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil) // No additional authenticated data

	// Prepend nonce to ciphertext for storage
	encryptedData := append(nonce, ciphertext...)

	return hex.EncodeToString(encryptedData), nil
}

// Decrypt decrypts hex-encoded ciphertext using AES-GCM with a key.
func Decrypt(encryptedHexString string, key []byte) ([]byte, error) {
	encryptedData, err := hex.DecodeString(encryptedHexString)
	if err != nil {
		return nil, fmt.Errorf("cannot hex decode string: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cannot create new cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cannot create GCM: %w", err)
	}

	nonceSize := aesgcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil) // No additional authenticated data
	if err != nil {
		// Do not return detailed error messages (e.g., "authentication failed")
		// to prevent potential padding oracle attacks.
		return nil, errors.New("failed to decrypt data")
	}

	return plaintext, nil
}

// GenerateSecureRandomString generates a cryptographically secure random string
// of the specified length, encoded in hex.
func GenerateSecureRandomString(length int) (string, error) {
	// Number of bytes needed is half the hex string length
	numBytes := length / 2
	if length%2 != 0 {
		numBytes++ // Ensure enough bytes for odd lengths
	}

	b := make([]byte, numBytes)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}

	// Encode bytes to hex string and return the required length
	hexString := hex.EncodeToString(b)
	return hexString[:length], nil
}
