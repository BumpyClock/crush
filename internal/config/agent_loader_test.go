package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAgentDefinitions(t *testing.T) {
	t.Run("loads project and user agents", func(t *testing.T) {
		// Create temporary directories
		tempDir := t.TempDir()
		projectAgentDir := filepath.Join(tempDir, ".crush", "agents")
		require.NoError(t, os.MkdirAll(projectAgentDir, 0o755))

		// Create a project agent
		projectAgent := `---
name: test-agent
description: Test agent for unit tests
tools: bash, view
---

This is a test agent system prompt.`
		require.NoError(t, os.WriteFile(
			filepath.Join(projectAgentDir, "test-agent.md"),
			[]byte(projectAgent),
			0o644,
		))

		// Load agents
		agents, err := LoadAgentDefinitions(tempDir)
		require.NoError(t, err)
		require.Len(t, agents, 1)

		// Verify agent properties
		agent, exists := agents["test-agent"]
		require.True(t, exists)
		require.Equal(t, "test-agent", agent.Name)
		require.Equal(t, "Test agent for unit tests", agent.Description)
		require.Equal(t, []string{"bash", "view"}, agent.Tools)
		require.Equal(t, "This is a test agent system prompt.", agent.SystemPrompt)
		require.True(t, agent.IsPriority) // Project-level agent
	})

	t.Run("project agents override user agents", func(t *testing.T) {
		// Create temporary directories
		tempDir := t.TempDir()
		projectAgentDir := filepath.Join(tempDir, ".crush", "agents")
		require.NoError(t, os.MkdirAll(projectAgentDir, 0o755))

		// Create both project and user agents with same name
		projectAgent := `---
name: shared-agent
description: Project version
---

Project agent prompt.`
		require.NoError(t, os.WriteFile(
			filepath.Join(projectAgentDir, "shared.md"),
			[]byte(projectAgent),
			0o644,
		))

		// Load agents
		agents, err := LoadAgentDefinitions(tempDir)
		require.NoError(t, err)

		// Verify project agent takes precedence
		agent, exists := agents["shared-agent"]
		require.True(t, exists)
		require.Equal(t, "Project version", agent.Description)
		require.Equal(t, "Project agent prompt.", agent.SystemPrompt)
	})

	t.Run("handles missing directories gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		// Don't create any agent directories

		agents, err := LoadAgentDefinitions(tempDir)
		require.NoError(t, err)
		require.Empty(t, agents)
	})

	t.Run("skips invalid agent files", func(t *testing.T) {
		tempDir := t.TempDir()
		agentDir := filepath.Join(tempDir, ".crush", "agents")
		require.NoError(t, os.MkdirAll(agentDir, 0o755))

		// Create invalid agent (missing required fields)
		invalidAgent := `---
name: invalid-agent
---

Missing description field.`
		require.NoError(t, os.WriteFile(
			filepath.Join(agentDir, "invalid.md"),
			[]byte(invalidAgent),
			0o644,
		))

		// Create valid agent
		validAgent := `---
name: valid-agent
description: Valid agent
---

Valid prompt.`
		require.NoError(t, os.WriteFile(
			filepath.Join(agentDir, "valid.md"),
			[]byte(validAgent),
			0o644,
		))

		agents, err := LoadAgentDefinitions(tempDir)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		_, exists := agents["valid-agent"]
		require.True(t, exists)
	})
}

func TestParseAgentMarkdown(t *testing.T) {
	t.Run("parses valid agent markdown", func(t *testing.T) {
		content := `---
name: test-agent
description: Test description
tools: bash, view, edit
---

This is the system prompt.
It can have multiple lines.`

		agent, err := parseAgentMarkdown(content)
		require.NoError(t, err)
		require.NotNil(t, agent)
		require.Equal(t, "test-agent", agent.Name)
		require.Equal(t, "Test description", agent.Description)
		require.Equal(t, []string{"bash", "view", "edit"}, agent.Tools)
		require.Equal(t, "This is the system prompt.\nIt can have multiple lines.", agent.SystemPrompt)
	})

	t.Run("handles comma-separated tools string", func(t *testing.T) {
		content := `---
name: test-agent
description: Test description
tools: "bash, view, edit"
---

Prompt.`

		agent, err := parseAgentMarkdown(content)
		require.NoError(t, err)
		require.Equal(t, []string{"bash", "view", "edit"}, agent.Tools)
	})

	t.Run("handles missing tools field", func(t *testing.T) {
		content := `---
name: test-agent
description: Test description
---

Prompt.`

		agent, err := parseAgentMarkdown(content)
		require.NoError(t, err)
		require.Nil(t, agent.Tools) // Should be nil to inherit all tools
	})

	t.Run("returns error for missing name", func(t *testing.T) {
		content := `---
description: Test description
---

Prompt.`

		_, err := parseAgentMarkdown(content)
		require.Error(t, err)
		require.Contains(t, err.Error(), "name is required")
	})

	t.Run("returns error for missing description", func(t *testing.T) {
		content := `---
name: test-agent
---

Prompt.`

		_, err := parseAgentMarkdown(content)
		require.Error(t, err)
		require.Contains(t, err.Error(), "description is required")
	})

	t.Run("returns error for missing system prompt", func(t *testing.T) {
		content := `---
name: test-agent
description: Test
---

`

		_, err := parseAgentMarkdown(content)
		require.Error(t, err)
		require.Contains(t, err.Error(), "system prompt is required")
	})
}

func TestConvertDefinitionToAgent(t *testing.T) {
	t.Run("converts with all tools", func(t *testing.T) {
		def := AgentDefinition{
			Name:         "test-agent",
			Description:  "Test description",
			Tools:        nil, // No tools specified = all tools
			SystemPrompt: "Test prompt",
		}

		agent := ConvertDefinitionToAgent(def, SelectedModelTypeLarge)
		require.Equal(t, "test-agent", agent.ID)
		require.Equal(t, "test-agent", agent.Name)
		require.Equal(t, "Test description", agent.Description)
		require.Equal(t, SelectedModelTypeLarge, agent.Model)
		require.Nil(t, agent.AllowedTools) // nil means all tools
	})

	t.Run("converts with specific tools", func(t *testing.T) {
		def := AgentDefinition{
			Name:         "test-agent",
			Description:  "Test description",
			Tools:        []string{"bash", "view"},
			SystemPrompt: "Test prompt",
		}

		agent := ConvertDefinitionToAgent(def, SelectedModelTypeSmall)
		require.Equal(t, []string{"bash", "view"}, agent.AllowedTools)
		require.Equal(t, SelectedModelTypeSmall, agent.Model)
	})

	t.Run("converts with MCP servers", func(t *testing.T) {
		def := AgentDefinition{
			Name:         "test-agent",
			Description:  "Test description",
			MCPServers:   []string{"server1", "server2"},
			SystemPrompt: "Test prompt",
		}

		agent := ConvertDefinitionToAgent(def, SelectedModelTypeLarge)
		require.NotNil(t, agent.AllowedMCP)
		require.Len(t, agent.AllowedMCP, 2)
		require.Contains(t, agent.AllowedMCP, "server1")
		require.Contains(t, agent.AllowedMCP, "server2")
	})
}
