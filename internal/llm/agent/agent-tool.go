package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

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

const agentToolDescriptionTemplate = `Launch a new agent to handle complex, multi-step tasks autonomously. 

Available agent types and the tools they have access to:
{agents}

When using the Agent tool, you must specify a agent_name parameter to select which agent type to use.

When to use the Agent tool:
- If you are searching for a keyword like "config" or "logger", or for questions like "which file does X?", the Agent tool is strongly recommended
- If you want to read a specific file path, use the View or Glob tool instead of the Agent tool, to find the match more quickly
- If you are searching for a specific class definition like "class Foo", use the Glob tool instead, to find the match more quickly
- Other tasks that are not related to the agent descriptions above

Usage notes:
1. Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses
2. **CRITICAL**: The result returned by the agent is NOT visible to the user. You MUST synthesize the agent's response into your own message to communicate findings to the user.
3. Each agent invocation is stateless. You will not be able to send additional messages to the agent, nor will the agent be able to communicate with you outside of its final report. Therefore, your prompt should contain a highly detailed task description for the agent to perform autonomously and you should specify exactly what information the agent should return back to you in its final and only message to you.
4. The agent's outputs should generally be trusted
5. IMPORTANT: The agent has access to ALL tools including file modification tools (Bash, Edit, MultiEdit, Write). You can delegate complex tasks to sub-agents to perform when you decide, allowing the main crush agent to preserve context.

## Synthesis Guidelines

**CRITICAL**: Sub-agent responses are invisible to users. You must translate their work into meaningful user-facing responses with appropriate detail.

### Synthesis Patterns by Task Type

**Code Discovery/Search**:
- Good: "I found the authentication logic in auth/handler.go:89-156. It uses JWT tokens with role-based validation."
- Poor: "The agent found the code."

**Code Review/Analysis**:
- Good: "The code reviewer identified 3 critical issues: SQL injection vulnerability in queries.go:45, missing error handling in api.go:123, and deprecated function usage in utils.go:67."
- Poor: "The agent reviewed the code and found issues."

**Implementation/Fixes**:
- Good: "I've implemented the new caching layer using Redis. Added cache middleware in middleware/cache.go and updated the user service in services/user.go to use cached lookups."
- Poor: "I've completed the implementation."

**Testing/Validation**:
- Good: "The test runner executed 47 tests with 3 failures: user validation test fails due to missing email format check, API timeout test fails in integration suite, and login flow test has assertion errors."
- Poor: "The agent ran the tests."

**Debugging/Investigation**:
- Good: "The debugger traced the memory leak to the event listeners in dashboard.js:234. The issue occurs because cleanup handlers aren't being called when components unmount."
- Poor: "The agent found the bug."

**Refactoring/Optimization**:
- Good: "The refactorer consolidated 4 duplicate data transformation functions into a single utility in utils/transform.go:45. This reduces code duplication by 150 lines and improves maintainability."
- Poor: "The agent refactored the code."

**Error/Partial Results**:
- Good: "The task agent attempted to fix the failing tests but encountered permission issues accessing test/fixtures/. Manual intervention needed to update file permissions."
- Poor: "The agent couldn't complete the task."

### Quality Requirements

1. **Be specific**: Include file paths, line numbers, function names, and exact findings
2. **Provide context**: Explain WHY something matters, not just WHAT was found
3. **Include next steps**: When relevant, mention what the user should do next
4. **Acknowledge limitations**: If the sub-agent had partial success, explain what's left
5. **Match detail to complexity**: Simple tasks need brief summaries, complex analysis needs comprehensive details

Always provide enough detail that the user understands what was accomplished and can take appropriate action.`

type AgentParams struct {
	Prompt    string `json:"prompt"`
	AgentName string `json:"agent_name,omitempty"` // Optional: specify which agent to use
}

func (b *agentTool) Name() string {
	return AgentToolName
}

func (b *agentTool) Info() tools.ToolInfo {
	// Build agent list dynamically
	var agentList []string
	for name, config := range b.agentConfigs {
		if name == "coder" {
			continue // Skip coder to avoid recursion
		}
		if config.Disabled {
			continue // Skip disabled agents
		}

		description := config.Description
		if description == "" {
			description = "Specialized agent for various tasks"
		}

		agentList = append(agentList, fmt.Sprintf("- %s: %s", name, description))
	}

	// Sort for consistent ordering
	sort.Strings(agentList)

	// Replace placeholder in template
	description := strings.Replace(agentToolDescriptionTemplate,
		"{agents}",
		strings.Join(agentList, "\n"),
		1)

	return tools.ToolInfo{
		Name:        AgentToolName,
		Description: description,
		Parameters: map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"agent_name": map[string]any{
				"type":        "string",
				"description": "Name of the specific agent to use. Required. Choose from the available agents listed above.",
			},
		},
		Required: []string{"prompt", "agent_name"},
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

	// Agent name is now required
	agentName := params.AgentName
	if agentName == "" {
		return tools.NewTextErrorResponse("agent_name is required. Check the tool description for available agents."), nil
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

	// Create a new context with a reasonable timeout for sub-agents
	// This prevents the sub-agent from inheriting a nearly-expired context
	subAgentCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Copy essential values from parent context
	if sessionID != "" {
		subAgentCtx = context.WithValue(subAgentCtx, tools.SessionIDContextKey, sessionID)
	}
	if messageID != "" {
		subAgentCtx = context.WithValue(subAgentCtx, tools.MessageIDContextKey, messageID)
	}

	done, err := agent.Run(subAgentCtx, session.ID, params.Prompt)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", err)
	}

	// Wait for result with a timeout
	var response message.Message
	select {
	case result := <-done:
		if result.Error != nil {
			return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", result.Error)
		}
		response = result.Message
	case <-subAgentCtx.Done():
		return tools.ToolResponse{}, fmt.Errorf("sub-agent timed out after 30 minutes")
	}

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

	// Handle empty finish reasons - attempt recovery
	finishReason := response.FinishReason()
	if finishReason == "" {
		slog.Warn("Sub-agent returned empty finish reason, attempting recovery",
			"agent", agentName,
			"has_tool_calls", len(response.ToolCalls()) > 0,
			"has_content", response.Content().String() != "",
		)

		// Try to determine the appropriate finish reason based on message state
		if len(response.ToolCalls()) > 0 {
			// If there are tool calls, it should be tool use
			finishReason = message.FinishReasonToolUse
			slog.Info("Recovered finish reason as tool_use", "agent", agentName)
		} else if response.Content().String() != "" {
			// If there's content but no tools, assume end turn
			finishReason = message.FinishReasonEndTurn
			slog.Info("Recovered finish reason as end_turn", "agent", agentName)
		} else {
			// No content and no tools - likely cancelled or error
			finishReason = message.FinishReasonCanceled
			slog.Warn("Recovered finish reason as canceled due to no content or tools", "agent", agentName)
		}
	}

	// Handle different finish reasons more gracefully
	if finishReason == message.FinishReasonCanceled || finishReason == message.FinishReasonError {
		errMsg := "Sub-agent execution was interrupted"
		if finishReason == message.FinishReasonError {
			errMsg = "Sub-agent encountered an error"
		}
		slog.Warn("Agent execution issue", "agent", agentName, "finish_reason", finishReason)
		return tools.NewTextErrorResponse(errMsg), nil
	}

	if response.Role != message.Assistant {
		slog.Warn("Agent returned non-assistant message", "role", response.Role, "agent", agentName)
		// Try to extract any useful content from tool results
		if response.Role == message.Tool {
			for _, part := range response.Parts {
				if tr, ok := part.(message.ToolResult); ok && !tr.IsError {
					if tr.Content != "" {
						return tools.NewTextResponse(tr.Content), nil
					}
				}
			}
		}
		return tools.NewTextErrorResponse("Sub-agent did not produce a valid response"), nil
	}

	// Check if the response has actual content - try comprehensive recovery strategies
	contentStr := response.Content().String()
	if contentStr == "" {
		slog.Warn("Agent returned empty content, attempting comprehensive recovery",
			"agent", agentName,
			"prompt_length", len(params.Prompt),
			"parts_count", len(response.Parts),
			"tool_calls", len(response.ToolCalls()),
		)

		// Strategy 1: Check for reasoning content
		reasoningContent := response.ReasoningContent()
		if reasoningContent.Thinking != "" {
			slog.Info("Using reasoning content as response", "agent", agentName)
			contentStr = "Sub-agent analysis: " + reasoningContent.Thinking
		} else {
			// Strategy 2: Check for tool results with useful content
			var toolOutputs []string
			var errorOutputs []string
			for _, part := range response.Parts {
				if tr, ok := part.(message.ToolResult); ok {
					if tr.IsError && tr.Content != "" {
						errorOutputs = append(errorOutputs, fmt.Sprintf("%s error: %s", tr.Name, tr.Content))
					} else if !tr.IsError && tr.Content != "" {
						toolOutputs = append(toolOutputs, fmt.Sprintf("%s: %s", tr.Name, tr.Content))
					}
				}
			}

			if len(toolOutputs) > 0 {
				slog.Info("Using tool result content as response", "agent", agentName, "tool_results_count", len(toolOutputs))
				contentStr = "Sub-agent completed task with findings:\n" + strings.Join(toolOutputs, "\n")
			} else if len(errorOutputs) > 0 {
				slog.Info("Using tool error content as response", "agent", agentName, "error_count", len(errorOutputs))
				contentStr = "Sub-agent encountered issues:\n" + strings.Join(errorOutputs, "\n")
			} else {
				// Strategy 3: Check text content in message parts
				var textParts []string
				for _, part := range response.Parts {
					if textPart, ok := part.(message.TextContent); ok && textPart.Text != "" {
						textParts = append(textParts, textPart.Text)
					}
				}

				if len(textParts) > 0 {
					slog.Info("Using text parts as response", "agent", agentName, "text_parts", len(textParts))
					contentStr = strings.Join(textParts, " ")
				} else if len(response.ToolCalls()) > 0 {
					// Strategy 4: Tool calls without content - indicate activity
					var toolNames []string
					for _, tc := range response.ToolCalls() {
						toolNames = append(toolNames, tc.Name)
					}
					slog.Info("Using tool call summary as response", "agent", agentName, "tools_used", toolNames)
					contentStr = fmt.Sprintf("Sub-agent executed %d tools (%s) but produced no text output. The agent may have completed its task through tool actions.",
						len(toolNames), strings.Join(toolNames, ", "))
				} else {
					// Strategy 5: Final fallback with detailed diagnostics
					slog.Error("All recovery strategies failed - truly empty response",
						"agent", agentName,
						"finish_reason", finishReason,
						"parts_count", len(response.Parts),
						"has_tool_calls", len(response.ToolCalls()) > 0,
						"has_reasoning", reasoningContent.Thinking != "",
						"message_role", response.Role,
					)
					return tools.NewTextErrorResponse(fmt.Sprintf(
						"Sub-agent '%s' completed but produced no output. "+
							"This typically indicates the agent encountered an issue but didn't report it properly. "+
							"Possible causes: task was unclear, agent lacked necessary permissions, or an internal error occurred. "+
							"Consider re-running with more specific instructions or checking agent logs.",
						agentName)), nil
				}
			}
		}

		// Validate that we actually recovered something meaningful
		if contentStr == "" || strings.TrimSpace(contentStr) == "" {
			slog.Error("Recovery failed - still no content after all strategies",
				"agent", agentName,
				"finish_reason", finishReason,
			)
			return tools.NewTextErrorResponse(fmt.Sprintf(
				"Sub-agent '%s' failed to provide any response despite multiple recovery attempts. "+
					"This suggests a serious issue with the agent or task execution. "+
					"Please check the task requirements and try again with simpler instructions.",
				agentName)), nil
		}
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
	return tools.NewTextResponse(contentStr), nil
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
