package config

import "github.com/charmbracelet/catwalk/pkg/catwalk"

// Default model catalogue for OAuth/subscription providers (single source of truth).
// Costs are zeroed elsewhere for subscription contexts.
var defaultOAuthModels = map[string][]catwalk.Model{
	"claudesub": {
		{ID: "claude-opus-4-1-20250805", Name: "Claude Opus 4.1", ContextWindow: 200000, DefaultMaxTokens: 32000, CanReason: true, HasReasoningEffort: false, SupportsImages: true},
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", ContextWindow: 200000, DefaultMaxTokens: 32000, CanReason: true, HasReasoningEffort: false, SupportsImages: true},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextWindow: 200000, DefaultMaxTokens: 8192, CanReason: true, HasReasoningEffort: true, DefaultReasoningEffort: "medium", SupportsImages: true},
		{ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", ContextWindow: 200000, DefaultMaxTokens: 8192, CanReason: true, HasReasoningEffort: true, DefaultReasoningEffort: "medium", SupportsImages: true},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet (New)", ContextWindow: 200000, DefaultMaxTokens: 8192, CanReason: true, HasReasoningEffort: true, DefaultReasoningEffort: "medium", SupportsImages: true},
		{ID: "claude-3-5-sonnet-20240620", Name: "Claude 3.5 Sonnet (Old)", ContextWindow: 200000, DefaultMaxTokens: 8192, CanReason: true, HasReasoningEffort: true, DefaultReasoningEffort: "medium", SupportsImages: true},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", ContextWindow: 200000, DefaultMaxTokens: 5000, CanReason: false, SupportsImages: true},
	},
	"github-copilot": {
		{ID: "gpt-4o", Name: "GitHub Copilot (GPT-4o)", ContextWindow: 128000, DefaultMaxTokens: 8192, CanReason: true, SupportsImages: true},
		{ID: "gpt-4o-mini", Name: "GitHub Copilot (GPT-4o-mini)", ContextWindow: 128000, DefaultMaxTokens: 8192, CanReason: false, SupportsImages: true},
	},
}

// DefaultModels returns a copy of the default model slice for a provider.
func DefaultModels(providerID string) []catwalk.Model {
	models, ok := defaultOAuthModels[providerID]
	if !ok || len(models) == 0 {
		return nil
	}
	out := make([]catwalk.Model, len(models))
	copy(out, models)
	return out
}
