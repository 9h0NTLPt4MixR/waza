package skill

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentFrontmatter holds parsed agent-specific YAML fields from .agent.md files.
type AgentFrontmatter struct {
	Frontmatter `yaml:",inline"`
	Tools       []string         `yaml:"tools,omitempty"`
	Model       string           `yaml:"model,omitempty"`
	Handoffs    []AgentHandoff   `yaml:"handoffs,omitempty"`
	MCPServers  []AgentMCPServer `yaml:"mcp-servers,omitempty"`
	Agents      []string         `yaml:"agents,omitempty"`
}

// AgentHandoff describes a handoff target for an agent.
type AgentHandoff struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// AgentMCPServer describes an MCP server configuration for an agent.
type AgentMCPServer struct {
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config,omitempty"`
}

// ParseAgentFrontmatter splits an .agent.md file into its YAML frontmatter and
// markdown body. It reuses the existing parseFrontmatter helper to split the
// --- delimited blocks, then unmarshals agent-specific fields.
func ParseAgentFrontmatter(content string) (*AgentFrontmatter, string, error) {
	fm, _, _, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, "", err
	}

	af := &AgentFrontmatter{}
	af.Frontmatter = fm

	// Re-parse the YAML block for agent-specific fields.
	// Extract the YAML block the same way parseFrontmatter does.
	if strings.HasPrefix(content, "---") {
		rest := content[3:]
		if strings.HasPrefix(rest, "\r\n") {
			rest = rest[2:]
		} else if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		}
		if idx := strings.Index(rest, "\n---"); idx >= 0 {
			yamlBlock := rest[:idx]
			if err := yaml.Unmarshal([]byte(yamlBlock), af); err != nil {
				return nil, "", err
			}
		}
	}

	return af, body, nil
}

// LoadAgentDefinition reads an .agent.md file and returns its parsed frontmatter
// and body. Returns nil if path is not an agent file or doesn't exist.
func LoadAgentDefinition(path string) (*AgentFrontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return ParseAgentFrontmatter(string(data))
}

// IsAgentFile returns true if the filename matches the *.agent.md pattern.
func IsAgentFile(filename string) bool {
	base := filepath.Base(filename)
	return strings.HasSuffix(base, ".agent.md")
}
