package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	AuthFileName = "auth.json"
	AuthFileMode = 0o600 // Read/write for owner only
)

type OAuthCredentials struct {
	Type    string `json:"type"`
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	Expires int64  `json:"expires"`         // Unix timestamp in milliseconds
	Scope   string `json:"scope,omitempty"` // Granted OAuth scopes
}

// AuthData represents the complete authentication data structure
type AuthData struct {
	ClaudeSub     *OAuthCredentials `json:"claudesub,omitempty"`
	GithubCopilot *OAuthCredentials `json:"github-copilot,omitempty"`
}

// AuthManager handles storage and retrieval of authentication data
type AuthManager struct {
	dataDir string
	mu      sync.RWMutex
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(dataDir string) *AuthManager {
	return &AuthManager{
		dataDir: dataDir,
	}
}

// authFilePath returns the full path to the auth.json file
func (am *AuthManager) authFilePath() string {
	return filepath.Join(am.dataDir, AuthFileName)
}

// LoadAuthData loads authentication data from disk
func (am *AuthManager) LoadAuthData() (*AuthData, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	authPath := am.authFilePath()
	data, err := os.ReadFile(authPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty auth data if file doesn't exist
			return &AuthData{}, nil
		}
		return nil, fmt.Errorf("failed to read auth file: %w", err)
	}

	var authData AuthData
	if err := json.Unmarshal(data, &authData); err != nil {
		return nil, fmt.Errorf("failed to parse auth data: %w", err)
	}

	return &authData, nil
}

// SaveAuthData saves authentication data to disk with secure permissions
func (am *AuthManager) SaveAuthData(authData *AuthData) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Ensure data directory exists
	if err := os.MkdirAll(am.dataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Serialize auth data
	data, err := json.MarshalIndent(authData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal auth data: %w", err)
	}

	authPath := am.authFilePath()

	// Write with secure permissions (owner only)
	if err := os.WriteFile(authPath, data, AuthFileMode); err != nil {
		return fmt.Errorf("failed to write auth file: %w", err)
	}

	return nil
}

func (am *AuthManager) StoreClaudeSubCredentials(tokenResp *TokenResponse) error {
	authData, err := am.LoadAuthData()
	if err != nil {
		return fmt.Errorf("failed to load auth data: %w", err)
	}

	// Calculate expiration timestamp
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).UnixMilli()

	authData.ClaudeSub = &OAuthCredentials{
		Type:    "oauth",
		Access:  tokenResp.AccessToken,
		Refresh: tokenResp.RefreshToken,
		Expires: expiresAt,
		Scope:   tokenResp.Scope,
	}

	if err := am.SaveAuthData(authData); err != nil {
		return fmt.Errorf("failed to save claudesub credentials: %w", err)
	}

	slog.Info("Successfully stored ClaudeSub OAuth credentials")
	return nil
}

func (am *AuthManager) GetClaudeSubCredentials() (*OAuthCredentials, error) {
	authData, err := am.LoadAuthData()
	if err != nil {
		return nil, fmt.Errorf("failed to load auth data: %w", err)
	}

	if authData.ClaudeSub == nil {
		return nil, fmt.Errorf("no claudesub credentials found")
	}

	return authData.ClaudeSub, nil
}

func (am *AuthManager) IsClaudeSubTokenValid() (bool, error) {
	creds, err := am.GetClaudeSubCredentials()
	if err != nil {
		return false, err
	}

	now := time.Now().UnixMilli()
	// Consider token expired if less than 5 minutes remaining
	bufferMs := int64(5 * 60 * 1000) // 5 minutes in milliseconds

	return now < (creds.Expires - bufferMs), nil
}

func (am *AuthManager) RefreshClaudeSubToken(ctx context.Context) error {
	creds, err := am.GetClaudeSubCredentials()
	if err != nil {
		return fmt.Errorf("no claudesub credentials to refresh: %w", err)
	}

	if creds.Refresh == "" {
		return fmt.Errorf("no refresh token available")
	}

	// Use OAuth flow to refresh the token
	oauthFlow := NewOAuthFlow()
	tokenResp, err := oauthFlow.RefreshToken(ctx, creds.Refresh)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	// Store the new credentials
	if err := am.StoreClaudeSubCredentials(tokenResp); err != nil {
		return fmt.Errorf("failed to store refreshed credentials: %w", err)
	}

	slog.Info("Successfully refreshed ClaudeSub OAuth token")
	return nil
}

func (am *AuthManager) ClearClaudeSubCredentials() error {
	authData, err := am.LoadAuthData()
	if err != nil {
		return fmt.Errorf("failed to load auth data: %w", err)
	}

	authData.ClaudeSub = nil

	if err := am.SaveAuthData(authData); err != nil {
		return fmt.Errorf("failed to save auth data after clearing credentials: %w", err)
	}

	slog.Info("Cleared ClaudeSub OAuth credentials")
	return nil
}

func (am *AuthManager) GetValidAccessToken(ctx context.Context) (string, error) {
	valid, err := am.IsClaudeSubTokenValid()
	if err != nil {
		return "", err
	}

	// Refresh token if expired
	if !valid {
		if err := am.RefreshClaudeSubToken(ctx); err != nil {
			return "", fmt.Errorf("failed to refresh expired token: %w", err)
		}
	}

	// Get the credentials (now guaranteed to be valid)
	creds, err := am.GetClaudeSubCredentials()
	if err != nil {
		return "", err
	}

	return creds.Access, nil
}

func (am *AuthManager) HasClaudeSubAuth() bool {
	_, err := am.GetClaudeSubCredentials()
	return err == nil
}

// GitHub Copilot credential helpers.

// StoreGithubCopilotRefresh stores the GitHub OAuth token (used to mint Copilot API tokens).
func (am *AuthManager) StoreGithubCopilotRefresh(githubOAuthToken string) error {
	authData, err := am.LoadAuthData()
	if err != nil {
		return fmt.Errorf("failed to load auth data: %w", err)
	}

	if authData.GithubCopilot == nil {
		authData.GithubCopilot = &OAuthCredentials{}
	}

	authData.GithubCopilot.Type = "oauth"
	authData.GithubCopilot.Refresh = githubOAuthToken
	// Access and Expires will be set after minting a Copilot token.

	if err := am.SaveAuthData(authData); err != nil {
		return fmt.Errorf("failed to save github-copilot credentials: %w", err)
	}

	slog.Info("Stored GitHub OAuth token for Copilot")
	return nil
}

// UpdateGithubCopilotAccess exchanges the GitHub OAuth token for a Copilot API token and saves it.
func (am *AuthManager) UpdateGithubCopilotAccess(ctx context.Context) error {
	creds, err := am.GetGithubCopilotCredentials()
	if err != nil {
		return fmt.Errorf("no github-copilot credentials to update: %w", err)
	}
	if creds.Refresh == "" {
		return fmt.Errorf("no github oauth token available")
	}

	token, expiresAt, err := FetchCopilotToken(ctx, creds.Refresh)
	if err != nil {
		return fmt.Errorf("failed to fetch copilot token: %w", err)
	}

	authData, err := am.LoadAuthData()
	if err != nil {
		return fmt.Errorf("failed to load auth data: %w", err)
	}
	if authData.GithubCopilot == nil {
		authData.GithubCopilot = &OAuthCredentials{}
	}

	authData.GithubCopilot.Type = "oauth"
	authData.GithubCopilot.Access = token
	authData.GithubCopilot.Expires = expiresAt
	// Keep Refresh as GitHub OAuth token.

	if err := am.SaveAuthData(authData); err != nil {
		return fmt.Errorf("failed to save github-copilot access token: %w", err)
	}

	slog.Info("Stored GitHub Copilot access token")
	return nil
}

// GetGithubCopilotCredentials returns stored GitHub Copilot credentials.
func (am *AuthManager) GetGithubCopilotCredentials() (*OAuthCredentials, error) {
	authData, err := am.LoadAuthData()
	if err != nil {
		return nil, fmt.Errorf("failed to load auth data: %w", err)
	}
	if authData.GithubCopilot == nil {
		return nil, fmt.Errorf("no github-copilot credentials found")
	}
	return authData.GithubCopilot, nil
}

// IsGithubCopilotTokenValid reports whether the stored Copilot access token is still valid (5m buffer).
func (am *AuthManager) IsGithubCopilotTokenValid() (bool, error) {
	creds, err := am.GetGithubCopilotCredentials()
	if err != nil {
		return false, err
	}
	if creds.Access == "" || creds.Expires == 0 {
		return false, nil
	}
	now := time.Now().UnixMilli()
	bufferMs := int64(5 * 60 * 1000)
	return now < (creds.Expires - bufferMs), nil
}

// GetValidGithubCopilotAccess ensures a valid Copilot access token, updating if needed.
func (am *AuthManager) GetValidGithubCopilotAccess(ctx context.Context) (string, error) {
	valid, err := am.IsGithubCopilotTokenValid()
	if err != nil {
		return "", err
	}
	if !valid {
		if err := am.UpdateGithubCopilotAccess(ctx); err != nil {
			return "", fmt.Errorf("failed to refresh copilot token: %w", err)
		}
	}
	creds, err := am.GetGithubCopilotCredentials()
	if err != nil {
		return "", err
	}
	return creds.Access, nil
}

// ClearGithubCopilotCredentials clears stored GitHub Copilot credentials.
func (am *AuthManager) ClearGithubCopilotCredentials() error {
	authData, err := am.LoadAuthData()
	if err != nil {
		return fmt.Errorf("failed to load auth data: %w", err)
	}
	authData.GithubCopilot = nil
	if err := am.SaveAuthData(authData); err != nil {
		return fmt.Errorf("failed to save auth data after clearing credentials: %w", err)
	}
	slog.Info("Cleared GitHub Copilot credentials")
	return nil
}

// HasGithubCopilotAuth returns true if a GitHub OAuth token is stored.
func (am *AuthManager) HasGithubCopilotAuth() bool {
	_, err := am.GetGithubCopilotCredentials()
	return err == nil
}
