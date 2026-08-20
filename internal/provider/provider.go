// Package provider chooses which model provider a session uses.
//
// It exists so nothing else in cinch has to know which providers exist. The
// import of a concrete provider cannot be removed — Go needs a real type
// somewhere — but it can be confined to one file whose only job is choosing.
package provider

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/llm/openai"
)

// builders maps a provider name to the code that constructs it.
//
// A map rather than a switch, so Names() can list them for an error message and
// for `cinch doctor` without the list being written out twice.
var builders = map[string]func(config.Config) (llm.Provider, error){
	"openai": func(cfg config.Config) (llm.Provider, error) {
		return openai.New(cfg.APIKey, cfg.Model, openai.WithBaseURL(cfg.BaseURL)), nil
	},

	// Any service that implements the OpenAI Responses API at another address.
	// No adapter is needed: only the address changes.
	//
	// Note this means the Responses API specifically, not the older
	// chat-completions format. A service advertising "OpenAI compatible" may
	// only implement the latter, in which case it needs a real adapter.
	"openai-compatible": func(cfg config.Config) (llm.Provider, error) {
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("provider %q needs CINCH_BASE_URL", "openai-compatible")
		}
		return openai.New(cfg.APIKey, cfg.Model, openai.WithBaseURL(cfg.BaseURL)), nil
	},
}

// New builds the provider named in the configuration.
func New(cfg config.Config) (llm.Provider, error) {
	build, ok := builders[strings.ToLower(cfg.Provider)]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (known: %s)",
			cfg.Provider, strings.Join(Names(), ", "))
	}
	return build(cfg)
}

// Names lists the providers cinch can build, in a stable order.
func Names() []string {
	names := make([]string, 0, len(builders))
	for name := range builders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
