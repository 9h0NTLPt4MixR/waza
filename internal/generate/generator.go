package generate

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillFrontmatter holds parsed SKILL.md YAML frontmatter fields.
type SkillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// ParseSkillMD reads a SKILL.md file and extracts the YAML frontmatter.
func ParseSkillMD(path string) (fm *SkillFrontmatter, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening skill file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// First line must be "---"
	if !scanner.Scan() {
		return nil, fmt.Errorf("skill file is empty")
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return nil, fmt.Errorf("skill file missing YAML frontmatter delimiter (---)")
	}

	// Collect lines until closing "---"
	var lines []string
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			found = true
			break
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading skill file: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("skill file missing closing frontmatter delimiter (---)")
	}

	raw := strings.Join(lines, "\n")
	var parsed SkillFrontmatter

	// We explicitly *don't* use KnownFields here because we want to allow extra fields in
	// the frontmatter that might be used for other purposes (e.g. by skill registry or
	// other tools) without causing errors in Waza.
	decoder := yaml.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parsing frontmatter YAML: %w", err)
	}

	if parsed.Name == "" {
		return nil, fmt.Errorf("skill frontmatter missing required 'name' field")
	}

	if err := sanitizeSkillName(parsed.Name); err != nil {
		return nil, err
	}

	return &parsed, nil
}

// sanitizeSkillName rejects names that could cause path traversal or are empty.
func sanitizeSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("skill name must not be empty")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return fmt.Errorf("skill name %q contains invalid path characters", name)
	}
	return nil
}
