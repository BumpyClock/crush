package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/charmbracelet/catwalk/pkg/catwalk"
)

// models.dev schema (subset) used by opencode
type modelsDevProvider struct {
	ID     string                        `json:"id"`
	Name   string                        `json:"name"`
	Models map[string]modelsDevModelInfo `json:"models"`
}

type modelsDevModelInfo struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Limit  modelsDevLimits `json:"limit"`
	Reason bool            `json:"reasoning"`
	Opts   map[string]any  `json:"options"`
}

type modelsDevLimits struct {
	Context int64 `json:"context"`
	Output  int64 `json:"output"`
}

// fetchModelsDevProvider downloads the models.dev provider catalogue and extracts
// models for the given provider ID.
func fetchModelsDevProvider(providerID string) ([]catwalk.Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://models.dev/api.json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models.dev fetch failed: %d %s", resp.StatusCode, string(b))
	}

	var all map[string]modelsDevProvider
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, err
	}
	p, ok := all[providerID]
	if !ok || len(p.Models) == 0 {
		return nil, fmt.Errorf("provider %s not found in models.dev", providerID)
	}
	out := make([]catwalk.Model, 0, len(p.Models))
	for id, m := range p.Models {
		name := m.Name
		if name == "" {
			name = id
		}
		out = append(out, catwalk.Model{
			ID:               id,
			Name:             name,
			ContextWindow:    m.Limit.Context,
			DefaultMaxTokens: m.Limit.Output,
			CanReason:        m.Reason,
			SupportsImages:   true,
		})
	}
	return out, nil
}
