package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

const (
	defaultProvider  = "openai"
	defaultModel     = "gpt-5.6"
	defaultCompactAt = 100_000
)

type Config struct {
	Provider  string
	APIKey    string
	Model     string
	BaseURL   string
	CompactAt int
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("loading .env: %w", err)
	}

	provider := os.Getenv("CINCH_PROVIDER")
	if provider == "" {
		provider = defaultProvider
	}

	model := firstEnv("CINCH_MODEL", "OPENAI_MODEL")
	if model == "" {
		model = defaultModel
	}

	compactAt := defaultCompactAt
	if v := os.Getenv("CINCH_COMPACT_AT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("CINCH_COMPACT_AT must be a positive number, got %q", v)
		}
		compactAt = n
	}

	return Config{
		Provider:  provider,
		APIKey:    firstEnv(KeyEnvFor(provider), "CINCH_API_KEY"),
		Model:     model,
		BaseURL:   os.Getenv("CINCH_BASE_URL"),
		CompactAt: compactAt,
	}, nil
}

func (c Config) Validate() error {
	if c.APIKey == "" {
		return errors.New(MissingKeyMessage(c.Provider))
	}
	return nil
}

func KeyEnvFor(provider string) string {
	switch provider {
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	}
	return "CINCH_API_KEY"
}

func MissingKeyMessage(provider string) string {
	name := KeyEnvFor(provider)
	if name == "CINCH_API_KEY" {
		return "no API key: set CINCH_API_KEY"
	}
	return fmt.Sprintf("no API key: set %s or CINCH_API_KEY", name)
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}
