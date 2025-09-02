package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAgentToolWithCustomAgents(t *testing.T) {
	t.Run("loads and uses custom agents", func(t *testing.T) {
		// Create a temporary directory for testing
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir) // Set HOME for consistent test environment

		// Create a custom agent file
		agentDir := filepath.Join(tempDir, ".crush", "agents")
		require.NoError(t, os.MkdirAll(agentDir, 0o755))

		agentContent := `---
name: test-custom
description: Custom test agent
tools: view, grep
---

You are a custom test agent.`

		require.NoError(t, os.WriteFile(
			filepath.Join(agentDir, "test-custom.md"),
			[]byte(agentContent),
			0o644,
		))

		// Initialize config with the temp directory
		cfg, err := config.Init(tempDir, ".crush", false)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Verify the custom agent was loaded
		require.Contains(t, cfg.Agents, "test-custom")
		agent := cfg.Agents["test-custom"]
		require.Equal(t, "test-custom", agent.ID)
		require.Equal(t, "Custom test agent", agent.Description)
		require.Equal(t, []string{"view", "grep"}, agent.AllowedTools)

		// Verify the agent definition was stored
		require.NotNil(t, cfg.AgentDefinitions)
		require.Contains(t, cfg.AgentDefinitions, "test-custom")
		def := cfg.AgentDefinitions["test-custom"]
		require.Equal(t, "You are a custom test agent.", def.SystemPrompt)
	})

	t.Run("creates sub-agents for coder agent", func(t *testing.T) {
		// Create a temporary directory for testing
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)

		// Create custom agent files
		agentDir := filepath.Join(tempDir, ".crush", "agents")
		require.NoError(t, os.MkdirAll(agentDir, 0o755))

		// Create multiple custom agents
		agents := []struct {
			name string
			desc string
		}{
			{"reviewer", "Code reviewer"},
			{"debugger", "Debugger agent"},
			{"tester", "Test runner"},
		}

		for _, ag := range agents {
			content := fmt.Sprintf(`---
name: %s
description: %s
---

System prompt for %s.`, ag.name, ag.desc, ag.name)
			require.NoError(t, os.WriteFile(
				filepath.Join(agentDir, ag.name+".md"),
				[]byte(content),
				0o644,
			))
		}

		// Initialize config
		cfg, err := config.Init(tempDir, ".crush", false)
		require.NoError(t, err)

		// Verify all agents were loaded
		require.GreaterOrEqual(t, len(cfg.Agents), 5) // coder, task, plus 3 custom
		for _, ag := range agents {
			require.Contains(t, cfg.Agents, ag.name)
		}

		// Mock services for agent creation
		// Note: In a real test, you'd use proper mocks or test implementations
		// This is a simplified example to show the structure
	})

	t.Run("agent tool uses descriptive names", func(t *testing.T) {
		// Create a temporary directory for testing
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)

		// Create a custom agent with a specific name
		agentDir := filepath.Join(tempDir, ".crush", "agents")
		require.NoError(t, os.MkdirAll(agentDir, 0o755))

		agentContent := `---
name: code-reviewer
description: Expert code review specialist
tools: view, grep
---

Code review system prompt.`

		require.NoError(t, os.WriteFile(
			filepath.Join(agentDir, "code-reviewer.md"),
			[]byte(agentContent),
			0o644,
		))

		// Initialize config
		cfg, err := config.Init(tempDir, ".crush", false)
		require.NoError(t, err)

		// Verify the agent config has the right name
		agent, exists := cfg.Agents["code-reviewer"]
		require.True(t, exists)
		require.Equal(t, "code-reviewer", agent.ID)
		require.Equal(t, "code-reviewer", agent.Name) // This will be used as session title
	})
}

func TestFormatAgentName(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"task agent", "task", "Task Agent"},
		{"code reviewer", "code-reviewer", "Code Reviewer"},
		{"debugger", "debugger", "Debugger"},
		{"test runner", "test-runner", "Test Runner"},
		{"refactorer", "refactorer", "Refactorer"},
		{"empty string", "", "Agent"},
		{"custom agent", "custom-special-agent", "Custom Special Agent"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatAgentName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestAgentToolInvocation(t *testing.T) {
	t.Run("invokes correct agent by name", func(t *testing.T) {
		// This test would require mocking the Service interface
		// and verifying that the correct agent is invoked
		// when agent_name is specified in the parameters

		// Example structure (would need actual mock implementations):
		/*
			mockAgents := map[string]Service{
				"task": mockTaskAgent,
				"reviewer": mockReviewerAgent,
				"debugger": mockDebuggerAgent,
			}

			tool := NewAgentTool(mockAgents, mockSessions, mockMessages)

			// Test invoking specific agent
			params := AgentParams{
				Prompt: "Review this code",
				AgentName: "reviewer",
			}

			// Verify the reviewer agent was called
		*/
	})
}
