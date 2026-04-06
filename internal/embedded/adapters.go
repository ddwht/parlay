package embedded

import (
	"embed"
	"fmt"
)

//go:embed adapters/*.adapter.yaml
var adaptersFS embed.FS

// ReadAdapter returns the content of a bundled adapter by name.
func ReadAdapter(name string) ([]byte, error) {
	filename := fmt.Sprintf("adapters/%s.adapter.yaml", name)
	return adaptersFS.ReadFile(filename)
}
