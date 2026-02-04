// Package config handles application configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"hooks.dx314.com/internal/crypto"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	DatabasePath       string
	EncryptionKey      []byte
	Port               int
	BaseURL            string
	HomeHubSecret      string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubOrg          string
	GitHubAllowedUsers []string
	TelegramBotToken   string
	TelegramChatID     string
}

// Load loads configuration from environment variables.
// Optionally loads from .env file if present.
func Load() (*Config, error) {
	// Load .env file if present (ignore errors)
	_ = godotenv.Load()

	cfg := &Config{}

	// Required fields
	cfg.DatabasePath = getEnv("DATABASE_PATH", "./hookly.db")

	keyHex := os.Getenv("ENCRYPTION_KEY")
	if keyHex == "" {
		return nil, errors.New("ENCRYPTION_KEY is required")
	}
	key, err := crypto.ParseKey(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid ENCRYPTION_KEY: %w", err)
	}
	cfg.EncryptionKey = key

	cfg.Port = getEnvInt("PORT", 8080)
	cfg.BaseURL = getEnv("BASE_URL", "http://localhost:8080")
	cfg.HomeHubSecret = os.Getenv("HOME_HUB_SECRET")

	// GitHub OAuth (optional)
	cfg.GitHubClientID = os.Getenv("GITHUB_CLIENT_ID")
	cfg.GitHubClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	cfg.GitHubOrg = os.Getenv("GITHUB_ORG")
	if users := os.Getenv("GITHUB_ALLOWED_USERS"); users != "" {
		cfg.GitHubAllowedUsers = strings.Split(users, ",")
		for i, u := range cfg.GitHubAllowedUsers {
			cfg.GitHubAllowedUsers[i] = strings.TrimSpace(u)
		}
	}

	// Telegram notifications (optional)
	cfg.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	cfg.TelegramChatID = os.Getenv("TELEGRAM_CHAT_ID")

	return cfg, nil
}

// GitHubAuthEnabled returns true if GitHub OAuth is configured.
func (c *Config) GitHubAuthEnabled() bool {
	return c.GitHubClientID != "" && c.GitHubClientSecret != ""
}

// TelegramEnabled returns true if Telegram notifications are configured.
func (c *Config) TelegramEnabled() bool {
	return c.TelegramBotToken != "" && c.TelegramChatID != ""
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
