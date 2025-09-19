package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/auth"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/llm/tools"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/openai/openai-go/option"
)

type copilotSubClient struct {
	providerOptions providerClientOptions
	openaiClient    OpenAIClient
	authManager     *auth.AuthManager
	refreshMu       sync.Mutex
	lastRefresh     time.Time
}

func newCopilotSubClient(opts providerClientOptions) (ProviderClient, error) {
	cfg := config.Get()
	if cfg == nil {
		slog.Error("Configuration not loaded for github-copilot provider")
		return nil, fmt.Errorf("configuration not loaded")
	}

	authManager := auth.NewAuthManager(config.GlobalDataDir())

	// Build HTTP client that injects Copilot token + headers.
	httpClient, err := createCopilotSubHTTPClient()
	if err != nil {
		slog.Error("Failed to create Copilot OAuth HTTP client", "error", err)
		return nil, fmt.Errorf("failed to create http client: %w", err)
	}

	// Build OpenAI client pointing at Copilot API.
	openaiOpts := []option.RequestOption{
		option.WithHTTPClient(httpClient),
	}
	// Prefer baseURL from config if present, otherwise use known Copilot endpoint.
	baseURL := opts.baseURL
	if baseURL == "" {
		baseURL = "https://api.githubcopilot.com"
	}
	openaiOpts = append(openaiOpts, option.WithBaseURL(baseURL))

	// Add any extra headers/body from config.
	for k, v := range opts.extraHeaders {
		openaiOpts = append(openaiOpts, option.WithHeaderAdd(k, v))
	}
	for k, v := range opts.extraBody {
		openaiOpts = append(openaiOpts, option.WithJSONSet(k, v))
	}

	client := newOpenAIClientWithOptions(opts, openaiOpts)

	return &copilotSubClient{
		providerOptions: opts,
		openaiClient:    client,
		authManager:     authManager,
	}, nil
}

func (c *copilotSubClient) send(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (*ProviderResponse, error) {
	return c.openaiClient.send(ctx, messages, tools)
}

func (c *copilotSubClient) stream(ctx context.Context, messages []message.Message, tools []tools.BaseTool) <-chan ProviderEvent {
	return c.openaiClient.stream(ctx, messages, tools)
}

func (c *copilotSubClient) Model() catwalk.Model {
	if c.providerOptions.modelType != "" {
		return c.providerOptions.model(c.providerOptions.modelType)
	}
	return c.openaiClient.Model()
}

// Register github-copilot OAuth provider in registry.
func init() {
	MustRegisterProvider(&ProviderRegistration{
		ID:            "github-copilot",
		Name:          "GitHub Copilot Subscription",
		Type:          catwalk.TypeOpenAI,
		SupportsOAuth: true,
		Constructor: func(opts providerClientOptions) (ProviderClient, error) {
			return newCopilotSubClient(opts)
		},
	})
}
