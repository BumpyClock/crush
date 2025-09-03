package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/auth"
	"strings"
)

// OAuthProvider represents a provider that uses OAuth authentication
type OAuthProvider struct {
	ID          string
	Name        string
	Type        catwalk.Type
	Models      []catwalk.Model
	BaseURL     string
	AuthManager *auth.AuthManager
}

// OAuthProviderRegistry manages OAuth providers
type OAuthProviderRegistry struct {
	providers map[string]OAuthProvider
}

var oauthRegistry = &OAuthProviderRegistry{
	providers: make(map[string]OAuthProvider),
}

// RegisterOAuthProvider registers a new OAuth provider
func RegisterOAuthProvider(provider OAuthProvider) {
	oauthRegistry.providers[provider.ID] = provider
}

// GetOAuthProviders returns all registered OAuth providers
func GetOAuthProviders(dataDirectory string) []OAuthProvider {
	authManager := auth.NewAuthManager(dataDirectory)

	var providers []OAuthProvider
	for _, provider := range oauthRegistry.providers {
		// Create a copy with the auth manager
		providerCopy := provider
		providerCopy.AuthManager = authManager
		providers = append(providers, providerCopy)
	}

	return providers
}

// GetOAuthProvider returns a specific OAuth provider by ID
func GetOAuthProvider(id string, dataDirectory string) (*OAuthProvider, bool) {
	provider, exists := oauthRegistry.providers[id]
	if !exists {
		return nil, false
	}

	// Create a copy with the auth manager
	providerCopy := provider
	providerCopy.AuthManager = auth.NewAuthManager(dataDirectory)

	return &providerCopy, true
}

// HasOAuthCredentials checks if an OAuth provider has valid credentials
func (p *OAuthProvider) HasOAuthCredentials() bool {
	if p.AuthManager == nil {
		return false
	}

	switch p.ID {
	case "claudesub":
		return p.AuthManager.HasClaudeSubAuth()
	case "github-copilot":
		return p.AuthManager.HasGithubCopilotAuth()
	default:
		return false
	}
}

// ToDisplayProvider converts an OAuth provider to a catwalk.Provider for TUI display
func (p *OAuthProvider) ToDisplayProvider() catwalk.Provider {
	name := p.Name
	if !p.HasOAuthCredentials() {
		name += " (Auth Required)"
	}
	// Determine model list for GitHub Copilot.
	models := p.Models
	if p.ID == "github-copilot" {
		// Prefer models.dev for freshest catalogue; fall back to Copilot /v1/models when authenticated.
		if fetched, err := fetchModelsDevProvider("github-copilot"); err == nil && len(fetched) > 0 {
			models = fetched
		} else if p.AuthManager != nil && p.AuthManager.HasGithubCopilotAuth() {
			if fetched2, err2 := fetchCopilotModels(p.AuthManager); err2 == nil && len(fetched2) > 0 {
				models = fetched2
			}
		}
	}

	// Convert models to OAuth models with zero cost
	oauthModels := make([]catwalk.Model, len(models))
	for i, model := range models {
		oauthModels[i] = catwalk.Model{
			ID:                     model.ID,
			Name:                   model.Name,
			CostPer1MIn:            0, // OAuth providers are subscription-based
			CostPer1MOut:           0,
			CostPer1MInCached:      0,
			CostPer1MOutCached:     0,
			ContextWindow:          model.ContextWindow,
			DefaultMaxTokens:       model.DefaultMaxTokens,
			CanReason:              model.CanReason,
			HasReasoningEffort:     model.HasReasoningEffort,
			DefaultReasoningEffort: model.DefaultReasoningEffort,
			SupportsImages:         model.SupportsImages,
		}
	}

	// Pick reasonable defaults for large/small from the available list.
	var defLarge, defSmall string
	if len(oauthModels) > 0 {
		defLarge = oauthModels[0].ID
		defSmall = oauthModels[0].ID
		if len(oauthModels) > 1 {
			defSmall = oauthModels[len(oauthModels)-1].ID
		}
	}
	// Override with provider-specific defaults when sensible.
	if p.ID == "github-copilot" {
		for _, m := range oauthModels {
			if m.ID == "gpt-4o" {
				defLarge = m.ID
			}
			if m.ID == "gpt-4o-mini" {
				defSmall = m.ID
			}
		}
	}
	if p.ID == "claudesub" {
		for _, m := range oauthModels {
			if m.ID != "" && defLarge == "" {
				defLarge = m.ID
			}
			if strings.Contains(strings.ToLower(m.ID), "haiku") {
				defSmall = m.ID
			}
		}
		if defSmall == "" && len(oauthModels) > 0 {
			defSmall = oauthModels[len(oauthModels)-1].ID
		}
	}

	return catwalk.Provider{
		ID:                  catwalk.InferenceProvider(p.ID),
		Name:                name,
		Type:                p.Type,
		Models:              oauthModels,
		DefaultLargeModelID: defLarge,
		DefaultSmallModelID: defSmall,
	}
}

// RegisterBuiltinOAuthProviders registers the built-in OAuth providers
func RegisterBuiltinOAuthProviders() {
	RegisterOAuthProvider(OAuthProvider{ID: "claudesub", Name: "Claude Max/Pro Subscription", Type: catwalk.TypeAnthropic, BaseURL: "https://api.anthropic.com/v1", Models: DefaultModels("claudesub")})
	RegisterOAuthProvider(OAuthProvider{ID: "github-copilot", Name: "GitHub Copilot Subscription", Type: catwalk.TypeOpenAI, BaseURL: "https://api.githubcopilot.com", Models: DefaultModels("github-copilot")})
}

// Initialize OAuth providers
func init() {
	RegisterBuiltinOAuthProviders()
}

// fetchCopilotModels retrieves the actual model list allowed by the Copilot subscription
// using the OpenAI-compatible /v1/models endpoint on api.githubcopilot.com.
func fetchCopilotModels(am *auth.AuthManager) ([]catwalk.Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := am.GetValidGithubCopilotAccess(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.githubcopilot.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", auth.GitHubUserAgent)
	req.Header.Set("Editor-Version", auth.GitHubEditorVersion)
	req.Header.Set("Editor-Plugin-Version", auth.GitHubPluginVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("copilot /models failed: %d %s", resp.StatusCode, string(b))
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]catwalk.Model, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID == "" {
			continue
		}
		out = append(out, catwalk.Model{
			ID:               m.ID,
			Name:             m.ID,
			ContextWindow:    128000,
			DefaultMaxTokens: 8192,
			SupportsImages:   true,
		})
	}
	return out, nil
}
