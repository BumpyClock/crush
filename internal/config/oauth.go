package config

import "github.com/charmbracelet/catwalk/pkg/catwalk"

// IsOAuthProvider checks if a provider ID supports OAuth authentication
// This is a simple, direct approach without unnecessary abstractions
func IsOAuthProvider(providerID string) bool {
	switch providerID {
	case "claudesub":
		return true
	case "github-copilot":
		return true
	default:
		return false
	}
}

// ListOAuthProviders returns all OAuth-capable provider IDs
func ListOAuthProviders() []string {
	return []string{"claudesub", "github-copilot"}
}

// GetDefaultOAuthModels returns default models for OAuth providers
// This provides fallback models when no config models are specified
func GetDefaultOAuthModels(providerID string) []catwalk.Model {
	switch providerID {
	case "claudesub":
		return DefaultModels("claudesub")
	case "github-copilot":
		return DefaultModels("github-copilot")
	default:
		return nil
	}
}
