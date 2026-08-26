package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
)

var fallbackModels = []string{"gpt-5.6", "gpt-5.6-mini", "gpt-5.5", "gpt-5.5-mini"}

func (c *Client) Models(ctx context.Context) ([]string, error) {
	url := strings.TrimSuffix(c.baseURL, "/responses") + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fallbackModels, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apikey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fallbackModels, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallbackModels, nil
	}

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fallbackModels, err
	}

	var out []string
	for _, m := range body.Data {
		if usableModel(m.ID) {
			out = append(out, m.ID)
		}
	}
	if len(out) == 0 {
		return fallbackModels, nil
	}

	slices.Sort(out)
	return out, nil
}

func usableModel(id string) bool {
	for _, bad := range []string{
		"embedding", "whisper", "tts", "dall-e", "moderation",
		"transcribe", "realtime", "image", "audio", "search", "codex",
	} {
		if strings.Contains(id, bad) {
			return false
		}
	}

	if parts := strings.Split(id, "-"); len(parts) >= 4 {
		tail := parts[len(parts)-3:]
		if len(tail[0]) == 4 && len(tail[1]) == 2 && len(tail[2]) == 2 {
			return false
		}
	}

	return strings.HasPrefix(id, "gpt") || strings.HasPrefix(id, "o") ||
		strings.HasPrefix(id, "claude") || strings.HasPrefix(id, "gemini")
}
