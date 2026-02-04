package db

import (
	"hookly/internal/crypto"
)

// SecretManager handles encryption and decryption of secrets stored in the database.
type SecretManager struct {
	key []byte
}

// NewSecretManager creates a new SecretManager with the given encryption key.
func NewSecretManager(key []byte) *SecretManager {
	return &SecretManager{key: key}
}

// EncryptSecret encrypts a secret for storage.
func (sm *SecretManager) EncryptSecret(plaintext string) ([]byte, error) {
	return crypto.Encrypt([]byte(plaintext), sm.key)
}

// DecryptSecret decrypts a secret from storage.
func (sm *SecretManager) DecryptSecret(ciphertext []byte) (string, error) {
	plaintext, err := crypto.Decrypt(ciphertext, sm.key)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
