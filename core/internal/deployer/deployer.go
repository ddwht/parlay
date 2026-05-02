package deployer

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/embedded"
)

// Deployer packages agent-agnostic skills into agent-specific format.
type Deployer interface {
	// Name returns the agent identifier.
	Name() string

	// Deploy writes skill files and agent config to the project.
	Deploy(projectRoot string, skills []embedded.SkillEntry) error

	// AgentSurfacePaths returns the deployer-owned paths (relative to a
	// parlay root) that must NOT exist inside a child root — they live
	// only at the repo-level root. Each entry is interpreted as a
	// directory or file name relative to the root being checked.
	AgentSurfacePaths() []string
}

var registry = map[string]func() Deployer{}

// Register adds a deployer factory.
func Register(name string, factory func() Deployer) {
	registry[strings.ToLower(name)] = factory
}

// Get returns the deployer for the given agent name.
func Get(name string) (Deployer, error) {
	factory, ok := registry[strings.ToLower(name)]
	if !ok {
		// Fall back to generic
		if f, ok := registry["generic"]; ok {
			return f(), nil
		}
		return nil, fmt.Errorf("no deployer for agent %q", name)
	}
	return factory(), nil
}

func init() {
	Register("claude code", func() Deployer { return &ClaudeDeployer{} })
	Register("generic", func() Deployer { return &GenericDeployer{} })
}

// renderMultiRootSection returns a markdown subsection listing the
// project's registered child roots, or "" if there are no children.
// Shared by every deployer's agent-rules generator (CLAUDE.md, Cursor
// rules file, AGENT_INSTRUCTIONS.md).
//
// The section is rendered INSIDE the parlay markers (callers are
// responsible for placement); deployers concatenate this string into
// their tool-managed block so user-authored content outside the
// markers is preserved.
func renderMultiRootSection(projectRoot string) string {
	idx, err := config.LoadRootsIndex(projectRoot)
	if err != nil || idx == nil || len(idx.Children) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Multi-Root Layout\n\n")
	b.WriteString("This project has registered child roots. Each child has its own\n")
	b.WriteString("intents, dialogs, and build artifacts; schemas, adapters, and the\n")
	b.WriteString("agent surface live at the repo-level root and are shared.\n\n")
	for _, c := range idx.Children {
		desc := ""
		if c.Description != "" {
			desc = " — " + c.Description
		}
		b.WriteString(fmt.Sprintf("- **%s** (`%s`)%s\n", c.Name, c.RelativePath, desc))
	}
	return b.String()
}

// AllAgentSurfacePaths returns the union of every registered deployer's
// AgentSurfacePaths, deduplicated. Used by the cobra entry layer to
// configure the forbidden-directory check without adapter-specific
// knowledge.
func AllAgentSurfacePaths() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, factory := range registry {
		for _, p := range factory().AgentSurfacePaths() {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}
