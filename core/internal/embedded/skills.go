package embedded

import (
	"embed"
	"io/fs"
)

//go:embed skills/*.skill.md
var skillsFS embed.FS

// SkillEntry holds a skill's name and content.
type SkillEntry struct {
	Name    string
	Content []byte
}

// ReadAllSkills returns all embedded skill files.
func ReadAllSkills() ([]SkillEntry, error) {
	entries, err := fs.ReadDir(skillsFS, "skills")
	if err != nil {
		return nil, err
	}

	var skills []SkillEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := skillsFS.ReadFile("skills/" + entry.Name())
		if err != nil {
			return nil, err
		}
		// Strip .skill.md suffix to get the skill name
		name := entry.Name()
		if len(name) > 9 {
			name = name[:len(name)-9] // remove ".skill.md"
		}
		skills = append(skills, SkillEntry{Name: name, Content: data})
	}
	return skills, nil
}
