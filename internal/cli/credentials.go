// Package cli provides the CLI implementation for hookly.
package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hooks.dx314.com/internal/crypto"
)

const (
	// ConfigDir is the directory name for hookly config files.
	ConfigDir = "hookly"
	// CredentialsFile is the name of the credentials file.
	CredentialsFile = "credentials.json"
)

// Credentials holds the stored authentication credentials.
type Credentials struct {
	EdgeURL   string    `json:"edge_url"`
	APIToken  string    `json:"api_token"` // Stored encrypted
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// CredentialsManager handles loading and saving credentials.
type CredentialsManager struct {
	configDir string
	key       []byte
}

// NewCredentialsManager creates a new credentials manager.
// The encryption key is derived from machine-specific data.
func NewCredentialsManager() (*CredentialsManager, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config dir: %w", err)
	}

	key, err := deriveKey()
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	return &CredentialsManager{
		configDir: configDir,
		key:       key,
	}, nil
}

// Load loads credentials from disk.
// Returns nil if no credentials exist.
func (m *CredentialsManager) Load() (*Credentials, error) {
	path := filepath.Join(m.configDir, CredentialsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var stored storedCredentials
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	// Decrypt the token
	token, err := m.decryptToken(stored.EncryptedToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	return &Credentials{
		EdgeURL:   stored.EdgeURL,
		APIToken:  token,
		UserID:    stored.UserID,
		Username:  stored.Username,
		CreatedAt: stored.CreatedAt,
	}, nil
}

// Save saves credentials to disk.
func (m *CredentialsManager) Save(creds *Credentials) error {
	// Ensure config directory exists
	if err := os.MkdirAll(m.configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Encrypt the token
	encryptedToken, err := m.encryptToken(creds.APIToken)
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}

	stored := storedCredentials{
		EdgeURL:        creds.EdgeURL,
		EncryptedToken: encryptedToken,
		UserID:         creds.UserID,
		Username:       creds.Username,
		CreatedAt:      creds.CreatedAt,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	path := filepath.Join(m.configDir, CredentialsFile)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

// Delete removes the credentials file.
func (m *CredentialsManager) Delete() error {
	path := filepath.Join(m.configDir, CredentialsFile)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}

// Path returns the path to the credentials file.
func (m *CredentialsManager) Path() string {
	return filepath.Join(m.configDir, CredentialsFile)
}

// storedCredentials is the on-disk format with encrypted token.
type storedCredentials struct {
	EdgeURL        string    `json:"edge_url"`
	EncryptedToken []byte    `json:"encrypted_token"`
	UserID         string    `json:"user_id"`
	Username       string    `json:"username"`
	CreatedAt      time.Time `json:"created_at"`
}

// encryptToken encrypts the API token for storage.
func (m *CredentialsManager) encryptToken(token string) ([]byte, error) {
	return crypto.Encrypt([]byte(token), m.key)
}

// decryptToken decrypts the API token from storage.
func (m *CredentialsManager) decryptToken(encrypted []byte) (string, error) {
	plaintext, err := crypto.Decrypt(encrypted, m.key)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// getConfigDir returns the path to the hookly config directory.
func getConfigDir() (string, error) {
	// Use XDG_CONFIG_HOME if set, otherwise ~/.config
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}

	return filepath.Join(configHome, ConfigDir), nil
}

// deriveKey derives an encryption key from machine-specific data.
// This provides basic protection against copying credentials to another machine.
func deriveKey() ([]byte, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME") // Windows
	}
	if username == "" {
		username = "unknown"
	}

	// Create a deterministic key from machine-specific data
	// This is NOT secure against a determined attacker, but provides
	// basic protection and ties credentials to the machine
	data := fmt.Sprintf("hookly:%s:%s:v1", hostname, username)
	hash := sha256.Sum256([]byte(data))
	return hash[:], nil
}

// ErrNotLoggedIn is returned when no credentials are found.
var ErrNotLoggedIn = errors.New("not logged in")
