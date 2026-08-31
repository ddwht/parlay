// parlay-feature: parlay-tool/feedback-mode
// parlay-component: recorder
//
// An opt-in record of what a run actually did, written so that a user can
// send it to the parlay team without reviewing it first.
//
// WHY THIS EXISTS. The most expensive failures in this toolkit are not
// crashes, they are agents working around it. A build subagent authors a v2
// buildfile, gets it rejected, retries v1; omits `models:`, gets
// missing-model-reference, re-adds it. Every phase pays that independently
// and none of it is written down, so a schema that teaches by rejection is
// indistinguishable from one that teaches by documentation.
//
// WHY IT LOOKS LIKE THIS. The first version captured richly — argv, full
// error strings, whole validator messages — because it was designed on the
// assumption the log stayed on the machine. It does not: the point is that
// a user reproduces a problem and sends the log in. That inverts the
// requirement. A validator message interpolates paths, operation ids,
// entity names and, in a few places, verbatim prose out of the user's own
// spec files; roughly one message in five carries a filesystem path.
// Filtering that back out would be a denylist over ~86 construction sites
// and would fail open the first time a new message shape appeared.
//
// So nothing sensitive is written in the first place. Every value that
// reaches the file is either a member of parlay's own closed vocabulary —
// error codes, phase names, command names, decision kinds — or a salted
// hash. Free text has no field to live in. That is enforced three ways:
// per-kind payload types (below), a validating encoder at the single write
// point (encodeValue), and a property test that pushes adversarial input
// through every producer and asserts none of it survives.
package feedback

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Dir is the subdirectory under .parlay/ that holds the logs.
const Dir = "feedback"

// SchemaVersion is stamped on every event as "v".
//
// Versioned at the EVENT, not the file, because a day file can span an
// upgrade. Export refuses anything below this, which is what makes logs
// written by v0.2.3 — the release that captured argv and message text —
// mechanically impossible to send rather than merely discouraged.
const SchemaVersion = 2

// EnvVar forces the mode on or off for one invocation, whatever the
// project config says.
const EnvVar = "PARLAY_FEEDBACK"

// RunEnvVar carries the correlation id for one PIPELINE run across the
// many CLI invocations it makes.
//
// This has to come from the environment, and the first version of this
// package proved why by getting it wrong: it minted a fresh id per process
// and offered `feedback-status` as the way to read it. But status is itself
// a process, so it minted its own — an agent that asked for the run id
// received the id of the asking, and every event it then filed was
// correlated to nothing.
const RunEnvVar = "PARLAY_RUN_ID"

// SaltFile holds the per-project hashing salt. Never exported.
const SaltFile = ".salt"

// RetentionDays bounds how long day files are kept. Nothing rotated them
// before, so a project that enabled the mode and forgot accumulated
// indefinitely.
const RetentionDays = 14

// Redacted replaces any value that fails validation.
//
// A sentinel rather than a silent drop: the log records THAT something was
// rejected, which is a bug report about this package. Dropping the field
// would hide the one event worth investigating.
const Redacted = "redacted"

// Event kinds. A closed set — an open one becomes a dozen spellings of the
// same thing, and the whole point is that these aggregate.
const (
	// KindFinding is one diagnostic parlay produced. CLI-owned.
	KindFinding = "finding"
	// KindTally is one process's summary, written at Stop. CLI-owned.
	// It replaces the old per-invocation record and carries strictly less:
	// no argv, no error text, no exact duration.
	KindTally = "tally"
	// KindSession is the once-per-day-file header describing run shape.
	KindSession = "session"

	// KindPhase is a pipeline phase starting or finishing. Agent-emitted.
	KindPhase = "phase"
	// KindDecision is a parlay-decision block raised, and how it resolved.
	KindDecision = "decision"
	// KindRetry is the signal this package exists for: an agent
	// re-attempted something after a rejection.
	KindRetry = "retry"
	// KindImprovised is an agent proceeding without a rule it needed.
	KindImprovised = "improvised"
	// KindNote is the escape hatch. A log full of notes means the closed
	// set above is missing a kind that should be added.
	KindNote = "note"
)

// safeToken is the shape every free-position string must match to reach
// the file. Parlay's own vocabularies — error codes, phase names, command
// paths, kinds — all satisfy it; paths, sentences and identifiers with
// capitals or spaces do not.
var safeToken = regexp.MustCompile(`^[a-z0-9][a-z0-9._\- ]{0,63}$`)

// Payload is what Record accepts. Closed by construction: adding a field
// means editing this file, which is the review checkpoint.
//
// Per-kind types rather than one wide struct with fifteen omitempty
// fields — that shape degenerates back into the map it replaced, and a
// reviewer can no longer tell which fields a kind actually uses.
type Payload interface {
	kind() string
	fields() map[string]string
}

// FindingData is one diagnostic. No message, no context, no fix: the
// message is rendered from a template the parlay team already has in its
// own source, so the only new information in it is the interpolated
// values — which are exactly the user's data.
type FindingData struct {
	Code     string // parlay's own code vocabulary
	Mode     string // authoring | build
	Severity string // warning | error
	// Site is the emitting function's symbol, e.g.
	// ".../core/internal/agent.validateSupportsBlock". Several codes fire
	// from more than one branch — validate_adapter.go uses one add(code,
	// msg) closure across ~20 conditions — so a code alone cannot tell an
	// investigator which branch fired. Parlay's own symbols, no user
	// content, and free to collect.
	Site     string
	Phase    string // closed phase vocabulary, when known
	Artifact string // closed artifact vocabulary, when known
	Subject  string // ALREADY HASHED by the caller via Hash()
}

func (FindingData) kind() string { return KindFinding }
func (d FindingData) fields() map[string]string {
	return map[string]string{
		"code": d.Code, "mode": d.Mode, "severity": d.Severity,
		"site": d.Site, "phase": d.Phase, "artifact": d.Artifact,
		"subject": d.Subject,
	}
}

// TallyData is one process's summary. Emitted at Stop.
type TallyData struct {
	Command   string // cobra's own command path
	Exit      string // bucketed: ok | failed | exit-<n>
	MsBucket  string // under-100ms | 100ms-1s | 1s-10s | over-10s
	Findings  string // integer as string, encoded through the same guard
	Completed string // "true" — absent means the process died before Stop
}

func (TallyData) kind() string { return KindTally }
func (d TallyData) fields() map[string]string {
	return map[string]string{
		"command": d.Command, "exit": d.Exit, "ms_bucket": d.MsBucket,
		"findings": d.Findings, "completed": d.Completed,
	}
}

// SessionData describes the shape of the project, once per day file.
type SessionData struct {
	Version     string // parlay version, or "dev"
	OS          string
	Arch        string
	MultiRoot   string // true | false
	Features    string // bucketed: 0 | 1-5 | 6-20 | 20-plus
	Adapters    string // space-separated bundled names, custom ones hashed
	Interactive string // true | false
}

func (SessionData) kind() string { return KindSession }
func (d SessionData) fields() map[string]string {
	return map[string]string{
		"version": d.Version, "os": d.OS, "arch": d.Arch,
		"multi_root": d.MultiRoot, "features": d.Features,
		"adapters": d.Adapters, "interactive": d.Interactive,
	}
}

// AgentData is what a skill emits: phase, decision, retry, improvised,
// note. Every field is a closed enum validated by the intake command
// before it reaches here.
type AgentData struct {
	Kind     string // one of the agent-owned kinds
	Skill    string // parlay's own skill names
	Phase    string
	Artifact string
	Code     string // for retry: the code retried after
	Changed  string // closed enum
	Needed   string // closed enum
	Decision string // decision kind
	Option   string // chosen option id
	Subject  string // ALREADY HASHED
}

func (d AgentData) kind() string { return d.Kind }
func (d AgentData) fields() map[string]string {
	return map[string]string{
		"skill": d.Skill, "phase": d.Phase, "artifact": d.Artifact,
		"code": d.Code, "changed": d.Changed, "needed": d.Needed,
		"decision": d.Decision, "option": d.Option, "subject": d.Subject,
	}
}

// Event is one line of the log.
type Event struct {
	V    int               `json:"v"`
	At   string            `json:"at"`
	Run  string            `json:"run"`
	Proc string            `json:"proc"`
	Kind string            `json:"kind"`
	Data map[string]string `json:"data,omitempty"`
}

// Recorder appends events. The zero value is a valid disabled recorder, so
// every call site can be unconditional.
//
// Package-level rather than carried on config.Context because Execute()
// brackets the whole invocation and runs before any Context exists — and
// because instrumentation each call site must be handed is instrumentation
// that gets dropped from the call sites nobody remembered.
type Recorder struct {
	mu      sync.Mutex
	file    *os.File
	runID   string
	procID  string
	salt    []byte
	enabled bool
	// failed latches on the first write error. Feedback must never break a
	// command, and retrying per event would turn a full disk into a stall
	// in every command.
	failed bool
	// findings counts this process's findings, for the tally at Stop.
	findings int
	// needsSession records that this process created today's file.
	needsSession bool
}

var active = &Recorder{}

// Enabled reports whether the mode should run, given the project config's
// value and the environment.
//
// The env var overrides the config in BOTH directions, deliberately. This is
// a diagnostic mode: "on for this one run" and "off for this one run" are
// equally wanted, and an OR merge — the shape the retired editor opt-out used,
// where flag and config only ever combined toward "off" — expresses only the
// first.
func Enabled(configValue bool, env func(string) string) bool {
	raw, set := "", false
	if env != nil {
		raw = strings.TrimSpace(env(EnvVar))
		set = raw != ""
	}
	if !set {
		return configValue
	}
	switch strings.ToLower(raw) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// Start opens the log for one invocation under the given root. Errors are
// swallowed by design — a project that cannot write its feedback log still
// has to run.
//
// runID is the pipeline-run correlation id, normally read from RunEnvVar.
// Empty means this invocation is not part of a tracked pipeline run and
// gets its own, which is correct for a command run by hand.
func Start(rootPath string, enabled bool, runID string) {
	if !enabled || rootPath == "" {
		return
	}
	dir := filepath.Join(rootPath, ".parlay", Dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	// Containment before anything is written. .parlay/ is version
	// controlled by convention, so without this a user who enables the
	// mode commits their log and pushes it.
	writeGitignore(dir)
	pruneOldLogs(dir)

	// One file per day rather than per run: the patterns worth seeing —
	// the same code four times, a phase that always retries — span runs.
	path := filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}

	active.mu.Lock()
	active.file = f
	active.enabled = true
	active.findings = 0
	active.procID = newID()
	active.salt = loadOrCreateSalt(dir)
	active.mu.Unlock()

	// The correlation id is HASHED, not stored as given.
	//
	// It arrives from PARLAY_RUN_ID, and the loop driver is instructed to
	// build it from the feature name — so storing it verbatim would put
	// the feature on every single line, which is precisely what this
	// redesign exists to prevent. Hashing keeps correlation exact (every
	// process in one pipeline run hashes the same input to the same
	// digest) and costs nothing, because nobody reads a run id for its
	// content.
	//
	// It also cannot go through encodeValue: a legitimate id like
	// "20260804T143000Z-checkout" has capitals and would be redacted to a
	// constant, collapsing every run into one.
	active.mu.Lock()
	if id := strings.TrimSpace(runID); id != "" {
		active.runID = hashWith(active.salt, id)
	} else {
		active.runID = newID()
	}
	active.mu.Unlock()

	// First writer to today's file owes the header. Two concurrent first
	// processes produce two identical headers, which is harmless — and the
	// size check costs nothing, where a per-invocation project scan would
	// be paid a dozen times per pipeline run.
	//
	// The header is not built here. Describing the project needs to know
	// about adapters, roots and the feature tree, and this package stays a
	// leaf with no project knowledge — which is also what lets the AST
	// guard reason about it. The caller asks NeedsSession and supplies it.
	// Assigned, not latched. An earlier version only ever set this to true,
	// so a package global stayed true for the rest of the process and every
	// subsequent Start re-emitted the header.
	info, statErr := f.Stat()
	active.mu.Lock()
	active.needsSession = statErr == nil && info.Size() == 0
	active.mu.Unlock()
}

// LogPath is today's log file under the given root — the real path, so a
// user asked to send their log can find it.
func LogPath(rootPath string) string {
	return filepath.Join(rootPath, ".parlay", Dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
}

// LogDir is the directory holding every day file.
func LogDir(rootPath string) string {
	return filepath.Join(rootPath, ".parlay", Dir)
}

// NeedsSession reports whether this process opened today's log file and
// therefore owes the one-per-file session header.
func NeedsSession() bool {
	active.mu.Lock()
	defer active.mu.Unlock()
	return active.needsSession && active.enabled && active.file != nil
}

// Stop writes this process's tally and closes the log.
func Stop(command, exit string, elapsed time.Duration) {
	active.mu.Lock()
	enabled := active.enabled && active.file != nil && !active.failed
	findings := active.findings
	active.mu.Unlock()

	if enabled {
		Record(TallyData{
			Command:   command,
			Exit:      exit,
			MsBucket:  bucketDuration(elapsed),
			Findings:  fmt.Sprintf("%d", findings),
			Completed: "true",
		})
	}

	active.mu.Lock()
	defer active.mu.Unlock()
	if active.file != nil {
		active.file.Close()
		active.file = nil
	}
	active.enabled = false
}

// Discard closes the log without writing a tally, for invocations that
// should not count toward the denominator.
func Discard() {
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.file != nil {
		active.file.Close()
		active.file = nil
	}
	active.enabled = false
}

// IsEnabled reports whether events are being recorded this run.
func IsEnabled() bool {
	active.mu.Lock()
	defer active.mu.Unlock()
	return active.enabled && active.file != nil && !active.failed
}

// RunID is the correlation id for this invocation.
func RunID() string {
	active.mu.Lock()
	defer active.mu.Unlock()
	return active.runID
}

// SetRunID adopts a caller-supplied correlation id.
func SetRunID(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	// Hashed on the same reasoning as Start's: the id is built from the
	// feature name, and it lands on every line.
	active.runID = hashWith(active.salt, id)
}

// Record appends one event. A no-op when the mode is off, and it never
// returns an error: a command must not fail because its telemetry did.
func Record(p Payload) {
	if p == nil {
		return
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	if !active.enabled || active.file == nil || active.failed {
		return
	}
	if p.kind() == KindFinding {
		active.findings++
	}

	data := map[string]string{}
	for k, v := range p.fields() {
		if v == "" {
			continue
		}
		data[k] = encodeValue(v)
	}

	ev := Event{
		V:    SchemaVersion,
		At:   time.Now().UTC().Format(time.RFC3339Nano),
		Run:  active.runID,
		Proc: active.procID,
		Kind: p.kind(),
		Data: data,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		active.failed = true
		return
	}
	// One small write to an O_APPEND regular file allocates its offset
	// atomically, so concurrent parlay processes cannot tear each other's
	// lines. That property is why JSONL is the container.
	if _, err := active.file.Write(append(line, '\n')); err != nil {
		active.failed = true
	}
}

// Finding is the convenience wrapper for the diagnostic path.
func Finding(code, mode, severity, site string) {
	if code == "" {
		return
	}
	Record(FindingData{Code: code, Mode: mode, Severity: severity, Site: site})
}

// encodeValue is the guard every value passes on its way to the file.
//
// This is where the guarantee actually lives. The payload types make a
// leak conspicuous; they do not make it impossible, because Subject(x)
// compiles for any x. A value that is not a safe token is replaced rather
// than written, so a mistake at a call site degrades to a redaction marker
// instead of shipping someone's filesystem layout.
func encodeValue(v string) string {
	if !safeToken.MatchString(v) {
		return Redacted
	}
	return v
}

// Hash pseudonymizes a user identifier with the per-project salt.
//
// Stable within a project so "the same feature failed four times" survives,
// and meaningless across projects. Salted rather than a bare sha256 because
// an unsalted digest of "checkout" is the same everywhere and falls to a
// dictionary in seconds.
//
// The only analytic loss is cross-project identifier joins, which are worth
// approximately nothing: parlay's diagnostics are about artifact shape, not
// about which noun a user picked.
func Hash(s string) string {
	active.mu.Lock()
	salt := active.salt
	active.mu.Unlock()
	return hashWith(salt, s)
}

// hashWith is Hash with the salt passed in, for callers that already hold
// the lock.
func hashWith(salt []byte, s string) string {
	s = normalize(s)
	if s == "" {
		return ""
	}
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// normalize lowercases and trims so that trivially different spellings of
// one identifier hash alike.
//
// Written here rather than reusing parser.Slugify: importing the parser
// into a leaf telemetry package, for five lines, is coupling this package
// should not have. Same reason commands.sha256Hex is not reused — it is
// unexported, and feedback must not depend on commands.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Bucket coarsens a count so an exact project size never leaves.
func Bucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n <= 5:
		return "1-5"
	case n <= 20:
		return "6-20"
	default:
		return "20-plus"
	}
}

func bucketDuration(d time.Duration) string {
	switch {
	case d < 100*time.Millisecond:
		return "under-100ms"
	case d < time.Second:
		return "100ms-1s"
	case d < 10*time.Second:
		return "1s-10s"
	default:
		return "over-10s"
	}
}

// CallerSite returns the symbol name of the function skip frames up, for
// the Site field. Symbol only — never runtime.Caller's file return, which
// under `go run` is a path inside the user's tree.
func CallerSite(skip int) string {
	pcs := make([]uintptr, 1)
	if runtime.Callers(skip+2, pcs) == 0 {
		return ""
	}
	fn := runtime.FuncForPC(pcs[0])
	if fn == nil {
		return ""
	}
	name := fn.Name()
	// Keep the trailing package.Function only; the module prefix is
	// constant and wastes a third of every finding line.
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return strings.ToLower(name)
}

// loadOrCreateSalt reads the per-project salt, creating it on first use.
// A failure yields a nil salt, which still hashes — consistently for this
// process — rather than failing the command.
func loadOrCreateSalt(dir string) []byte {
	path := filepath.Join(dir, SaltFile)
	if b, err := os.ReadFile(path); err == nil && len(b) >= 16 {
		return b
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil
	}
	_ = os.WriteFile(path, b, 0600)
	return b
}

// EnsureContained adds the gitignore to a log directory that already
// exists, for projects that enabled the mode before it was written.
//
// A no-op when the directory is absent, so `upgrade` can call it
// unconditionally without creating a feedback directory in a project that
// never asked for one.
func EnsureContained(rootPath string) {
	dir := LogDir(rootPath)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	writeGitignore(dir)
}

// writeGitignore keeps the log out of the user's commits. .parlay/ is
// version controlled by convention, so the enclosing directory is already
// tracked in most projects.
func writeGitignore(dir string) {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, []byte("# Feedback logs are local diagnostic data.\n*\n"), 0644)
}

// pruneOldLogs bounds retention. Runs on the path that already does
// MkdirAll, so it costs one ReadDir per invocation.
func pruneOldLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -RetentionDays)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		day, perr := time.Parse("2006-01-02", strings.TrimSuffix(name, ".jsonl"))
		if perr != nil || !day.Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Uniqueness matters only within a day's log, and a collision
		// costs a mis-grouped pair of events rather than a wrong answer.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
