// parlay-feature: annotations
// parlay-component: annotation-ref-resolution
//
// Every anchor carries a generic identity any Markdown or YAML file can
// supply — a heading path or a YAML path, and a line span. This file adds the
// second identity: the parlay ref, for the file types parlay understands.
//
// The ref matters because of where a resolution goes next. It reuses the
// `affects:` vocabulary (operation | surface | infrastructure | domain), so a
// resolution that becomes an amendment can name its dirty set without
// translation — the thread already says which contract entry it is about.
//
// A file parlay does not know keeps the generic identity and nothing else.
// That is not a degraded case: the generic identity is what makes the scanner
// work on a blueprint, an adapter or a page manifest at all.

package commands

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

// resolveAnnotationRefs fills in Ref, Field and Index on every thread's anchor
// for one file. feature is the qualified feature id ("initiative/feature" or
// "feature"); an empty feature means a project-level file, which has no ref.
func resolveAnnotationRefs(feature, path string, content []byte, threads []parser.AnnotationThread) {
	if feature == "" && !strings.HasSuffix(filepath.Base(path), ".page.md") {
		// A project-level file has no ref to resolve. A page manifest is the
		// exception: its identity is the page, which owes nothing to a feature.
		return
	}
	// The structural view, not the raw one: a commented-out `**Verify**:` or a
	// commented-out bullet must not be counted, or the ref names a different
	// bullet than the reviewer read. Same predicate the scanner uses.
	lines := parser.StructuralLines(path, content)
	base := filepath.Base(path)

	for i := range threads {
		anchor := &threads[i].Anchor
		switch {
		case base == "intents.md":
			resolveIntentRef(feature, lines, anchor)
		case base == "dialogs.md":
			resolveDialogRef(feature, lines, anchor)
		case base == "infrastructure.md":
			resolveInfrastructureRef(feature, lines, anchor)
		case base == "surface.yaml":
			resolveYAMLEntryRef(feature, "surface", "fragments", "name", lines, anchor)
		case base == "capabilities.yaml":
			resolveYAMLEntryRef(feature, "operation", "operations", "id", lines, anchor)
		case base == "domain-model.yaml":
			resolveYAMLEntryRef(feature, "domain", "entities", "name", lines, anchor)
		case strings.HasSuffix(base, ".page.md"):
			resolvePageRef(base, anchor)
		case isAmendmentPath(path):
			resolveAmendmentAnnotationRef(feature, base, anchor)
		}
	}
}

func isAmendmentPath(path string) bool {
	dir := filepath.Base(filepath.Dir(path))
	if dir == "archive" {
		dir = filepath.Base(filepath.Dir(filepath.Dir(path)))
	}
	return dir == "amendments" && parser.AmendmentFileNameValid(filepath.Base(path))
}

// headingAtLevel returns the anchor's enclosing heading at a given level.
// intents.md opens with a level-1 title and names each intent at level 2;
// dialogs.md names each dialog at level 3. Asking for the level rather than a
// position in the stack is what keeps the file's own title out of the ref.
func headingAtLevel(anchor *parser.AnnotationAnchor, level int) string {
	for i := len(anchor.HeadingLevels) - 1; i >= 0; i-- {
		if anchor.HeadingLevels[i] == level && i < len(anchor.HeadingPath) {
			return anchor.HeadingPath[i]
		}
	}
	return ""
}

func resolveIntentRef(feature string, lines []string, anchor *parser.AnnotationAnchor) {
	title := headingAtLevel(anchor, 2)
	if title == "" {
		return
	}
	anchor.Ref = fmt.Sprintf("@%s/intent:%s", feature, parser.Slugify(title))
	anchor.Field, anchor.Index = fieldAndIndexAt(lines, anchor.Span[0]-1)
}

func resolveDialogRef(feature string, lines []string, anchor *parser.AnnotationAnchor) {
	title := headingAtLevel(anchor, 3)
	if title == "" {
		return
	}
	anchor.Ref = fmt.Sprintf("@%s/dialog:%s", feature, parser.Slugify(title))
	if idx := turnIndexAt(lines, anchor.Span[0]-1); idx >= 0 {
		anchor.Field = "turn"
		anchor.Index = &idx
	}
}

func resolveInfrastructureRef(feature string, lines []string, anchor *parser.AnnotationAnchor) {
	title := headingAtLevel(anchor, 2)
	if title == "" {
		return
	}
	anchor.Ref = fmt.Sprintf("@%s/infrastructure:%s", feature, parser.Slugify(title))
	anchor.Field, anchor.Index = fieldAndIndexAt(lines, anchor.Span[0]-1)
}

// resolvePageRef names the page and the region, and deliberately NOT a
// feature-qualified ref. §4.4 gives pages "page name + region / layout node
// id"; the `affects:` vocabulary is operation | surface | infrastructure |
// domain and has no page kind, so `@<feature>/page:<name>` would be shaped
// like an amendment target that no amendment could ever name. A page manifest
// is project-owned, multi-feature, and not ledgered.
func resolvePageRef(base string, anchor *parser.AnnotationAnchor) {
	anchor.Ref = "page:" + strings.TrimSuffix(base, ".page.md")
	if region := headingAtLevel(anchor, 2); region != "" {
		anchor.Field = region
	}
}

func resolveAmendmentAnnotationRef(feature, base string, anchor *parser.AnnotationAnchor) {
	name := strings.TrimSuffix(base, ".md")
	if dash := strings.Index(name, "-"); dash > 0 {
		name = name[dash+1:]
	}
	anchor.Ref = fmt.Sprintf("@%s/amendment:%s", feature, name)
	if section := headingAtLevel(anchor, 2); section != "" {
		anchor.Field = section
	}
}

// resolveYAMLEntryRef names the entry a YAML anchor sits inside: the fragment,
// operation or entity whose collection index the YAML path opens with.
//
// The index is resolved back to a NAME rather than left as a position,
// because a position stops meaning anything the moment the list is reordered,
// and a thread outlives the edit that answers it.
func resolveYAMLEntryRef(feature, kind, collection, nameKey string, lines []string, anchor *parser.AnnotationAnchor) {
	idx, rest, ok := leadingCollectionIndex(anchor.YAMLPath, collection)
	if !ok {
		return
	}
	name := nthEntryName(lines, collection, nameKey, idx)
	if name == "" {
		return
	}
	anchor.Ref = fmt.Sprintf("@%s/%s:%s", feature, kind, name)
	if rest != "" {
		anchor.Field = rest
	}
}

// leadingCollectionIndex reads `collection[N]` off the front of a YAML path
// and returns N and whatever follows it.
func leadingCollectionIndex(path, collection string) (int, string, bool) {
	rest, ok := strings.CutPrefix(path, collection+"[")
	if !ok {
		return 0, "", false
	}
	end := strings.Index(rest, "]")
	if end < 0 {
		return 0, "", false
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0, "", false
	}
	return n, strings.TrimPrefix(rest[end+1:], "."), true
}

// nthEntryName reads the nameKey of the nth item of a top-level collection,
// by the same column walk the scanner uses. Parsing the file with its real
// loader would be stricter, and wrong: an annotation is most useful on a file
// that does not yet validate.
func nthEntryName(lines []string, collection, nameKey string, want int) string {
	inCollection := false
	index := -1
	itemIndent := -1
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if !inCollection {
			if indent == 0 && strings.HasPrefix(trimmed, collection+":") {
				inCollection = true
			}
			continue
		}
		if indent == 0 {
			return ""
		}
		if rest, isItem := strings.CutPrefix(trimmed, "- "); isItem {
			if itemIndent < 0 {
				itemIndent = indent
			}
			if indent != itemIndent {
				continue
			}
			index++
			if index == want {
				if name, ok := keyValue(rest, nameKey); ok {
					return name
				}
			}
			continue
		}
		if index == want && indent == itemIndent+2 {
			if name, ok := keyValue(trimmed, nameKey); ok {
				return name
			}
		}
	}
	return ""
}

func keyValue(s, key string) (string, bool) {
	rest, ok := strings.CutPrefix(s, key+":")
	if !ok {
		return "", false
	}
	value := strings.TrimSpace(rest)
	value = strings.Trim(value, `"'`)
	if value == "" {
		return "", false
	}
	return value, true
}

// fieldAndIndexAt names the `**Field**:` an anchored line sits under, and —
// when the anchored line is itself a bullet — that bullet's position within
// the field. Both are what a resolver needs to say "the second constraint of
// this intent" without counting lines again.
//
// An index is reported only for a bullet. A paragraph or a turn under a field
// has a field but no position in a list, and inventing one would be a number
// that points at a different bullet than the reviewer read.
func fieldAndIndexAt(lines []string, at int) (string, *int) {
	if at < 0 || at >= len(lines) {
		return "", nil
	}
	isBullet := strings.HasPrefix(strings.TrimSpace(lines[at]), "- ")
	bullets := 0
	for i := at; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "#") {
			return "", nil
		}
		if name, ok := markdownFieldName(trimmed); ok {
			if !isBullet || bullets == 0 {
				return name, nil
			}
			idx := bullets - 1
			return name, &idx
		}
		if strings.HasPrefix(trimmed, "- ") {
			bullets++
		}
	}
	return "", nil
}

func markdownFieldName(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "**") {
		return "", false
	}
	end := strings.Index(trimmed[2:], "**")
	if end < 0 || !strings.HasPrefix(trimmed[2+end+2:], ":") {
		return "", false
	}
	name := trimmed[2 : 2+end]
	if name == "" || strings.Contains(name, "*") {
		return "", false
	}
	return name, true
}

// turnIndexAt counts recognised turns from the dialog's heading down to the
// anchored line, so a thread can say which turn it is about.
func turnIndexAt(lines []string, at int) int {
	if at < 0 || at >= len(lines) {
		return -1
	}
	start := 0
	for i := at; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "### ") {
			start = i
			break
		}
	}
	index := -1
	for i := start; i <= at; i++ {
		if parser.IsRecognisedTurn(lines[i]) {
			index++
		}
	}
	return index
}
