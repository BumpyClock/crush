package provider

import (
	"fmt"
	"net/http"

	"github.com/charmbracelet/crush/internal/auth"
	"github.com/charmbracelet/crush/internal/config"
)

type copilotSubTransport struct {
	base        http.RoundTripper
	authManager *auth.AuthManager
}

func newCopilotSubTransport(authManager *auth.AuthManager) http.RoundTripper {
	return &copilotSubTransport{
		base:        http.DefaultTransport,
		authManager: authManager,
	}
}

func (t *copilotSubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	if err := t.modifyRequestHeaders(cloned); err != nil {
		return nil, fmt.Errorf("failed to set headers: %w", err)
	}

	resp, err := t.base.RoundTrip(cloned)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Attempt to mint a new Copilot token and retry once.
		resp.Body.Close()
		if err := t.authManager.UpdateGithubCopilotAccess(req.Context()); err != nil {
			return nil, fmt.Errorf("copilot token refresh failed: %w", err)
		}
		cloned2 := req.Clone(req.Context())
		if err := t.modifyRequestHeaders(cloned2); err != nil {
			return nil, fmt.Errorf("failed to set headers after refresh: %w", err)
		}
		return t.base.RoundTrip(cloned2)
	}
	return resp, nil
}

func (t *copilotSubTransport) modifyRequestHeaders(req *http.Request) error {
	accessToken, err := t.authManager.GetValidGithubCopilotAccess(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get copilot token: %w", err)
	}

	// Ensure Authorization and GitHub-specific headers are set.
	req.Header.Del("x-api-key")
	req.Header.Del("X-Api-Key")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", auth.GitHubUserAgent)
	req.Header.Set("Editor-Version", auth.GitHubEditorVersion)
	req.Header.Set("Editor-Plugin-Version", auth.GitHubPluginVersion)
	return nil
}

func createCopilotSubHTTPClient() (*http.Client, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}
	am := auth.NewAuthManager(config.GlobalDataDir())
	if !am.HasGithubCopilotAuth() {
		return nil, fmt.Errorf("no GitHub Copilot auth found - run 'crush auth login'")
	}
	transport := newCopilotSubTransport(am)
	return &http.Client{Transport: transport}, nil
}
