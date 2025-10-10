package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHub Copilot OAuth and token minting helpers.

const (
	// Copilot GitHub OAuth app client ID used by official Copilot Chat.
	githubClientID  = "Iv1.b507a08c87ecfe98"
	deviceCodeURL   = "https://github.com/login/device/code"
	accessTokenURL  = "https://github.com/login/oauth/access_token"
	copilotTokenURL = "https://api.github.com/copilot_internal/v2/token"
	ghUserAgent     = "GitHubCopilotChat/0.26.7"
	ghEditorVersion = "vscode/1.104.1"
	ghPluginVersion = "copilot-chat/0.26.7"
)

// Exported header constants for reuse by transports.
const (
	GitHubUserAgent     = ghUserAgent
	GitHubEditorVersion = ghEditorVersion
	GitHubPluginVersion = ghPluginVersion
)

type ghDeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type ghAccessTokenResponse struct {
	AccessToken      string `json:"access_token,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type ghCopilotTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	// RefreshIn is present but we currently do on-demand fetch via OAuth token.
	RefreshIn int64 `json:"refresh_in"`
}

// StartGithubDeviceAuth starts the GitHub device authorization flow.
func StartGithubDeviceAuth(ctx context.Context) (*ghDeviceCodeResponse, error) {
	body := fmt.Sprintf(`{"client_id":"%s","scope":"read:user"}`, githubClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ghUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed: %d %s", resp.StatusCode, string(b))
	}
	var out ghDeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode device code response: %w", err)
	}
	if out.Interval == 0 {
		out.Interval = 5
	}
	return &out, nil
}

// PollGithubDeviceAuth polls GitHub for device-code completion and returns an access token.
func PollGithubDeviceAuth(ctx context.Context, deviceCode string, intervalSec int) (string, error) {
	if intervalSec <= 0 {
		intervalSec = 5
	}
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			body := fmt.Sprintf(`{"client_id":"%s","device_code":"%s","grant_type":"urn:ietf:params:oauth:grant-type:device_code"}`, githubClientID, deviceCode)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, accessTokenURL, strings.NewReader(body))
			if err != nil {
				return "", err
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", ghUserAgent)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			var data ghAccessTokenResponse
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("token poll failed: %d %s", resp.StatusCode, string(b))
			}
			_ = json.Unmarshal(b, &data)
			if data.AccessToken != "" {
				return data.AccessToken, nil
			}
			switch data.Error {
			case "authorization_pending":
				// keep polling
			case "slow_down":
				// increase interval modestly
				intervalSec += 5
				ticker.Reset(time.Duration(intervalSec) * time.Second)
			case "expired_token", "access_denied", "unsupported_grant_type", "incorrect_client_credentials", "incorrect_device_code":
				return "", fmt.Errorf("github device auth failed: %s", data.Error)
			default:
				// keep polling, unknown error may be transient
			}
		}
	}
}

// FetchCopilotToken exchanges the GitHub OAuth token for a Copilot API token.
// Returns token and expiry (ms since epoch).
func FetchCopilotToken(ctx context.Context, githubOAuthToken string) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotTokenURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+githubOAuthToken)
	req.Header.Set("User-Agent", ghUserAgent)
	req.Header.Set("Editor-Version", ghEditorVersion)
	req.Header.Set("Editor-Plugin-Version", ghPluginVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("copilot token request failed: %d %s", resp.StatusCode, string(b))
	}
	var data ghCopilotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", 0, fmt.Errorf("failed to decode copilot token response: %w", err)
	}
	// API returns seconds since epoch; convert to ms like the rest of auth.
	expiresAtMs := data.ExpiresAt * 1000
	return data.Token, expiresAtMs, nil
}
