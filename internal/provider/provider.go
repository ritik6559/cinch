package provider

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/llm/openai"
)

var builders = map[string]func(config.Config) (llm.Provider, error){
	"openai": func(cfg config.Config) (llm.Provider, error) {
		return openai.New(cfg.APIKey, cfg.Model, openai.WithBaseURL(cfg.BaseURL)), nil
	},

	"openai-compatible": func(cfg config.Config) (llm.Provider, error) {
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("provider %q needs CINCH_BASE_URL", "openai-compatible")
		}
		return openai.New(cfg.APIKey, cfg.Model, openai.WithBaseURL(cfg.BaseURL)), nil
	},
}

func New(cfg config.Config) (llm.Provider, error) {
	build, ok := builders[strings.ToLower(cfg.Provider)]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (known: %s)",
			cfg.Provider, strings.Join(Names(), ", "))
	}
	return build(cfg)
}

func Names() []string {
	names := make([]string, 0, len(builders))
	for name := range builders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
