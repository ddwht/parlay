// parlay-feature: parlay-tool/backlog-and-activity

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

func backlogCodesOf(fs []backlogFinding) map[string]string {
	out := map[string]string{}
	for _, f := range fs {
		out[f.Code] = f.Message
	}
	return out
}

// A `becomes:` THAT STOPPED RESOLVING must be reported.
//
// The mutation commands prevent CREATING one — fold resolves its
// destination and requires it open, promote scaffolds before it records
// — but nothing detected one that had since gone dangling. A closed item
// is never revisited, so a broken `becomes:` is a permanently wrong
// answer to "what did this become" that nobody would otherwise see.
func TestCrossFile_DanglingRefsAreReported(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	now := time.Now().UTC()

	// A fold whose destination is then deleted.
	src := capture(t, cfg, "gap", "Folded away", nil)
	dst := capture(t, cfg, "gap", "Destination", nil)
	if _, err := backlogCLI(t, cfg, "fold", src, "--into", dst, "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(parser.BacklogRoot(cfg.Root.Path), dst+".yaml")); err != nil {
		t.Fatal(err)
	}

	// A promotion whose feature is then deleted.
	promoted := capture(t, cfg, "gap", "Promoted away", nil)
	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})
	if err := promoteFeature(cmd, cfg, promoted, "Vanishing Feature", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfg.FeaturePath("vanishing-feature")); err != nil {
		t.Fatal(err)
	}

	// An amendment closure whose amendment is then deleted.
	amended := capture(t, cfg, "gap", "Amended away", nil)
	amendments := parser.AmendmentsDir(featurePath)
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\namendment: gone\ndate: 2026-09-01\ntrigger: backlog:" + amended + "\n---\n\n## Change\n\nx\n"
	amPath := filepath.Join(amendments, "001-gone.md")
	if err := os.WriteFile(amPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backlogCLI(t, cfg, "amend", amended, "--into", "@widget/001-gone", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(amPath); err != nil {
		t.Fatal(err)
	}

	items, broken, err := loadBacklog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := backlogCodesOf(crossFileFindings(cfg, "root", items, broken, now))

	if _, ok := got[CodeBacklogFoldDangling]; !ok {
		t.Errorf("a fold into a deleted item was not reported: %v", got)
	}
	if _, ok := got[CodeBacklogPromotionDangling]; !ok {
		t.Errorf("a promotion into a deleted feature was not reported: %v", got)
	}
}

// A ref that STILL resolves must not be reported. A check that fires on
// healthy state is one people learn to ignore.
func TestCrossFile_ResolvableRefsAreSilent(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")

	src := capture(t, cfg, "gap", "Folded", nil)
	dst := capture(t, cfg, "gap", "Destination", nil)
	if _, err := backlogCLI(t, cfg, "fold", src, "--into", dst, "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	promoted := capture(t, cfg, "gap", "Promoted", nil)
	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})
	if err := promoteFeature(cmd, cfg, promoted, "Real Feature", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	amended := capture(t, cfg, "gap", "Amended", nil)
	amendments := parser.AmendmentsDir(featurePath)
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\namendment: real\ndate: 2026-09-01\ntrigger: backlog:" + amended + "\n---\n\n## Change\n\nx\n"
	if err := os.WriteFile(filepath.Join(amendments, "001-real.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backlogCLI(t, cfg, "amend", amended, "--into", "@widget/001-real", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}

	items, broken, err := loadBacklog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range crossFileFindings(cfg, "root", items, broken, time.Now().UTC()) {
		if f.Code != CodeBacklogItemStale {
			t.Errorf("a resolvable ref was reported as dangling: %+v", f)
		}
	}
}

// AGE is a warning, and a deferral does NOT reset it. Treating a
// deferral as a fresh lease would let an item stay invisible by being
// repeatedly not-decided — the exact failure the age signal exists to
// surface.
func TestCrossFile_StaleIsByAgeAndDeferralDoesNotResetIt(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	fresh := capture(t, cfg, "gap", "Just captured", nil)
	old := capture(t, cfg, "gap", "Long open", nil)
	if _, err := backlogCLI(t, cfg, "defer", old, "--reason", "cannot say", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	closed := capture(t, cfg, "gap", "Long closed", nil)
	if _, err := backlogCLI(t, cfg, "decline", closed, "--reason", "no", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}

	items, broken, err := loadBacklog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Just under the threshold: nothing is stale.
	justUnder := time.Now().UTC().Add(staleAfter - time.Hour)
	for _, f := range crossFileFindings(cfg, "root", items, broken, justUnder) {
		if f.Code == CodeBacklogItemStale {
			t.Errorf("an item younger than the bucket was reported stale: %+v", f)
		}
	}

	// Past it: the open ones are, the closed one is not.
	past := time.Now().UTC().Add(staleAfter + 24*time.Hour)
	stale := map[string]string{}
	for _, f := range crossFileFindings(cfg, "root", items, broken, past) {
		if f.Code == CodeBacklogItemStale {
			stale[f.Item] = f.Message
		}
	}
	if _, ok := stale[fresh]; !ok {
		t.Error("an open item past the bucket was not reported")
	}
	if msg, ok := stale[old]; !ok {
		t.Error("a DEFERRED item past the bucket was not reported — a deferral is review context, not a fresh lease")
	} else if !strings.Contains(msg, "deferral") {
		t.Errorf("the finding does not mention the prior deferral, which is what the next reviewer needs: %q", msg)
	}
	if _, ok := stale[closed]; ok {
		t.Error("a CLOSED item was reported stale; age only matters while something is undecided")
	}
}

// The findings must reach the surfaces a person actually reads.
func TestCrossFile_FindingsSurfaceInListAndReview(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	src := capture(t, cfg, "gap", "Folded away", nil)
	dst := capture(t, cfg, "gap", "Destination", nil)
	if _, err := backlogCLI(t, cfg, "fold", src, "--into", dst, "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(parser.BacklogRoot(cfg.Root.Path), dst+".yaml")); err != nil {
		t.Fatal(err)
	}

	inv := collectBacklogInventory(cfg, backlogListFilters{openOnly: true})
	if len(inv.Findings) == 0 {
		t.Fatal("list does not carry cross-file findings")
	}
	var out bytes.Buffer
	writeBacklogInventory(&out, inv)
	if !strings.Contains(out.String(), CodeBacklogFoldDangling) {
		t.Errorf("the human listing does not name the code:\n%s", out.String())
	}

	// Reported regardless of filters: a dangling ref does not become
	// less wrong because the reader narrowed the listing.
	narrowed := collectBacklogInventory(cfg, backlogListFilters{openOnly: true, kind: "debt"})
	if len(narrowed.Findings) == 0 {
		t.Error("a filter hid a dangling ref, which is how it stays unnoticed")
	}

	got := reviewBacklog(t, cfg)
	if len(got.Findings) == 0 {
		t.Error("next-backlog-review does not carry cross-file findings")
	}
}

// `captured` and `history` are ENFORCED immutable, not merely
// unexpressible. The old argument was that no caller has a line that
// could reach them — true, and worth nothing the moment somebody adds
// one.
func TestMutate_RefusesToRewriteProvenanceOrHistory(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Guarded", nil)
	if _, err := backlogCLI(t, cfg, "defer", id, "--reason", "cannot say", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parser.BacklogRoot(cfg.Root.Path), id+".yaml")
	before := itemByID(t, cfg, id)

	for _, tc := range []struct {
		name string
		mut  func(*parser.BacklogItem) error
		code string
	}{
		{"rewrite captured.by", func(i *parser.BacklogItem) error {
			i.Captured.By = "somebody-else"
			return nil
		}, agent.CodeBacklogCapturedUpdateForbidden},
		{"rewrite captured.at", func(i *parser.BacklogItem) error {
			i.Captured.At = "2020-01-01T00:00:00Z"
			return nil
		}, agent.CodeBacklogCapturedUpdateForbidden},
		{"edit an existing event", func(i *parser.BacklogItem) error {
			i.History[0].Reason = "a different reason"
			return nil
		}, agent.CodeBacklogHistoryUpdateForbidden},
		{"drop an event", func(i *parser.BacklogItem) error {
			i.History = nil
			return nil
		}, agent.CodeBacklogHistoryUpdateForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := mutateBacklogItem(cfg, path, tc.mut)
			if err == nil {
				t.Fatalf("%s was permitted", tc.name)
			}
			if !strings.Contains(err.Error(), tc.code) {
				t.Errorf("the refusal does not carry the published code %s: %v", tc.code, err)
			}
			if after := itemByID(t, cfg, id); after.Captured != before.Captured || len(after.History) != len(before.History) {
				t.Errorf("the refused mutation still changed the item")
			}
		})
	}

	// Appending is still legal — the guard must not block the one thing
	// the history is for.
	if err := mutateBacklogItem(cfg, path, func(i *parser.BacklogItem) error {
		i.History = append(i.History, parser.BacklogEvent{
			Event: parser.EventDeferred, Reason: "still cannot say",
			At: time.Now().UTC().Format(time.RFC3339Nano), By: "dwht"})
		return nil
	}); err != nil {
		t.Fatalf("appending a disposition was refused: %v", err)
	}
	if n := len(itemByID(t, cfg, id).Deferrals()); n != 2 {
		t.Errorf("the append did not land: %d deferrals", n)
	}
}

// An undeclared feature has no file to diagnose, so the declaration
// validator can never emit its code — but it IS the finding, and a
// reviewer keying on codes would otherwise get a subject with none.
func TestActivityReview_UndeclaredCarriesItsPublishedCode(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	got := review(t, cfg)
	if got.Subject == nil {
		t.Fatal("an undeclared feature produced no subject")
	}
	var found bool
	for _, f := range got.Subject.Findings {
		if f.Code == agent.CodeActivityUndeclared {
			found = true
			if f.Fix == "" {
				t.Error("the finding does not say how to resolve it")
			}
		}
	}
	if !found {
		t.Errorf("the undeclared subject carries no %s finding: %+v", agent.CodeActivityUndeclared, got.Subject.Findings)
	}

	// Once declared, it is neither a subject nor a finding.
	if err := park(t, cfg, "@widget", "waiting on the upstream decision", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	after := review(t, cfg)
	if after.Subject != nil {
		t.Errorf("a declared feature is still being reviewed: %+v", after.Subject)
	}
}

// A COMPACTED amendment is retained history, not a deletion.
//
// `parlay internal compact` moves applied records into
// amendments/archive/, and the canonical loader's own doc says anything
// resolving applied history has to see both directories. Looking only in
// amendments/ turned every routine compaction into a dangling provenance
// link on every item closed against a record it archived.
func TestCrossFile_CompactedAmendmentIsNotDangling(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Closed by an amendment", nil)

	// authored → applied-against → compacted.
	amendments := parser.AmendmentsDir(featurePath)
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:" + id + "\n---\n\n## Change\n\nx\n"
	live := filepath.Join(amendments, "001-filter.md")
	if err := os.WriteFile(live, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}

	// Clean before compaction.
	items, broken, err := loadBacklog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := backlogCodesOf(crossFileFindings(cfg, "root", items, broken, time.Now().UTC())); got[CodeBacklogPromotionDangling] != "" {
		t.Fatalf("a live amendment was reported dangling: %v", got)
	}

	// Compaction moves it. The record still exists and is still what the
	// item became.
	archive := filepath.Join(amendments, "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(live, filepath.Join(archive, "001-filter.md")); err != nil {
		t.Fatal(err)
	}

	items, broken, err = loadBacklog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range crossFileFindings(cfg, "root", items, broken, time.Now().UTC()) {
		if f.Code == CodeBacklogPromotionDangling {
			t.Errorf("compaction turned retained ledger history into a dangling link: %+v", f)
		}
	}

	// And a record that is in NEITHER directory is still reported.
	if err := os.Remove(filepath.Join(archive, "001-filter.md")); err != nil {
		t.Fatal(err)
	}
	items, broken, _ = loadBacklog(cfg)
	if got := backlogCodesOf(crossFileFindings(cfg, "root", items, broken, time.Now().UTC())); got[CodeBacklogPromotionDangling] == "" {
		t.Error("a genuinely deleted amendment is no longer reported — the archive lookup swallowed the real case")
	}
}

// UNREADABLE IS NOT MISSING, for either ref type.
//
// Fold resolution built its index only from parseable items, so a
// destination that exists and will not parse was reported as "removed or
// renamed" — a confident claim about a file nobody read — while the same
// run separately reported it as unreadable. Promotion read every
// os.Stat error as absence, so a permissions fault became a deletion.
func TestCrossFile_UnreadableIsNotReportedAsMissing(t *testing.T) {
	t.Run("malformed fold destination", func(t *testing.T) {
		cfg, _ := parkFixture(t, "widget")
		src := capture(t, cfg, "gap", "Folded", nil)
		dst := capture(t, cfg, "gap", "Destination", nil)
		if _, err := backlogCLI(t, cfg, "fold", src, "--into", dst, "--by", "dwht"); err != nil {
			t.Fatal(err)
		}
		// Present, and will not parse.
		if err := os.WriteFile(filepath.Join(parser.BacklogRoot(cfg.Root.Path), dst+".yaml"),
			[]byte("{{{ not yaml"), 0o644); err != nil {
			t.Fatal(err)
		}

		items, broken, err := loadBacklog(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(broken) == 0 {
			t.Fatal("the fixture did not produce an unreadable record")
		}
		for _, f := range crossFileFindings(cfg, "root", items, broken, time.Now().UTC()) {
			if f.Code == CodeBacklogFoldDangling {
				t.Errorf("a file that exists and cannot be read was reported as removed: %+v", f)
			}
		}

		// The listing still tells the user something is wrong — under
		// the honest diagnosis, not a contradictory one.
		inv := collectBacklogInventory(cfg, backlogListFilters{})
		if len(inv.Unreadable) == 0 {
			t.Error("the unreadable record vanished from the listing entirely")
		}
	})

	t.Run("unstattable promotion target", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: permission faults are not reproducible")
		}
		cfg, _ := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "Promoted", nil)
		cmd := testCommandWithContext(t, cfg)
		cmd.SetOut(&bytes.Buffer{})
		if err := promoteFeature(cmd, cfg, id, "Blocked Feature", "", "dwht"); err != nil {
			t.Fatal(err)
		}

		// Make the PARENT unsearchable, so stat on the feature fails
		// with EACCES rather than ENOENT.
		parent := filepath.Dir(cfg.FeaturePath("blocked-feature"))
		if err := os.Chmod(parent, 0o000); err != nil {
			t.Skipf("cannot remove search permission here: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

		if got := featureTargetResolves(cfg, "@blocked-feature"); got != refUnavailable {
			t.Errorf("an unstattable target resolved as %v, want refUnavailable — a permissions fault is not a deletion", got)
		}
	})
}

// THE EVENT KIND DECIDES WHICH OBJECT IS LOOKED FOR.
//
// Resolution used to try the whole ref as a feature first and fall back
// to an amendment, so the WRONG OBJECT could mask a dangling ref: an
// item recorded as amended into `@widget/001-filter` resolved clean
// whenever an initiative-qualified feature happened to be named
// `widget/001-filter`, whether or not the amendment existed.
func TestCrossFile_AmendedDoesNotResolveAgainstAFeatureOfTheSameName(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Closed by an amendment", nil)

	// Close it against a real amendment, then delete the amendment and
	// create a FEATURE at the colliding path.
	amendments := parser.AmendmentsDir(featurePath)
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:" + id + "\n---\n\n## Change\n\nx\n"
	amPath := filepath.Join(amendments, "001-filter.md")
	if err := os.WriteFile(amPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(amPath); err != nil {
		t.Fatal(err)
	}
	// The collision: an initiative-qualified feature at widget/001-filter.
	if err := os.MkdirAll(cfg.FeaturePath("widget/001-filter"), 0o755); err != nil {
		t.Fatal(err)
	}

	items, broken, err := loadBacklog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := backlogCodesOf(crossFileFindings(cfg, "root", items, broken, time.Now().UTC())); got[CodeBacklogPromotionDangling] == "" {
		t.Errorf("a feature of the same name masked a missing amendment: %v", got)
	}

	// And the same textual ref, reached as a PROMOTION, does resolve —
	// because for that event it names exactly the feature that exists.
	if got := featureTargetResolves(cfg, "@widget/001-filter"); got != refResolved {
		t.Errorf("promoted into an existing feature resolved as %v, want refResolved", got)
	}
	if got := amendmentTargetResolves(cfg, "@widget/001-filter", id); got.resolution != refMissing {
		t.Errorf("amended into a missing amendment resolved as %v, want refMissing", got.resolution)
	}
}

// An amendment that EXISTS and will not parse is unavailable, not
// missing — the same absence-versus-unavailability rule, applied to the
// content rather than only to the directory entry. os.Stat proves a file
// is there, not that it is a readable amendment.
func TestCrossFile_CorruptAmendmentIsUnavailableNotMissing(t *testing.T) {
	for _, where := range []string{"live", "archive"} {
		t.Run(where, func(t *testing.T) {
			cfg, featurePath := parkFixture(t, "widget")
			id := capture(t, cfg, "gap", "Closed by an amendment", nil)

			amendments := parser.AmendmentsDir(featurePath)
			dir := amendments
			if where == "archive" {
				dir = filepath.Join(amendments, "archive")
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			good := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:" + id + "\n---\n\n## Change\n\nx\n"
			if err := os.WriteFile(filepath.Join(amendments, "001-filter.md"), []byte(good), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter", "--by", "dwht"); err != nil {
				t.Fatal(err)
			}
			// Now corrupt it where the subtest says.
			if where == "archive" {
				if err := os.Rename(filepath.Join(amendments, "001-filter.md"), filepath.Join(dir, "001-filter.md")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "001-filter.md"), []byte("not an amendment at all"), 0o644); err != nil {
				t.Fatal(err)
			}

			got := amendmentTargetResolves(cfg, "@widget/001-filter", id)
			if got.resolution != refUnavailable {
				t.Errorf("a corrupt %s amendment resolved as %v, want refUnavailable", where, got.resolution)
			}
			items, broken, err := loadBacklog(cfg)
			if err != nil {
				t.Fatal(err)
			}
			codes := backlogCodesOf(crossFileFindings(cfg, "root", items, broken, time.Now().UTC()))
			if codes[CodeBacklogPromotionDangling] != "" {
				t.Errorf("an unreadable amendment was reported as removed: %q", codes[CodeBacklogPromotionDangling])
			}
			// AND it must still be reported. Tri-state was honest inside
			// the resolver and dishonest at the surface: refUnavailable
			// produced no finding at all, so a corrupt amendment vanished
			// from list and review entirely — worse than mislabelling it.
			if codes[CodeBacklogPromotionTargetUnavailable] == "" {
				t.Errorf("an unreadable amendment produced NO finding — the fault disappeared: %v", codes)
			}

			// The surfaces a person reads, not only the resolver.
			inv := collectBacklogInventory(cfg, backlogListFilters{})
			var out bytes.Buffer
			writeBacklogInventory(&out, inv)
			if !strings.Contains(out.String(), CodeBacklogPromotionTargetUnavailable) {
				t.Errorf("the human listing does not report it:\n%s", out.String())
			}
			rev := reviewBacklog(t, cfg)
			var inReview bool
			for _, f := range rev.Findings {
				if f.Code == CodeBacklogPromotionTargetUnavailable {
					inReview = true
				}
			}
			if !inReview {
				t.Errorf("next-backlog-review does not report it: %+v", rev.Findings)
			}
		})
	}
}

// The trigger was required to name this item when the item was closed.
// An amendment that exists but no longer names it is exactly the
// post-mutation drift these cross-file checks exist to catch.
func TestCrossFile_AmendmentThatNoLongerNamesTheItemIsDangling(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Closed by an amendment", nil)

	amendments := parser.AmendmentsDir(featurePath)
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	amPath := filepath.Join(amendments, "001-filter.md")
	good := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:" + id + "\n---\n\n## Change\n\nx\n"
	if err := os.WriteFile(amPath, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}

	// The record drifts: same file, different cause.
	drifted := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:something-else\n---\n\n## Change\n\nx\n"
	if err := os.WriteFile(amPath, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}

	items, broken, err := loadBacklog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	msg := backlogCodesOf(crossFileFindings(cfg, "root", items, broken, time.Now().UTC()))[CodeBacklogPromotionDangling]
	if msg == "" {
		t.Fatal("an amendment that stopped naming this item was not reported")
	}
	if !strings.Contains(msg, "no longer names this item") {
		t.Errorf("the finding does not distinguish drift from deletion: %q", msg)
	}
}

// Each resolution carries the fix that BELONGS to it.
//
// Trigger drift used to return plain refMissing and then collect the
// generic dangling fix, "the amendment was removed or renamed" — flatly
// contradicting its own message, which said the amendment exists and
// names another item. A finding whose halves disagree is worse than none:
// the reader has to decide which half to believe.
//
// And neither remedy may tell somebody to edit an append-only record.
func TestCrossFile_FindingsCarryTheFixThatMatchesTheirMessage(t *testing.T) {
	amend := func(t *testing.T, trailer func(cfg *config.Context, featurePath, id, amPath string)) []backlogFinding {
		t.Helper()
		cfg, featurePath := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "Closed by an amendment", nil)
		amendments := parser.AmendmentsDir(featurePath)
		if err := os.MkdirAll(amendments, 0o755); err != nil {
			t.Fatal(err)
		}
		amPath := filepath.Join(amendments, "001-filter.md")
		good := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:" + id + "\n---\n\n## Change\n\nx\n"
		if err := os.WriteFile(amPath, []byte(good), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter", "--by", "dwht"); err != nil {
			t.Fatal(err)
		}
		trailer(cfg, featurePath, id, amPath)
		items, broken, err := loadBacklog(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return crossFileFindings(cfg, "root", items, broken, time.Now().UTC())
	}

	t.Run("deletion gets removal guidance", func(t *testing.T) {
		fs := amend(t, func(_ *config.Context, _, _, amPath string) {
			if err := os.Remove(amPath); err != nil {
				t.Fatal(err)
			}
		})
		f := findingWithCode(t, fs, CodeBacklogPromotionDangling)
		if !strings.Contains(f.Fix, "removed or renamed") {
			t.Errorf("a deletion does not get removal guidance: %q", f.Fix)
		}
	})

	t.Run("trigger drift does NOT get removal guidance", func(t *testing.T) {
		fs := amend(t, func(_ *config.Context, _, _, amPath string) {
			drifted := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:something-else\n---\n\n## Change\n\nx\n"
			if err := os.WriteFile(amPath, []byte(drifted), 0o644); err != nil {
				t.Fatal(err)
			}
		})
		f := findingWithCode(t, fs, CodeBacklogPromotionDangling)
		if !strings.Contains(f.Message, "no longer names this item") {
			t.Fatalf("the message is not the drift one: %q", f.Message)
		}
		if strings.Contains(f.Fix, "removed or renamed") {
			t.Errorf("the fix contradicts the message it is attached to: %q / %q", f.Message, f.Fix)
		}
		if !strings.Contains(f.Fix, "causal link changed") {
			t.Errorf("the fix does not describe what actually happened: %q", f.Fix)
		}
		// Both records are append-only; a remedy must not casually
		// instruct editing either.
		if !strings.Contains(f.Fix, "append-only") || !strings.Contains(f.Fix, "provenance repair") {
			t.Errorf("the fix does not say this needs provenance repair rather than an ordinary edit: %q", f.Fix)
		}
	})

	t.Run("unreadable gets repair guidance and no accusation", func(t *testing.T) {
		fs := amend(t, func(_ *config.Context, _, _, amPath string) {
			if err := os.WriteFile(amPath, []byte("not an amendment"), 0o644); err != nil {
				t.Fatal(err)
			}
		})
		f := findingWithCode(t, fs, CodeBacklogPromotionTargetUnavailable)
		if strings.Contains(f.Fix, "removed or renamed") {
			t.Errorf("an unreadable target is accused of being deleted: %q", f.Fix)
		}
		if !strings.Contains(f.Fix, "nobody has established") {
			t.Errorf("the fix does not say the link is unproven rather than broken: %q", f.Fix)
		}
	})
}

// SPLIT HISTORY must not resolve by first success.
//
// A record present in both the live directory and archive/ is duplicate
// history, which the canonical ledger treats as an integrity fault.
// Returning on the first candidate that parses would let a valid
// archived copy silently mask an unreadable or conflicting live
// duplicate — resolving clean against exactly the state that is wrong.
func TestCrossFile_SplitHistoryIsUnavailableNotResolved(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Closed by an amendment", nil)

	amendments := parser.AmendmentsDir(featurePath)
	archive := filepath.Join(amendments, "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	good := "---\namendment: filter\ndate: 2026-09-01\ntrigger: backlog:" + id + "\n---\n\n## Change\n\nx\n"
	if err := os.WriteFile(filepath.Join(amendments, "001-filter.md"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	// A COPY in the archive — compaction moves, it does not copy.
	if err := os.WriteFile(filepath.Join(archive, "001-filter.md"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}

	got := amendmentTargetResolves(cfg, "@widget/001-filter", id)
	if got.resolution != refUnavailable {
		t.Errorf("split history resolved as %v, want refUnavailable", got.resolution)
	}
	if !strings.Contains(got.reason, "split history") {
		t.Errorf("the reason does not name the fault: %q", got.reason)
	}

	// And the same holds when the LIVE copy is the unreadable one: a
	// valid archive must not mask it.
	if err := os.WriteFile(filepath.Join(amendments, "001-filter.md"), []byte("not an amendment"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := amendmentTargetResolves(cfg, "@widget/001-filter", id); got.resolution != refUnavailable {
		t.Errorf("a valid archive masked an unreadable live duplicate: %v", got.resolution)
	}
}

func findingWithCode(t *testing.T, fs []backlogFinding, code string) backlogFinding {
	t.Helper()
	for _, f := range fs {
		if f.Code == code {
			return f
		}
	}
	t.Fatalf("no %s finding in %+v", code, fs)
	return backlogFinding{}
}
