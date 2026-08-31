// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/archive-invisibility-and-integrity
//
// `parlay retired-roots [--check]` — exclusion from discovery is not
// exclusion from verification.
//
// Preserved roots live under <parent>/.parlay/retired/, inside the
// .parlay dot-directory that every live-work walk already skips:
// config.DiscoverRootsBelow skips dot-directories, and feature /
// build-state enumeration is anchored on registered roots, which a
// retired root no longer is. Invisibility is structural; this command is
// the deliberate way back in.
//
// --check reads each archive's manifest back and reports members whose
// content changed, members that are missing, and members present in the
// archive but absent from the manifest. Verification compares bytes
// against recorded hashes and never raises a finding on the grounds that
// preserved build state is empty or thin: a check that read thinness as
// damage would fire on the bulk of what a typical retired root preserves
// and stop being read.

package commands

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var retiredRootsCmd = &cobra.Command{
	Use:   "retired-roots",
	Short: "List retired roots, and verify their archives with --check",
	Args:  cobra.NoArgs,
	RunE:  runRetiredRoots,
}

var retiredRootsCheck bool

func init() {
	retiredRootsCmd.Flags().BoolVar(&retiredRootsCheck, "check", false,
		"Read each archive's manifest back and report changed, missing, and unlisted members")
}

// IntegrityReport is the result of verifying one archive against its
// manifest: members whose bytes changed, members the manifest names
// that are gone, and members present under contents/ that the manifest
// never listed.
type IntegrityReport struct {
	Changed  []string `json:"changed"`
	Missing  []string `json:"missing"`
	Unlisted []string `json:"unlisted"`
}

// Clean reports whether the archive matches its manifest exactly.
func (r IntegrityReport) Clean() bool {
	return len(r.Changed) == 0 && len(r.Missing) == 0 && len(r.Unlisted) == 0
}

// verifyArchiveIntegrity checks one retired root's archive against its
// manifest. Bytes against recorded hashes only: an empty or placeholder
// member that matches its hash is history, not corruption.
func verifyArchiveIntegrity(destination string) (IntegrityReport, error) {
	report := IntegrityReport{}
	manifest, err := ReadManifest(filepath.Join(destination, "manifest.yaml"))
	if err != nil {
		return report, err
	}
	contentsDir := filepath.Join(destination, "contents")

	listed := map[string]string{}
	for _, m := range manifest.Members {
		listed[m.Path] = m.SHA256
	}

	// Walk what is actually there, so unlisted members surface.
	onDisk := map[string]bool{}
	walkErr := filepath.WalkDir(contentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(contentsDir, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		onDisk[relSlash] = true
		want, ok := listed[relSlash]
		if !ok {
			report.Unlisted = append(report.Unlisted, relSlash)
			return nil
		}
		var got string
		if d.Type()&fs.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return fmt.Errorf("read preserved symlink %s: %w", relSlash, linkErr)
			}
			got = hashBytes([]byte(target))
		} else {
			var hashErr error
			got, hashErr = archiveHashFile(path)
			if hashErr != nil {
				return fmt.Errorf("read preserved member %s: %w", relSlash, hashErr)
			}
		}
		if got != want {
			report.Changed = append(report.Changed, relSlash)
		}
		return nil
	})
	if walkErr != nil {
		return report, walkErr
	}

	for path := range listed {
		if !onDisk[path] {
			report.Missing = append(report.Missing, path)
		}
	}
	sort.Strings(report.Changed)
	sort.Strings(report.Missing)
	sort.Strings(report.Unlisted)
	return report, nil
}

// listRetiredRoots returns the names of the roots archived under
// <parent>/.parlay/retired/ — directories only; journals and staging
// leftovers are not archives.
func listRetiredRoots(parentPath string) ([]string, error) {
	entries, err := os.ReadDir(retiredRootsDir(parentPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read retired-roots directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func runRetiredRoots(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	parentPath := cfg.RepoRoot()
	out := cmd.OutOrStdout()

	names, err := listRetiredRoots(parentPath)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintln(out, "No retired roots.")
		return nil
	}

	failed := false
	for _, name := range names {
		dest := retirementDestination(parentPath, name)
		if !retiredRootsCheck {
			fmt.Fprintf(out, "  - %s (%s)\n", name, dest)
			continue
		}
		report, err := verifyArchiveIntegrity(dest)
		if err != nil {
			fmt.Fprintf(out, "  - %s: [ERR] %v\n", name, err)
			failed = true
			continue
		}
		if report.Clean() {
			fmt.Fprintf(out, "  - %s: [OK] archive matches its manifest\n", name)
			continue
		}
		failed = true
		fmt.Fprintf(out, "  - %s: [ERR] archive does not match its manifest\n", name)
		for _, p := range report.Changed {
			fmt.Fprintf(out, "      changed:  %s\n", p)
		}
		for _, p := range report.Missing {
			fmt.Fprintf(out, "      missing:  %s\n", p)
		}
		for _, p := range report.Unlisted {
			fmt.Fprintf(out, "      unlisted: %s\n", p)
		}
	}
	if failed {
		return fmt.Errorf("retired-roots --check found archives that do not match their manifests")
	}
	return nil
}
