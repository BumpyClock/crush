# Sub-Agents Documentation

## Overview

Crush now supports full sub-agent capabilities, allowing the main agent to delegate complex tasks to specialized sub-agents while preserving context. This implementation follows the Anthropic Claude Code sub-agent pattern.

## Key Features

- **Full Tool Access**: Sub-agents have access to ALL tools including Bash, Edit, MultiEdit, Write, and more
- **Custom Agent Definitions**: Create specialized agents with custom system prompts
- **Two-Level Configuration**: Support for both project-level and user-level agents
- **Context Preservation**: Each sub-agent operates in its own context window
- **Flexible Tool Permissions**: Configure specific tools for each agent

## Creating Custom Agents

### File Locations

| Type | Location | Scope | Priority |
|------|----------|-------|----------|
| **Project agents** | `.crush/agents/` | Available in current project | Highest |
| **User agents** | `~/.crush/agents/` | Available across all projects | Lower |

When agent names conflict, project-level agents take precedence.

### File Format

Each agent is defined in a Markdown file with YAML frontmatter:

```markdown
---
name: your-agent-name
description: Description of when this agent should be invoked
tools: tool1, tool2, tool3  # Optional - inherits all tools if omitted
---

Your agent's system prompt goes here. This can be multiple paragraphs
and should clearly define the agent's role, capabilities, and approach
to solving problems.
```

#### Configuration Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique identifier using lowercase letters and hyphens |
| `description` | Yes | Natural language description of the agent's purpose |
| `tools` | No | Comma-separated list of specific tools. If omitted, inherits all tools |

## Using Sub-Agents

The main Crush agent can now delegate tasks to sub-agents:

```
# Automatic delegation based on task
> Review the code changes I just made
[Crush will automatically use the code-reviewer agent]

# Explicit agent invocation
> Use the debugger agent to fix the failing tests

# Multiple agents in parallel
> Have the test-runner agent run tests while the code-reviewer checks my changes
```

## Built-in Agents

### Coder Agent
- **Purpose**: Main agent for executing coding tasks
- **Tools**: All tools available
- **Can delegate to**: All other agents

### Task Agent  
- **Purpose**: Searching for context and finding implementation details
- **Tools**: All tools available (updated from limited set)
- **Best for**: Parallel searches and exploration tasks

## Example Custom Agents

### Code Reviewer
```yaml
name: code-reviewer
description: Expert code review specialist
tools: view, grep, glob, bash
```

Reviews code for:
- Code quality and readability
- Security issues
- Performance considerations
- Test coverage

### Debugger
```yaml
name: debugger
description: Debugging specialist for errors and test failures
tools: view, edit, bash, grep, glob
```

Specializes in:
- Root cause analysis
- Error diagnosis
- Test failure fixes
- Performance debugging

### Test Runner
```yaml
name: test-runner
description: Test automation expert
tools: bash, view, edit, grep
```

Handles:
- Running appropriate tests
- Fixing test failures
- Maintaining test coverage
- Test suite health

## Best Practices

1. **Design Focused Agents**: Create agents with single, clear responsibilities
2. **Write Detailed Prompts**: Include specific instructions and examples
3. **Limit Tool Access**: Only grant necessary tools for the agent's purpose
4. **Version Control**: Check project agents into version control
5. **Use Proactive Descriptions**: Include "proactively" or "immediately" in descriptions

## UI Display Improvements

### Agent Names in Session Display

When a sub-agent is running, the UI now shows the agent's descriptive name instead of a generic "Agent" label:

- **Before**: `Agent (code-reviewer) Session`
- **After**: `Code Reviewer` (or the agent's configured name)

This makes it clear which specialized agent is performing the task.

### Activity Indicators

The green dot (●) indicator next to the agent name continues to animate while the agent is processing, providing visual feedback that work is in progress.

### Custom Agent Names

Custom agents can specify their display name in the configuration:

```yaml
name: code-reviewer  # Agent ID
description: Expert code review specialist  # Used for agent selection
```

The agent's ID (`name` field) is used as the display name in the UI. If no custom configuration exists, the system formats the agent ID into a readable title (e.g., `test-runner` becomes "Test Runner").

## Implementation Details

### Agent Loading
- Agents are loaded from markdown files at startup
- Project agents override user agents with the same name
- Invalid agents are skipped with warnings

### Tool Access
- If `tools` field is omitted, agent inherits all tools
- Specified tools limit the agent to only those tools
- MCP tools are included when available

### Context Management
- Each sub-agent gets its own session
- Parent session tracks costs from sub-agents
- Sub-agents cannot communicate with each other

## Performance Considerations

- **Context Efficiency**: Sub-agents help preserve main context
- **Parallel Execution**: Multiple agents can run concurrently
- **Latency**: Sub-agents start fresh, may need to gather context

## Migration from Old System

The previous task agent with limited tools (glob, grep, ls, sourcegraph, view) has been updated to have access to all tools. This enables:
- File modification capabilities
- Command execution
- Full problem-solving abilities

Existing code using the agent tool will continue to work but now has enhanced capabilities.

## Troubleshooting

### Custom Agents Not Working

If custom agents don't seem to be available:

1. **Check file location**: Ensure agent files are in `.crush/agents/` (project) or `~/.crush/agents/` (user)
2. **Verify file format**: Files must be `.md` with valid YAML frontmatter
3. **Check logs**: Run with `--debug` flag to see agent loading logs
4. **Validate YAML**: Ensure frontmatter has required fields (name, description)
5. **Tool names**: Tool names are case-sensitive (use lowercase)

### Available Agent Names

When the agent tool reports "agent not found", it will list available agents. Common ones include:
- `task`: Default agent for general tasks
- `coder`: Main coding agent (only available as top-level, not as sub-agent)
- Your custom agents by name (e.g., `code-reviewer`, `debugger`, `test-runner`)

### Invoking Specific Agents

The main agent can explicitly use a specific sub-agent by name:
```
# Main agent will use the agent tool with agent_name parameter
Use the code-reviewer agent to check my changes
Have the debugger agent fix the test failures
Ask the test-runner agent to run all tests
```

Internally, this translates to:
```json
{
  "prompt": "Your task here",
  "agent_name": "code-reviewer"  
}
```

If `agent_name` is not specified, defaults to "task" agent.

## Technical Implementation

### Agent Loading Flow

1. **Config Initialization**: `config.SetupAgents()` loads built-in and custom agents
2. **Agent Creation**: When creating the coder agent, all other agents become sub-agents
3. **Tool Registration**: Sub-agents are passed to `NewAgentTool()` as a map
4. **Runtime Invocation**: Agent tool looks up requested agent by name

### Debugging Agent Loading

Enable debug mode to see agent loading:
```bash
crush --debug
```

This will show:
- Which agents are loaded from files
- Which sub-agents are created
- Available agents when the tool is invoked

Example log output:
```
INFO Loading custom agents count=3
INFO Loading custom agent name=code-reviewer description="Expert code review specialist"
INFO Creating sub-agents for coder total_agents=5
INFO Successfully created sub-agent name=task
INFO Successfully created sub-agent name=code-reviewer
INFO Agent tool invoked requested_agent=code-reviewer available_agents=[task,code-reviewer,debugger]
```

### Common Issues and Solutions

| Issue | Solution |
|-------|----------|
| Agent not found | Check agent name matches exactly (case-sensitive) |
| Custom agent not loaded | Verify YAML frontmatter syntax |
| Tools not working | Ensure tool names are lowercase and valid |
| Agent has no access | Check AllowedTools in agent config |