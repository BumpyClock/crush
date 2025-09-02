package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentDefinition represents an agent loaded from a markdown file with YAML frontmatter
type AgentDefinition struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Tools        []string `yaml:"tools,omitempty"`       // If omitted, inherits all tools
	MCPServers   []string `yaml:"mcp_servers,omitempty"` // If omitted, inherits all MCP servers
	LSPServers   []string `yaml:"lsp_servers,omitempty"` // If omitted, inherits all LSP servers
	SystemPrompt string   `yaml:"-"`                     // Loaded from markdown body
	FilePath     string   `yaml:"-"`                     // Source file path
	IsPriority   bool     `yaml:"-"`                     // True if project-level (higher priority)
}

// LoadAgentDefinitions loads agent definitions from project and user directories
func LoadAgentDefinitions(workingDir string) (map[string]AgentDefinition, error) {
	agents := make(map[string]AgentDefinition)

	// Load user-level agents first (lower priority)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userAgentDir := filepath.Join(homeDir, ".crush", "agents")
		if err := loadAgentsFromDir(userAgentDir, agents, false); err != nil {
			// Log but don't fail if user agents can't be loaded
			fmt.Printf("Warning: Failed to load user agents: %v\n", err)
		}
	}

	// Load project-level agents (higher priority, will override user agents)
	projectAgentDir := filepath.Join(workingDir, ".crush", "agents")
	if err := loadAgentsFromDir(projectAgentDir, agents, true); err != nil {
		// Log but don't fail if project agents can't be loaded
		fmt.Printf("Warning: Failed to load project agents: %v\n", err)
	}

	return agents, nil
}

func loadAgentsFromDir(dir string, agents map[string]AgentDefinition, isPriority bool) error {
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // Directory doesn't exist, not an error
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read agent directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		agent, err := loadAgentFromFile(filePath)
		if err != nil {
			// Log but continue with other agents
			fmt.Printf("Warning: Failed to load agent from %s: %v\n", filePath, err)
			continue
		}

		agent.FilePath = filePath
		agent.IsPriority = isPriority

		// Only override if this is a priority agent or the agent doesn't exist yet
		if existing, exists := agents[agent.Name]; !exists || isPriority || !existing.IsPriority {
			agents[agent.Name] = *agent
		}
	}

	return nil
}

func loadAgentFromFile(filePath string) (*AgentDefinition, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open agent file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent file: %w", err)
	}

	return parseAgentMarkdown(string(content))
}

func parseAgentMarkdown(content string) (*AgentDefinition, error) {
	// Split frontmatter and body
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid agent file format: missing YAML frontmatter")
	}

	// First try to parse with tools as string
	type agentDefString struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Tools       string `yaml:"tools,omitempty"`
		MCPServers  string `yaml:"mcp_servers,omitempty"`
		LSPServers  string `yaml:"lsp_servers,omitempty"`
	}

	var tempAgent agentDefString
	if err := yaml.Unmarshal([]byte(parts[1]), &tempAgent); err == nil && tempAgent.Tools != "" {
		// Successfully parsed with string tools, convert to AgentDefinition
		agent := &AgentDefinition{
			Name:        tempAgent.Name,
			Description: tempAgent.Description,
		}

		// Parse comma-separated tools
		if tempAgent.Tools != "" {
			tools := strings.Split(tempAgent.Tools, ",")
			for _, tool := range tools {
				tool = strings.TrimSpace(tool)
				if tool != "" {
					agent.Tools = append(agent.Tools, tool)
				}
			}
		}

		// Parse comma-separated MCP servers
		if tempAgent.MCPServers != "" {
			servers := strings.Split(tempAgent.MCPServers, ",")
			for _, server := range servers {
				server = strings.TrimSpace(server)
				if server != "" {
					agent.MCPServers = append(agent.MCPServers, server)
				}
			}
		}

		// Parse comma-separated LSP servers
		if tempAgent.LSPServers != "" {
			servers := strings.Split(tempAgent.LSPServers, ",")
			for _, server := range servers {
				server = strings.TrimSpace(server)
				if server != "" {
					agent.LSPServers = append(agent.LSPServers, server)
				}
			}
		}

		// Validate required fields
		if agent.Name == "" {
			return nil, fmt.Errorf("agent name is required")
		}
		if agent.Description == "" {
			return nil, fmt.Errorf("agent description is required")
		}

		// Extract system prompt from markdown body
		agent.SystemPrompt = strings.TrimSpace(parts[2])
		if agent.SystemPrompt == "" {
			return nil, fmt.Errorf("agent system prompt is required")
		}

		return agent, nil
	}

	// Try parsing with array format
	var agent AgentDefinition
	if err := yaml.Unmarshal([]byte(parts[1]), &agent); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Validate required fields
	if agent.Name == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if agent.Description == "" {
		return nil, fmt.Errorf("agent description is required")
	}

	// Extract system prompt from markdown body
	agent.SystemPrompt = strings.TrimSpace(parts[2])
	if agent.SystemPrompt == "" {
		return nil, fmt.Errorf("agent system prompt is required")
	}

	return &agent, nil
}

// ConvertDefinitionToAgent converts an AgentDefinition to an Agent config
func ConvertDefinitionToAgent(def AgentDefinition, modelType SelectedModelType) Agent {
	agent := Agent{
		ID:          def.Name,
		Name:        def.Name,
		Description: def.Description,
		Model:       modelType,
	}

	// If tools are specified, set AllowedTools
	if len(def.Tools) > 0 {
		agent.AllowedTools = def.Tools
	}
	// If tools is empty/nil, all tools are allowed (default behavior)

	// Handle MCP servers
	if len(def.MCPServers) > 0 {
		agent.AllowedMCP = make(map[string][]string)
		for _, server := range def.MCPServers {
			agent.AllowedMCP[server] = nil // nil means all tools from this MCP server
		}
	}
	// If MCPServers is empty/nil, all MCP servers are allowed (default behavior)

	// Handle LSP servers
	if len(def.LSPServers) > 0 {
		agent.AllowedLSP = def.LSPServers
	}
	// If LSPServers is empty/nil, all LSP servers are allowed (default behavior)

	return agent
}
