package claude

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is a skill of a plugin the user named, to be given to Claude
// (ADR-037).
type Skill struct {
	// Plugin is which of the plugins it is in, counted from 0.
	Plugin int
	// Name is what Claude Code calls it, under the plugin gh-triage makes.
	Name string
	// Dir is its directory in the plugin.
	Dir string
}

// SkillProblem is what keeps a plugin, or a skill of it, from being
// given to Claude.
type SkillProblem struct {
	Plugin  int
	Message string
}

// PluginName is the name of the plugin gh-triage makes of the skills, the
// part before the colon in a skill's name.
const PluginName = "gh-triage"

// PluginSkills are the skills of the plugins in dirs, checked against
// allowed, the tools gh-triage lets Claude use (CheckSkill). Only skills
// are taken; nothing else of a plugin is.
func PluginSkills(dirs []string, allowed []string) ([]Skill, []SkillProblem) {
	var skills []Skill
	var ps []SkillProblem
	seen := map[string]string{}
	for i, dir := range dirs {
		bad := func(format string, args ...any) {
			ps = append(ps, SkillProblem{Plugin: i, Message: fmt.Sprintf(format, args...)})
		}
		if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "plugin.json")); err != nil {
			bad("%s is not a plugin: it has no .claude-plugin/plugin.json", dir)
			continue
		}
		es, err := os.ReadDir(filepath.Join(dir, "skills"))
		if errors.Is(err, fs.ErrNotExist) {
			bad("%s has no skills", dir)
			continue
		}
		if err != nil {
			bad("%v", err)
			continue
		}
		for _, e := range es {
			sd := filepath.Join(dir, "skills", e.Name())
			if e.Type()&fs.ModeSymlink != 0 {
				bad("%s is a symlink; a skill is copied as it is, and a link may point outside its plugin", sd)
				continue
			}
			if !e.IsDir() {
				continue
			}
			name, err := CheckSkill(sd, allowed)
			if err != nil {
				bad("%s: %v", sd, err)
				continue
			}
			for _, n := range []string{e.Name(), name} {
				if other, ok := seen[n]; ok && other != sd {
					bad("%s and %s are both a skill named %s", other, sd, n)
				}
				seen[n] = sd
			}
			skills = append(skills, Skill{Plugin: i, Name: PluginName + ":" + name, Dir: sd})
		}
	}
	return skills, ps
}

// CheckSkill reads the skill in dir and returns its name. A skill is
// refused when anything in it is a symlink, when its SKILL.md has no
// frontmatter that reads, or when its allowed-tools grants a tool outside
// allowed: Claude Code lets such a tool run without a prompt, beyond the
// tools gh-triage allows.
func CheckSkill(dir string, allowed []string) (string, error) {
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink, which may point outside the plugin", p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return "", errors.New("it has no SKILL.md")
	}
	front, ok := frontmatter(b)
	if !ok {
		return "", errors.New("its SKILL.md has no frontmatter")
	}
	var meta map[string]any
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return "", fmt.Errorf("its frontmatter does not read: %v", err)
	}
	for k, v := range meta {
		if !strings.EqualFold(strings.ReplaceAll(k, "_", "-"), "allowed-tools") {
			continue
		}
		tools, err := toolList(v)
		if err != nil {
			return "", fmt.Errorf("%s: %v", k, err)
		}
		for _, t := range tools {
			if !slices.Contains(allowed, t) {
				return "", fmt.Errorf("%s grants %q, which gh-triage does not allow (it may grant only %s)", k, t, strings.Join(allowed, ", "))
			}
		}
	}
	name := filepath.Base(dir)
	if n, ok := meta["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = strings.TrimSpace(n)
	}
	return name, nil
}

// frontmatter is the YAML between the --- lines that open b.
func frontmatter(b []byte) ([]byte, bool) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		rest, ok = bytes.CutPrefix(b, []byte("---\r\n"))
	}
	if !ok {
		return nil, false
	}
	for i := 0; i < len(rest); {
		j := bytes.IndexByte(rest[i:], '\n')
		line := rest[i:]
		if j >= 0 {
			line = rest[i : i+j]
		}
		if string(bytes.TrimRight(line, "\r")) == "---" {
			return rest[:i], true
		}
		if j < 0 {
			break
		}
		i += j + 1
	}
	return nil, false
}

// toolList is allowed-tools as its tools: a list, or a string of them
// apart by commas or spaces. Anything else is refused rather than guessed
// at.
func toolList(v any) ([]string, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		return splitTools(x), nil
	case []any:
		var out []string
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("%v is not a tool", e)
			}
			out = append(out, splitTools(s)...)
		}
		return out, nil
	}
	return nil, fmt.Errorf("is %T, not a list of tools", v)
}

// splitTools splits s at commas and spaces, but not inside the
// parentheses of a rule such as Bash(git status:*), which is one tool.
func splitTools(s string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case depth == 0 && (r == ',' || r == ' ' || r == '\t' || r == '\n'):
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

// WritePlugin makes the plugin gh-triage gives Claude in dir, a copy of
// skills, and checks the copy again, so that what was checked is what is
// given. It returns the plugin's directory.
func WritePlugin(dir string, skills []Skill, allowed []string) (string, error) {
	root := filepath.Join(dir, "plugin")
	if err := os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o700); err != nil {
		return "", err
	}
	manifest := fmt.Sprintf(`{"name":%q,"description":"The skills gh-triage gives Claude, copied from the plugins in claude.plugins"}`, PluginName)
	if err := os.WriteFile(filepath.Join(root, ".claude-plugin", "plugin.json"), []byte(manifest), 0o600); err != nil {
		return "", err
	}
	for _, s := range skills {
		to := filepath.Join(root, "skills", filepath.Base(s.Dir))
		if err := copyDir(s.Dir, to); err != nil {
			return "", err
		}
		if _, err := CheckSkill(to, allowed); err != nil {
			return "", fmt.Errorf("%s changed while it was copied: %v", s.Dir, err)
		}
	}
	return root, nil
}

// copyDir copies the regular files and directories under from to to; a
// symlink or any other kind of file stops it.
func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(dst, 0o700)
		case d.Type().IsRegular():
			return copyFile(p, dst)
		}
		return fmt.Errorf("%s is not a regular file", p)
	})
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
