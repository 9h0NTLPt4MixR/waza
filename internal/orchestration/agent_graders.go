package orchestration

import (
	"os"
	"path/filepath"

	"github.com/microsoft/waza/internal/models"
	"github.com/microsoft/waza/internal/skill"
)

// augmentGradersFromAgent injects an implicit tool_constraint grader when the
// target is an .agent.md with a `tools:` frontmatter declaration. Skips
// injection if the eval already defines a tool_constraint grader (user opt-out).
func augmentGradersFromAgent(graders []models.GraderConfig, agentPath string) []models.GraderConfig {
	if agentPath == "" || !skill.IsAgentFile(agentPath) {
		return graders
	}

	// Skip if user already declared a tool_constraint grader
	for _, g := range graders {
		if g.Kind == models.GraderKindToolConstraint {
			return graders
		}
	}

	fm, _, err := skill.LoadAgentDefinition(agentPath)
	if err != nil || fm == nil || len(fm.Tools) == 0 {
		return graders
	}

	expectTools := make([]models.ToolSpecParameters, 0, len(fm.Tools))
	for _, t := range fm.Tools {
		expectTools = append(expectTools, models.ToolSpecParameters{Tool: t})
	}

	implicit := models.GraderConfig{
		Kind:       models.GraderKindToolConstraint,
		Identifier: "agent_tools_implicit",
		Parameters: models.ToolConstraintGraderParameters{
			ExpectTools: expectTools,
		},
	}

	return append(graders, implicit)
}

// resolveAgentPath finds the first .agent.md file in the given skill directories.
// Returns empty string if no agent file is found.
func resolveAgentPath(skillPaths []string) string {
	for _, dir := range skillPaths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && skill.IsAgentFile(entry.Name()) {
				return filepath.Join(dir, entry.Name())
			}
		}
	}
	return ""
}
