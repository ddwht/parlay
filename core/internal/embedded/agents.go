// parlay-feature: parlay-tool/parlay-loop
// parlay-section: cross-cutting
package embedded

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed agents/*.agent.md
var agentsFS embed.FS

// AgentEntry holds a subagent definition's name and raw content.
// Mirrors SkillEntry so deployers can iterate both bundles symmetrically.
type AgentEntry struct {
	Name    string
	Content []byte
}

// ReadAllAgents returns all embedded subagent definition files.
// Each entry's Name is the file's slug with the ".agent.md" suffix stripped.
func ReadAllAgents() ([]AgentEntry, error) {
	entries, err := fs.ReadDir(agentsFS, "agents")
	if err != nil {
		return nil, err
	}

	var agents []AgentEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := agentsFS.ReadFile("agents/" + entry.Name())
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(entry.Name(), ".agent.md")
		agents = append(agents, AgentEntry{Name: name, Content: data})
	}
	return agents, nil
}
