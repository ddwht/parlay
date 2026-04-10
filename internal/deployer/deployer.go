package deployer

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/internal/embedded"
)

// Deployer packages agent-agnostic skills into agent-specific format.
type Deployer interface {
	// Name returns the agent identifier.
	Name() string

	// Deploy writes skill files and agent config to the project.
	Deploy(projectRoot string, skills []embedded.SkillEntry) error
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
