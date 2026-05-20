package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/microsoft/waza/internal/projectconfig"
	"github.com/microsoft/waza/internal/utils"
)

// DiscoveredSkill represents a skill found during directory traversal.
type DiscoveredSkill struct {
	Name      string // directory name containing SKILL.md
	SkillPath string // absolute path to SKILL.md
	EvalPath  string // absolute path to the eval file (empty if not found)
	Dir       string // absolute path to the skill directory
}

// HasEval returns true if the skill has a discovered eval config.
func (d DiscoveredSkill) HasEval() bool {
	return d.EvalPath != ""
}

// Discover walks the given root directory and finds all skills with eval configs.
// A skill is a directory containing SKILL.md. An eval config is an eval file either
// in the same directory, in an evals/ subdirectory, in a tests/ subdirectory, or
// in a project-layout evals/{name}/ directory two levels above the skill directory
// (e.g. project-root/skills/{name}/SKILL.md → project-root/evals/{name}/{evalFile}).
func Discover(root string) ([]DiscoveredSkill, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving root path: %w", err)
	}

	// Verify root exists before walking
	if _, err := os.Stat(absRoot); err != nil {
		return nil, fmt.Errorf("root path: %w", err)
	}

	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving root symlink: %w", err)
	}

	var skills []DiscoveredSkill
	seenNames := make(map[string]struct{})
	rootGitHubDir := filepath.Join(resolvedRoot, ".github")
	rootGitHubSkillsDir := filepath.Join(rootGitHubDir, "skills")
	evalFile := projectconfig.DefaultEvalFile
	if cfg, cfgErr := projectconfig.Load(resolvedRoot); cfgErr == nil {
		evalFile = cfg.Files.EvalFile
	}

	err = filepath.Walk(resolvedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		// Skip hidden directories, except root-level .github
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			if path != rootGitHubDir {
				return filepath.SkipDir
			}
		}

		// Under root .github, only recurse into .github/skills.
		if info.IsDir() &&
			strings.HasPrefix(path, rootGitHubDir+string(filepath.Separator)) &&
			path != rootGitHubSkillsDir &&
			!strings.HasPrefix(path, rootGitHubSkillsDir+string(filepath.Separator)) {
			return filepath.SkipDir
		}

		// Skip node_modules and similar
		if info.IsDir() && (info.Name() == "node_modules" || info.Name() == "vendor") {
			return filepath.SkipDir
		}

		// Look for SKILL.md files
		if !info.IsDir() && info.Name() == "SKILL.md" {
			dir := filepath.Dir(path)
			name := filepath.Base(dir)
			if _, exists := seenNames[name]; exists {
				return nil
			}

			evalPath := findEvalConfig(dir, name, evalFile)
			skills = append(skills, DiscoveredSkill{
				Name:      name,
				SkillPath: path,
				EvalPath:  evalPath,
				Dir:       dir,
			})
			seenNames[name] = struct{}{}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory %s: %w", resolvedRoot, err)
	}

	return skills, nil
}

// findEvalConfig looks for an eval file in standard locations relative to a skill directory.
// Priority: tests/{evalFile} > evals/{evalFile} > {evalFile} > ../../evals/{name}/{evalFile}
// The last candidate handles the project layout produced by `waza new` in project mode,
// where SKILL.md lives at skills/{name}/SKILL.md and eval file at evals/{name}/{evalFile}.
func findEvalConfig(skillDir, name, evalFile string) string {
	var candidates []string
	for _, filename := range evalFilenames(evalFile) {
		candidates = append(candidates,
			filepath.Join(skillDir, "tests", filename),
			filepath.Join(skillDir, "evals", filename),
			filepath.Join(skillDir, filename),
			// Project layout: project-root/skills/{name}/SKILL.md → project-root/evals/{name}/{evalFile}
			filepath.Join(filepath.Dir(filepath.Dir(skillDir)), "evals", name, filename),
		)
	}

	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func evalFilenames(configured string) []string {
	if configured == "" || configured == projectconfig.DefaultEvalFile {
		return []string{projectconfig.DefaultEvalFile}
	}
	return []string{configured, projectconfig.DefaultEvalFile}
}

// FilterWithEval returns only skills that have a discovered eval config.
func FilterWithEval(skills []DiscoveredSkill) []DiscoveredSkill {
	var result []DiscoveredSkill
	for _, s := range skills {
		if s.HasEval() {
			result = append(result, s)
		}
	}
	return result
}

// FilterWithoutEval returns only skills that lack an eval config.
func FilterWithoutEval(skills []DiscoveredSkill) []DiscoveredSkill {
	var result []DiscoveredSkill
	for _, s := range skills {
		if !s.HasEval() {
			result = append(result, s)
		}
	}
	return result
}

// fileExists checks if a path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func mergeSkillsByName(base, additional []DiscoveredSkill) []DiscoveredSkill {
	return utils.MergeByKey(base, additional, func(skill DiscoveredSkill) string {
		return skill.Name
	})
}
