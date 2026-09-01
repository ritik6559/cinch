package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/sandbox"
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
	Effort    string
	Sandbox   sandbox.Mode
}

func Load() (Config, error) {
	root, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}
	return LoadFrom(root)
}

// LoadFrom builds the configuration in layers, each beating the one before:
//
//	built-in defaults
//	~/.cinch/config.yaml        your own preferences
//	<root>/.cinch/config.yaml   what this repository asks for
//	.env
//	the environment
//
// The environment comes last so a single run can override everything without
// anyone editing a file.
func LoadFrom(root string) (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("loading .env: %w", err)
	}

	cfg := Config{
		Provider:  defaultProvider,
		Model:     defaultModel,
		CompactAt: defaultCompactAt,
		Sandbox:   sandbox.ModePolicy,
	}

	for _, src := range Sources(root) {
		f, err := ReadFile(src.Path)
		if err != nil {
			return Config{}, err
		}
		if err := f.Apply(&cfg, src.Trust, src.Path); err != nil {
			return Config{}, err
		}
	}

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}

	// Resolved last: which key variable to read depends on the final provider.
	cfg.APIKey = firstEnv(KeyEnvFor(cfg.Provider), "CINCH_API_KEY")
	return cfg, nil
}

func applyEnv(c *Config) error {
	if v := os.Getenv("CINCH_PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := firstEnv("CINCH_MODEL", "OPENAI_MODEL"); v != "" {
		c.Model = v
	}
	if v := os.Getenv("CINCH_BASE_URL"); v != "" {
		c.BaseURL = v
	}

	if v := os.Getenv("CINCH_COMPACT_AT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("CINCH_COMPACT_AT must be a positive number, got %q", v)
		}
		c.CompactAt = n
	}

	if v := os.Getenv("CINCH_EFFORT"); v != "" {
		if !llm.ValidEffort(v) {
			return fmt.Errorf("CINCH_EFFORT must be one of %s, got %q",
				strings.Join(llm.Efforts, ", "), v)
		}
		c.Effort = v
	}

	if v := os.Getenv("CINCH_SANDBOX"); v != "" {
		if !sandbox.ValidMode(v) {
			return fmt.Errorf("CINCH_SANDBOX must be one of %s, got %q",
				strings.Join(sandbox.Modes, ", "), v)
		}
		c.Sandbox = sandbox.Mode(v)
	}

	return nil
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
