package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RootsIndexFile is the on-disk filename for the parent's roots index,
// stored at <parent>/.parlay/roots.yaml.
const RootsIndexFile = "roots.yaml"

// rootsIndexYAML is the on-disk shape of roots.yaml.
type rootsIndexYAML struct {
	Children []childEntryYAML `yaml:"children"`
}

type childEntryYAML struct {
	Name         string `yaml:"name"`
	RelativePath string `yaml:"relative-path"`
	Description  string `yaml:"description,omitempty"`
}

// childConfigYAML is the on-disk shape of a child root's
// .parlay/config.yaml additions for parent linkage. It coexists with
// the existing ProjectConfig fields on the same file.
type childConfigYAML struct {
	// Parent is a relative path from the child root to the parent root.
	Parent string `yaml:"parent,omitempty"`
	// All other fields are tolerated (preserved on round-trip via the
	// existing ProjectConfig path).
}

// rootsIndexPath returns the absolute path to the parent's roots.yaml.
func rootsIndexPath(parentRootPath string) string {
	return filepath.Join(parentRootPath, ParlayDir, RootsIndexFile)
}

// LoadRootsIndex reads <parent>/.parlay/roots.yaml. Returns a non-nil
// index with an empty Children slice when the file does not exist —
// that's the canonical "parent without children" state. Returns an
// error on read or parse failure.
func LoadRootsIndex(parentRootPath string) (*RootsIndex, error) {
	path := rootsIndexPath(parentRootPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &RootsIndex{ParentPath: parentRootPath}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc rootsIndexYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	idx := &RootsIndex{ParentPath: parentRootPath}
	for _, e := range doc.Children {
		idx.Children = append(idx.Children, Root{
			Name:         e.Name,
			RelativePath: e.RelativePath,
			Path:         filepath.Join(parentRootPath, e.RelativePath),
			ParentPath:   parentRootPath,
			Kind:         RootKindChild,
			Description:  e.Description,
		})
	}
	return idx, nil
}

// SaveRootsIndex writes the index to <parent>/.parlay/roots.yaml. The
// caller is responsible for creating the parent's .parlay/ directory.
func SaveRootsIndex(idx *RootsIndex) error {
	if idx == nil {
		return fmt.Errorf("nil index")
	}
	doc := rootsIndexYAML{}
	for _, c := range idx.Children {
		doc.Children = append(doc.Children, childEntryYAML{
			Name:         c.Name,
			RelativePath: c.RelativePath,
			Description:  c.Description,
		})
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(rootsIndexPath(idx.ParentPath), data, 0644)
}

// AppendRootToIndex adds a new child to the index, persists, and returns
// the updated index. Refuses when a child with the same name or path
// already exists.
func AppendRootToIndex(idx *RootsIndex, child Root) (*RootsIndex, error) {
	if idx == nil {
		return nil, fmt.Errorf("nil index")
	}
	for _, existing := range idx.Children {
		if existing.Name == child.Name {
			return nil, fmt.Errorf("child root %q already registered", child.Name)
		}
		if existing.RelativePath == child.RelativePath {
			return nil, fmt.Errorf("path %q already registered as %q", child.RelativePath, existing.Name)
		}
	}
	idx.Children = append(idx.Children, child)
	return idx, SaveRootsIndex(idx)
}

// readParentPointer reads the parent: field from <root>/.parlay/config.yaml.
// Returns "" (not an error) when the field is absent, when the file does
// not exist, or when the file does not parse — making the caller treat
// any of these as "no parent pointer".
func readParentPointer(rootPath string) (string, error) {
	cfgPath := filepath.Join(rootPath, ParlayDir, ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", err
	}
	var doc childConfigYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	if doc.Parent == "" {
		return "", nil
	}
	if filepath.IsAbs(doc.Parent) {
		return doc.Parent, nil
	}
	return filepath.Clean(filepath.Join(rootPath, doc.Parent)), nil
}

// WriteParentPointer adds or replaces the parent: field in the child
// root's .parlay/config.yaml. The parent path is stored relative to the
// child root so that moving the whole tree on disk preserves the link.
func WriteParentPointer(childRootPath, parentRootPath string) error {
	cfgPath := filepath.Join(childRootPath, ParlayDir, ConfigFile)
	rel, err := filepath.Rel(childRootPath, parentRootPath)
	if err != nil {
		return err
	}
	// Read existing config (if any), preserving other fields.
	raw := map[string]any{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(data, &raw)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", cfgPath, err)
	}
	raw["parent"] = rel
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

// RemoveParentPointer strips the parent: field from the child root's
// .parlay/config.yaml. Other fields are preserved. Used by promote-root.
func RemoveParentPointer(rootPath string) error {
	cfgPath := filepath.Join(rootPath, ParlayDir, ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", cfgPath, err)
	}
	raw := map[string]any{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	delete(raw, "parent")
	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0644)
}
