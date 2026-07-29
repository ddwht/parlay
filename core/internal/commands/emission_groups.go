package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// emissionGroupsCmd computes which units of work may be emitted concurrently.
//
// The plan for this stage said "features are independent until the
// cross-cutting merge". They are not. Two constraints in generate-code make
// features genuinely order-dependent:
//
//   - Sibling-create satisfies modify: feature B's plan.modifies may name a
//     path that only exists because feature A's plan.creates makes it. A must
//     emit first. The skill already sorts features topologically for exactly
//     this reason.
//   - Project-scoped files are shared. The merged model layer, the route
//     table and the app shell are written from every feature's sections. Two
//     agents merging into app.routes.ts concurrently do not race visibly —
//     one write simply wins, and the losing feature's routes are missing from
//     a file that still parses.
//
// So "run features in parallel" would corrupt a shared file some fraction of
// the time and produce a prototype that builds. That is the worst available
// failure mode, and it is why this is a computed answer rather than an
// instruction in a skill.
//
// The unit is a component or cross-cutting entry, and its footprint is the
// plan rows citing it — which Stage 3.2's derivation now makes trustworthy.
// Units conflict when they write the same path; they are ordered when one
// creates what another modifies. Everything else can run at once.
var emissionGroupsCmd = &cobra.Command{
	Use:   "emission-groups",
	Short: "Compute conflict-free waves of buildfile units that may be emitted concurrently (JSON output)",
	Args:  cobra.NoArgs,
	RunE:  runEmissionGroups,
}

// emissionUnit is one schedulable piece of work: one file, and the buildfile
// entries jointly responsible for its content.
//
// The unit is the *path*, not the source entry. My first version keyed on
// source, which made a plan row citing two entries into two units both
// claiming the same path — so they conflicted, and ordinary component files
// showed up as contended. They were never contended: one row is one write, and
// its several sources are co-authors of that single write, not competitors for
// it. Keying on the path makes a conflict mean what it says.
type emissionUnit struct {
	ID       string   `json:"id"` // the path, which is the unit's identity
	Path     string   `json:"path"`
	Feature  string   `json:"feature"`
	Creates  bool     `json:"creates"`
	Sources  []string `json:"sources,omitempty"`
	Features []string `json:"features,omitempty"` // >1 when features co-write a shared file
}

// writes returns the single path the unit touches.
func (u emissionUnit) writes() []string { return []string{u.Path} }

type emissionWave struct {
	Wave  int            `json:"wave"`
	Units []emissionUnit `json:"units"`
}

type emissionGroupsOutput struct {
	Waves       []emissionWave `json:"waves"`
	UnitCount   int            `json:"unit_count"`
	MaxWidth    int            `json:"max_concurrency"`
	SerialWaves int            `json:"wave_count"`
	SharedPaths []string       `json:"shared_paths,omitempty"`
	Cycles      []string       `json:"cycles,omitempty"`

	// Safe reports whether the schedule may actually be used to run work
	// concurrently. A schedule computed from incomplete plans is worse than
	// no schedule: it reads as an authorization.
	Safe   bool     `json:"safe_to_parallelize"`
	Unsafe []string `json:"unsafe_because,omitempty"`
}

type planBuildfile struct {
	Feature string `yaml:"feature"`
	Plan    struct {
		Creates  []planEntry `yaml:"creates"`
		Modifies []planEntry `yaml:"modifies"`
	} `yaml:"plan"`
}

// collectEmissionUnits reads every feature's plan and attributes each row to
// the unit that cited it.
func collectEmissionUnits(buildDir string) ([]emissionUnit, error) {
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return nil, err
	}
	byPath := map[string]*emissionUnit{}
	var order []string

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(buildDir, e.Name(), "buildfile.yaml"))
		if err != nil {
			continue
		}
		var bf planBuildfile
		if err := yaml.Unmarshal(data, &bf); err != nil {
			continue
		}
		feature := bf.Feature
		if feature == "" {
			feature = e.Name()
		}

		absorb := func(rows []planEntry, create bool) {
			for _, row := range rows {
				u, ok := byPath[row.Path]
				if !ok {
					u = &emissionUnit{ID: row.Path, Path: row.Path, Feature: feature, Creates: create}
					byPath[row.Path] = u
					order = append(order, row.Path)
				}
				// A path any feature creates is a create: the create has to
				// happen before any modify of it, whichever side declared it.
				u.Creates = u.Creates || create
				u.Sources = appendUnique(u.Sources, row.Sources...)
				u.Features = appendUnique(u.Features, feature)
			}
		}
		absorb(bf.Plan.Creates, true)
		absorb(bf.Plan.Modifies, false)
	}

	sort.Strings(order)
	units := make([]emissionUnit, 0, len(order))
	for _, path := range order {
		u := byPath[path]
		sort.Strings(u.Sources)
		sort.Strings(u.Features)
		units = append(units, *u)
	}
	return units, nil
}

func appendUnique(dst []string, vals ...string) []string {
	for _, v := range vals {
		found := false
		for _, existing := range dst {
			if existing == v {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}

// scheduleEmissionWaves partitions units into ordered waves such that within
// a wave no two units write the same path, and every create precedes the
// modifies it satisfies.
//
// Deterministic by construction: units are considered in sorted id order, so
// the same plans always produce the same schedule. A schedule that varied
// between runs would make a parallel emission unreproducible, which is a
// worse property than being slow.
func scheduleEmissionWaves(units []emissionUnit) ([]emissionWave, []string, []string) {
	// With one unit per path there is no write conflict left to resolve —
	// the schedule's remaining constraint is ordering. A file several
	// features co-write is a join point: every feature contributes to it, so
	// it cannot be emitted until the per-feature work feeding it is done.
	var shared []string
	sharedSet := map[string]bool{}
	for _, u := range units {
		if len(u.Features) > 1 {
			shared = append(shared, u.Path)
			sharedSet[u.Path] = true
		}
	}
	sort.Strings(shared)

	// Every shared (join) unit depends on every unshared unit of the features
	// that co-write it. That is the cross-cutting merge barrier the plan
	// described, derived rather than asserted.
	deps := map[string]map[string]bool{}
	for _, u := range units {
		deps[u.ID] = map[string]bool{}
	}
	for _, j := range units {
		if !sharedSet[j.Path] {
			continue
		}
		contributors := map[string]bool{}
		for _, f := range j.Features {
			contributors[f] = true
		}
		for _, u := range units {
			if sharedSet[u.Path] || !contributors[u.Feature] {
				continue
			}
			deps[j.ID][u.ID] = true
		}
	}

	remaining := map[string]emissionUnit{}
	for _, u := range units {
		remaining[u.ID] = u
	}
	done := map[string]bool{}

	var waves []emissionWave
	for len(remaining) > 0 {
		var ids []string
		for id := range remaining {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		claimed := map[string]bool{} // paths taken by this wave
		var wave []emissionUnit
		for _, id := range ids {
			u := remaining[id]

			ready := true
			for d := range deps[id] {
				if !done[d] {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			conflicts := false
			for _, p := range u.writes() {
				if claimed[p] {
					conflicts = true
					break
				}
			}
			if conflicts {
				continue
			}
			for _, p := range u.writes() {
				claimed[p] = true
			}
			wave = append(wave, u)
		}

		if len(wave) == 0 {
			// Nothing schedulable and work remains: a dependency cycle. Name
			// the participants rather than looping — a scheduler that spins
			// on a cycle is indistinguishable from a slow one.
			var stuck []string
			for id := range remaining {
				stuck = append(stuck, id)
			}
			sort.Strings(stuck)
			return waves, shared, stuck
		}

		for _, u := range wave {
			done[u.ID] = true
			delete(remaining, u.ID)
		}
		waves = append(waves, emissionWave{Wave: len(waves), Units: wave})
	}
	return waves, shared, nil
}

func runEmissionGroups(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	buildDir := filepath.Dir(cfg.BuildPath("_probe"))

	units, err := collectEmissionUnits(buildDir)
	if err != nil {
		return fmt.Errorf("read build state under %s: %w", buildDir, err)
	}
	waves, shared, cycles := scheduleEmissionWaves(units)

	out := emissionGroupsOutput{
		Waves:       waves,
		UnitCount:   len(units),
		SerialWaves: len(waves),
		SharedPaths: shared,
		Cycles:      cycles,
	}
	for _, w := range waves {
		if len(w.Units) > out.MaxWidth {
			out.MaxWidth = len(w.Units)
		}
	}

	// The schedule is only as trustworthy as the plans it is derived from, and
	// today's plans are known to be incomplete: build-feature emits no rows
	// for the project-scoped files codegen is nonetheless required to write —
	// the route table, the merged model layer, the app shell. Nineteen files
	// were written outside the plan allowlist in one regression run.
	//
	// Those omitted files are exactly the contended ones. So a schedule
	// computed from such plans reports no contention and full concurrency,
	// and running it would have several agents writing app.routes.ts at once.
	// One write wins, the losing feature's routes vanish, and the result still
	// parses and still builds — the worst failure mode available.
	//
	// Refusing to bless that is the whole point of computing this rather than
	// writing "emit components in parallel" into a skill.
	if len(out.SharedPaths) == 0 && out.UnitCount > 1 {
		out.Safe = false
		out.Unsafe = append(out.Unsafe,
			"no plan row is co-written by more than one feature, which for a multi-feature project "+
				"means the plans omit their project-scoped writes (route table, model layer, app shell) "+
				"rather than that there is no contention — see plan-derivation via `parlay internal scaffold-plan`")
		out.MaxWidth = 1
	} else if len(out.Cycles) > 0 {
		out.Safe = false
		out.Unsafe = append(out.Unsafe, "the dependency graph has a cycle; no ordering exists")
		out.MaxWidth = 1
	} else {
		out.Safe = true
	}

	buf, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	if len(cycles) > 0 {
		return NewExitCodeError(1)
	}
	return nil
}
