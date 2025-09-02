package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/llm/tools"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

type agentTool struct {
	agents       map[string]Service      // Map of agent name to agent service
	agentConfigs map[string]config.Agent // Map of agent name to agent config
	sessions     session.Service
	messages     message.Service
}

const (
	AgentToolName = "agent"
)

type AgentParams struct {
	Prompt    string `json:"prompt"`
	AgentName string `json:"agent_name,omitempty"` // Optional: specify which agent to use
}

func (b *agentTool) Name() string {
	return AgentToolName
}

func (b *agentTool) Info() tools.ToolInfo {
	return tools.ToolInfo{
		Name:        AgentToolName,
		Description: "Launch a new agent that has access to the following tools: Bash, Download, Edit, MultiEdit, Fetch, Glob, Grep, LS, Sourcegraph, View, Write, Diagnostics (if LSP is enabled), and any configured MCP tools. When you are searching for a keyword or file and are not confident that you will find the right match on the first try, use the Agent tool to perform the search for you. For example:\n\n- If you are searching for a keyword like \"config\" or \"logger\", or for questions like \"which file does X?\", the Agent tool is strongly recommended\n- If you want to read a specific file path, use the View or GlobTool tool instead of the Agent tool, to find the match more quickly\n- If you are searching for a specific class definition like \"class Foo\", use the GlobTool tool instead, to find the match more quickly\n\nUsage notes:\n1. Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses\n2. When the agent is done, it will return a single message back to you. The result returned by the agent is not visible to the user. To show the user the result, you should send a text message back to the user with a concise summary of the result.\n3. Each agent invocation is stateless. You will not be able to send additional messages to the agent, nor will the agent be able to communicate with you outside of its final report. Therefore, your prompt should contain a highly detailed task description for the agent to perform autonomously and you should specify exactly what information the agent should return back to you in its final and only message to you.\n4. The agent's outputs should generally be trusted\n5. IMPORTANT: The agent has access to ALL tools including file modification tools (Bash, Edit, MultiEdit, Write). You can delegate complex tasks to sub-agents to perform when you decide, allowing the main crush agent to preserve context.",
		Parameters: map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"agent_name": map[string]any{
				"type":        "string",
				"description": "Optional: name of a specific agent to use (e.g., 'task', 'coder', or a custom agent name). If not specified, uses the default 'task' agent.",
			},
		},
		Required: []string{"prompt"},
	}
}

func (b *agentTool) Run(ctx context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	var params AgentParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return tools.NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if params.Prompt == "" {
		return tools.NewTextErrorResponse("prompt is required"), nil
	}

	// Default to task agent if not specified
	agentName := params.AgentName
	if agentName == "" {
		agentName = "task"
	}

	// Log available agents for debugging
	var availableAgents []string
	for name := range b.agents {
		availableAgents = append(availableAgents, name)
	}
	slog.Info("Agent tool invoked", "requested_agent", agentName, "available_agents", availableAgents)

	// Get the appropriate agent
	agent, exists := b.agents[agentName]
	if !exists {
		return tools.NewTextErrorResponse(fmt.Sprintf("agent '%s' not found. Available agents: %v", agentName, availableAgents)), nil
	}

	sessionID, messageID := tools.GetContextValues(ctx)
	if sessionID == "" || messageID == "" {
		return tools.ToolResponse{}, fmt.Errorf("session_id and message_id are required")
	}

	// Get the agent configuration for a better session name
	agentConfig, hasConfig := b.agentConfigs[agentName]
	var sessionTitle string
	if hasConfig && agentConfig.Name != "" {
		// Use the descriptive name from the config
		sessionTitle = agentConfig.Name
	} else {
		// Fallback to the agent ID with proper capitalization
		sessionTitle = FormatAgentName(agentName)
	}

	session, err := b.sessions.CreateTaskSession(ctx, call.ID, sessionID, sessionTitle)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error creating session: %s", err)
	}

	done, err := agent.Run(ctx, session.ID, params.Prompt)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", err)
	}
	result := <-done
	if result.Error != nil {
		return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", result.Error)
	}

	response := result.Message
	// Debug logging to understand why "no response" is happening
	var partTypes []string
	for _, part := range response.Parts {
		partTypes = append(partTypes, fmt.Sprintf("%T", part))
	}
	slog.Info("Agent tool response details", 
		"agent", agentName,
		"message_role", response.Role,
		"message_id", response.ID,
		"content_text", response.Content().String(),
		"content_length", len(response.Content().String()),
		"finish_reason", response.FinishReason(),
		"parts_count", len(response.Parts),
		"part_types", partTypes,
	)
	if response.Role != message.Assistant {
		slog.Warn("Agent returned non-assistant message", "role", response.Role)
		return tools.NewTextErrorResponse("no response"), nil
	}
	
	// Check if the response has actual content
	contentStr := response.Content().String()
	if contentStr == "" {
		slog.Warn("Agent returned empty content", 
			"agent", agentName,
			"finish_reason", response.FinishReason(),
		)
		return tools.NewTextErrorResponse("no response"), nil
	}

	updatedSession, err := b.sessions.Get(ctx, session.ID)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error getting session: %s", err)
	}
	parentSession, err := b.sessions.Get(ctx, sessionID)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error getting parent session: %s", err)
	}

	parentSession.Cost += updatedSession.Cost

	_, err = b.sessions.Save(ctx, parentSession)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error saving parent session: %s", err)
	}
	return tools.NewTextResponse(response.Content().String()), nil
}

// FormatAgentName formats an agent ID into a proper display name
func FormatAgentName(name string) string {
	switch name {
	case "task":
		return "Task Agent"
	case "code-reviewer":
		return "Code Reviewer"
	case "debugger":
		return "Debugger"
	case "test-runner":
		return "Test Runner"
	case "refactorer":
		return "Refactorer"
	default:
		// Capitalize first letter and replace hyphens with spaces
		if name == "" {
			return "Agent"
		}
		formatted := strings.ReplaceAll(name, "-", " ")
		// Capitalize each word
		words := strings.Fields(formatted)
		for i, word := range words {
			if len(word) > 0 {
				words[i] = strings.ToUpper(word[:1]) + word[1:]
			}
		}
		return strings.Join(words, " ")
	}
}

func NewAgentTool(
	agents map[string]Service, // Now accepts multiple agents
	agentConfigs map[string]config.Agent, // Agent configurations for names
	sessions session.Service,
	messages message.Service,
) tools.BaseTool {
	return &agentTool{
		sessions:     sessions,
		messages:     messages,
		agents:       agents,
		agentConfigs: agentConfigs,
	}
}
