package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0x666c6f/berth/pkg/config"
	"github.com/0x666c6f/berth/pkg/docker"
	"github.com/0x666c6f/berth/pkg/fleet"
	"github.com/0x666c6f/berth/pkg/labels"
	"github.com/0x666c6f/berth/pkg/vmexec"
)

// defaultJudgeDiffCap bounds how many bytes of each candidate diff are embedded
// in the judge prompt. Large diffs are truncated with an inline note so the
// judge prompt stays within a reasonable size.
const defaultJudgeDiffCap = 12000

// JudgeVerdict is the strict-JSON verdict the judge agent must emit.
type JudgeVerdict struct {
	Winner  string `json:"winner"`
	Reason  string `json:"reason"`
	Summary string `json:"summary"`
}

// JudgeCandidate is one competing implementation the judge compares.
type JudgeCandidate struct {
	Name    string // container name
	Agent   string // agent type label (claude/codex)
	Diff    string // working-tree diff of /workspace
	Message string // last agent message
}

// judgeRecord is the persisted judge outcome under the state dir.
type judgeRecord struct {
	Stage      string       `json:"stage"`
	Pipeline   string       `json:"pipeline"`
	Timestamp  string       `json:"timestamp"`
	Criteria   string       `json:"criteria,omitempty"`
	Candidates []string     `json:"candidates"`
	Verdict    JudgeVerdict `json:"verdict"`
	AutoPR     bool         `json:"auto_pr"`
	PRBranch   string       `json:"pr_branch,omitempty"`
	PRResult   string       `json:"pr_result,omitempty"`
	Failed     bool         `json:"failed,omitempty"`
	Error      string       `json:"error,omitempty"`
	RawOutput  string       `json:"raw_output,omitempty"`
}

// Seams — overridable in tests to isolate orchestration from heavy I/O.
var (
	judgeSpawn             = executeSpawn
	judgeReadOutput        = outputLastMessage
	judgeCollectCandidates = collectJudgeCandidatesImpl
	judgeCreatePR          = createWinnerPRImpl
)

// runJudgeStage executes a best-of-N judge stage: it collects the candidate
// containers produced by the stage's dependencies, runs a one-shot judge agent
// to select a winner, persists the verdict, and optionally opens a PR from the
// winning container. Candidate containers have already exited by the time a
// judge stage becomes ready (they are its dependencies).
func runJudgeStage(ctx context.Context, exec vmexec.Executor, stage fleet.PipelineStage, stageContainers map[string][]string, pipelineName, timestamp string) error {
	names := judgeCandidateNames(stage, stageContainers)
	if len(names) < 2 {
		return fmt.Errorf("judge stage %q: need at least 2 candidate containers, found %d", stage.Name, len(names))
	}

	candidates, err := judgeCollectCandidates(ctx, exec, names, judgeBaseOrDefault(stage.Judge.Base))
	if err != nil {
		return fmt.Errorf("judge stage %q: collect candidates: %w", stage.Name, err)
	}

	diffCap := stage.Judge.MaxDiff
	if diffCap <= 0 {
		diffCap = defaultJudgeDiffCap
	}
	prompt := buildJudgePrompt(pipelineName+" / "+stage.Name, candidates, stage.Judge.Criteria, diffCap)

	judgeName := safeNameComponent(stage.Name+"-judge-"+timestamp, 96)
	if err := judgeSpawn(SpawnOpts{
		AgentType:  "claude",
		Name:       judgeName,
		Prompt:     prompt,
		Background: true,
		ReuseAuth:  true,
		AutoTrust:  true,
	}); err != nil {
		return fmt.Errorf("judge stage %q: spawn judge: %w", stage.Name, err)
	}

	judgeContainer := resolveContainerName("claude", judgeName, timestamp, nil)
	if err := waitForContainers(ctx, exec, []string{judgeContainer}); err != nil {
		return fmt.Errorf("judge stage %q: judge agent: %w", stage.Name, err)
	}
	raw := judgeReadOutput(ctx, exec, judgeContainer)

	record := judgeRecord{
		Stage:      stage.Name,
		Pipeline:   pipelineName,
		Timestamp:  timestamp,
		Criteria:   stage.Judge.Criteria,
		Candidates: names,
		AutoPR:     stage.Judge.AutoPR,
		RawOutput:  raw,
	}

	verdict, perr := parseVerdict(raw, names)
	if perr != nil {
		record.Failed = true
		record.Error = perr.Error()
		path, saveErr := persistJudgeRecord(record)
		printJudgeFailure(stage.Name, perr, path, saveErr)
		if path != "" {
			return fmt.Errorf("judge stage %q: %w (raw output saved to %s)", stage.Name, perr, path)
		}
		return fmt.Errorf("judge stage %q: %w", stage.Name, perr)
	}
	record.Verdict = verdict

	if stage.Judge.AutoPR {
		record.PRBranch = winnerPRBranch(pipelineName, stage.Name, timestamp)
		record.PRResult = runJudgeAutoPR(ctx, exec, stage, verdict, record.PRBranch)
	}

	path, err := persistJudgeRecord(record)
	if err != nil {
		return fmt.Errorf("judge stage %q: persist verdict: %w", stage.Name, err)
	}
	printJudgeVerdict(stage.Name, verdict, path, record.PRResult)
	return nil
}

// judgeCandidateNames flattens (and de-duplicates) the container names produced
// by the judge stage's dependency stages.
func judgeCandidateNames(stage fleet.PipelineStage, stageContainers map[string][]string) []string {
	seen := make(map[string]bool)
	var names []string
	for _, dep := range stage.DependsOn {
		for _, name := range stageContainers[dep] {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func collectJudgeCandidatesImpl(ctx context.Context, exec vmexec.Executor, names []string, base string) ([]JudgeCandidate, error) {
	candidates := make([]JudgeCandidate, 0, len(names))
	for _, name := range names {
		agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
		candidates = append(candidates, JudgeCandidate{
			Name:    name,
			Agent:   agentType,
			Diff:    collectCandidateDiff(ctx, exec, name, base),
			Message: strings.TrimSpace(judgeReadOutput(ctx, exec, name)),
		})
	}
	return candidates, nil
}

// collectCandidateDiff returns the full candidate change against the base branch
// for a candidate container, handling both running and stopped containers.
// Candidates are led to *commit* their work, so a plain `git diff` (unstaged
// only) would miss it — we diff against the base ref instead, which captures
// committed, staged, and unstaged changes. Failures are folded into the returned
// text so one bad candidate does not abort the whole judge.
func collectCandidateDiff(ctx context.Context, exec vmexec.Executor, name, base string) string {
	gitCmd := candidateDiffGitCmd(base)
	running, _ := docker.IsRunning(ctx, exec, name)
	var (
		out []byte
		err error
	)
	if running {
		out, err = exec.Run(ctx, workspaceExec(name, gitCmd)...)
	} else {
		out, err = runGitOnStoppedWorkspace(ctx, exec, name, gitCmd)
	}
	if err != nil {
		return fmt.Sprintf("(diff unavailable: %v)", err)
	}
	return string(out)
}

// candidateDiffGitCmd builds the shell snippet used to diff a candidate's work
// against the base branch. It prefers the remote-tracking ref (origin/<base>),
// falls back to a local <base> ref, then to HEAD, and finally to a plain working
// -tree diff. `git diff <ref>` (single arg) compares <ref> to the working tree,
// so committed + staged + unstaged changes are all included.
//
// base is embedded into a shell command, so an unsafe/empty base is treated as
// "no base ref" and only the HEAD/working-tree fallbacks are used.
func candidateDiffGitCmd(base string) string {
	if !validBranchName.MatchString(base) {
		return "git diff HEAD 2>/dev/null || git diff"
	}
	return fmt.Sprintf(
		"if git rev-parse --verify --quiet origin/%[1]s >/dev/null 2>&1; then git diff origin/%[1]s; "+
			"elif git rev-parse --verify --quiet %[1]s >/dev/null 2>&1; then git diff %[1]s; "+
			"elif git rev-parse --verify --quiet HEAD >/dev/null 2>&1; then git diff HEAD; "+
			"else git diff; fi",
		base)
}

// buildJudgePrompt assembles the strict-JSON judge prompt from the task context,
// per-candidate diffs (truncated at cap) and messages, and the criteria.
func buildJudgePrompt(taskContext string, candidates []JudgeCandidate, criteria string, diffCap int) string {
	if strings.TrimSpace(criteria) == "" {
		criteria = "overall implementation quality: correctness first, then minimal and focused diff, then clarity and maintainability"
	}

	var b strings.Builder
	b.WriteString("You are the judge in a best-of-N coding contest. Several agents independently attempted the SAME task in separate sandboxes. ")
	b.WriteString("Your job is to pick the single best candidate and justify the choice.\n\n")
	b.WriteString("Task context: ")
	b.WriteString(taskContext)
	b.WriteString("\n\nJudging criteria: ")
	b.WriteString(criteria)
	b.WriteString("\n\n")

	names := make([]string, 0, len(candidates))
	for i, c := range candidates {
		names = append(names, c.Name)
		fmt.Fprintf(&b, "===== Candidate %d =====\n", i+1)
		fmt.Fprintf(&b, "container: %s\n", c.Name)
		if c.Agent != "" {
			fmt.Fprintf(&b, "agent: %s\n", c.Agent)
		}
		msg := strings.TrimSpace(c.Message)
		if msg == "" {
			msg = "(no final message captured)"
		}
		b.WriteString("final message:\n")
		b.WriteString(truncateForPrompt(msg, diffCap))
		b.WriteString("\ndiff:\n")
		diff := strings.TrimSpace(c.Diff)
		if diff == "" {
			diff = "(empty diff — no working-tree changes)"
		}
		b.WriteString(truncateForPrompt(diff, diffCap))
		b.WriteString("\n\n")
	}

	b.WriteString("Respond with EXACTLY ONE JSON object and NOTHING else — no prose, no markdown fences. Schema:\n")
	b.WriteString(`{"winner":"<container-name>","reason":"<why this candidate wins>","summary":"<PR-description-style summary of the winning change>"}`)
	b.WriteString("\n\nThe \"winner\" MUST be exactly one of these container names: ")
	b.WriteString(strings.Join(names, ", "))
	b.WriteString(".\n")
	return b.String()
}

// truncateForPrompt caps s at cap bytes on a rune boundary, appending a note
// describing how many bytes were dropped.
func truncateForPrompt(s string, diffCap int) string {
	if diffCap <= 0 || len(s) <= diffCap {
		return s
	}
	cut := diffCap
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	dropped := len(s) - cut
	return s[:cut] + fmt.Sprintf("\n… [truncated %d bytes]", dropped)
}

func isRuneStart(b byte) bool {
	// Continuation bytes match 0b10xxxxxx; everything else starts a rune.
	return b&0xC0 != 0x80
}

// parseVerdict leniently extracts the first well-formed JSON verdict from raw
// judge output whose winner is one of the candidate container names.
func parseVerdict(raw string, candidateNames []string) (JudgeVerdict, error) {
	objects := extractJSONObjects(raw)
	if len(objects) == 0 {
		return JudgeVerdict{}, fmt.Errorf("no JSON object found in judge output")
	}
	valid := make(map[string]bool, len(candidateNames))
	for _, n := range candidateNames {
		valid[n] = true
	}
	var firstErr error
	for _, obj := range objects {
		var v JudgeVerdict
		if err := json.Unmarshal([]byte(obj), &v); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		v.Winner = strings.TrimSpace(v.Winner)
		if v.Winner == "" {
			continue
		}
		if !valid[v.Winner] {
			if firstErr == nil {
				firstErr = fmt.Errorf("winner %q is not one of the candidates", v.Winner)
			}
			continue
		}
		return v, nil
	}
	if firstErr != nil {
		return JudgeVerdict{}, firstErr
	}
	return JudgeVerdict{}, fmt.Errorf("no verdict with a valid winner found")
}

// extractJSONObjects returns every top-level balanced {...} object in s, in
// order, ignoring braces inside JSON strings. It tolerates surrounding prose
// and markdown fences.
func extractJSONObjects(s string) []string {
	var objects []string
	depth := 0
	start := -1
	inStr := false
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					objects = append(objects, s[start:i+1])
					start = -1
				}
			}
		}
	}
	return objects
}

// runJudgeAutoPR opens a PR from the winning candidate's work and returns a
// short result string for the record. It never aborts the pipeline: a PR failure
// is surfaced as a warning because the verdict itself already succeeded.
func runJudgeAutoPR(ctx context.Context, exec vmexec.Executor, stage fleet.PipelineStage, verdict JudgeVerdict, branch string) string {
	base := judgeBaseOrDefault(stage.Judge.Base)
	title := "Best-of-N winner: " + verdict.Winner
	body := verdict.Summary
	if strings.TrimSpace(verdict.Reason) != "" {
		if body != "" {
			body += "\n\n"
		}
		body += "---\nJudge reason: " + verdict.Reason
	}
	out, err := judgeCreatePR(ctx, exec, verdict.Winner, branch, base, title, body)
	if err != nil {
		fmt.Printf("⚠️  judge stage %q: auto PR failed: %v\n", stage.Name, err)
		return "error: " + err.Error()
	}
	if s := strings.TrimSpace(out); s != "" {
		fmt.Println(s)
		return s
	}
	return "created"
}

func judgeBaseOrDefault(base string) string {
	if base == "" {
		return "main"
	}
	return base
}

// winnerPRBranch derives a dedicated, sanitized head branch for the PR so we
// never push the candidate's cloned default branch (e.g. main). The timestamp
// keeps re-runs from colliding.
func winnerPRBranch(pipeline, stage, timestamp string) string {
	return "berth/judge-" + safeNameComponent(pipeline+"-"+stage+"-"+timestamp, 80)
}

// winnerPRShell is the injection-safe helper script. Untrusted values (branch,
// base, title, body) are passed as positional arguments ($1..$4), never
// interpolated into the script text. It creates a dedicated head branch from the
// candidate's committed work, pushes it (falling back to gh HTTPS credentials
// when origin is an SSH URL with no reachable agent), and opens the PR.
const winnerPRShell = `set -eu
repo_dir=$(find /workspace -mindepth 1 -maxdepth 4 -name .git -type d -exec dirname {} \; 2>/dev/null | head -1)
if [ -n "$repo_dir" ]; then cd "$repo_dir"; else cd /workspace; fi
branch="$1"; base="$2"; title="$3"; body="$4"
git checkout -B "$branch"
gh auth setup-git
if ! git push -u origin "$branch"; then
  slug=$(gh repo view --json nameWithOwner -q .nameWithOwner) || {
    echo "auto_pr: gh could not resolve the repository — auto_pr needs a candidate spawned with reuse_gh_auth (HTTPS auth). SSH-agent auth cannot reach the PR helper." >&2
    exit 20
  }
  git push "https://github.com/$slug.git" "HEAD:refs/heads/$branch" || {
    echo "auto_pr: push failed — SSH-only candidates cannot push from the PR helper; spawn candidates with reuse_gh_auth/HTTPS." >&2
    exit 21
  }
fi
gh pr create --base "$base" --head "$branch" --title "$title" --body "$body"
`

// createWinnerPRImpl opens a PR from the winning candidate WITHOUT restarting the
// candidate container. Restarting an exited candidate re-runs its agent
// entrypoint (background/fleet sessions do not gate on the started-file
// heuristic), which could mutate the workspace or race the push. Instead we run a
// short-lived helper container that mounts the winner's volumes with
// --volumes-from (carrying /workspace and the gh/auth named volumes) but
// overrides the entrypoint, so no agent runs.
//
// Auth: gh credentials come from the winner's gh auth volume, populated only when
// the candidate ran with reuse_gh_auth. SSH-agent auth cannot cross into the
// helper (the relay socket is per-container and .ssh is tmpfs), so SSH-only
// candidates need reuse_gh_auth/HTTPS for auto_pr; the helper surfaces a clear
// error otherwise. Failures keep the existing non-fatal auto-PR semantics.
func createWinnerPRImpl(ctx context.Context, exec vmexec.Executor, winner, branch, base, title, body string) (string, error) {
	if !validBranchName.MatchString(branch) {
		return "", fmt.Errorf("invalid head branch: %s", branch)
	}
	if !validBranchName.MatchString(base) {
		return "", fmt.Errorf("invalid base branch: %s", base)
	}
	image, err := winnerContainerImage(ctx, exec, winner)
	if err != nil {
		return "", err
	}

	// Hardened, short-lived helper: drop caps + no-new-privileges, reuse the
	// winner's network for egress parity, override the entrypoint so no agent
	// runs. Untrusted branch/base/title/body are passed as argv, not shell text.
	args := []string{
		"docker", "run", "--rm",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--volumes-from", winner,
	}
	if network := winnerContainerNetwork(ctx, exec, winner); network != "" && network != "default" {
		args = append(args, "--network", network)
	}
	args = append(args,
		"--entrypoint", "bash", image,
		"-c", winnerPRShell, "berth-judge-pr",
		branch, base, title, body,
	)

	out, err := exec.Run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("PR helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// winnerContainerImage returns the image the winner container was created from,
// so the helper runs the same toolchain (git + gh) with the same volume layout.
func winnerContainerImage(ctx context.Context, exec vmexec.Executor, winner string) (string, error) {
	out, err := exec.Run(ctx, "docker", "inspect", "--format", "{{.Config.Image}}", winner)
	if err != nil {
		return "", fmt.Errorf("inspect winner image: %w", err)
	}
	image := strings.TrimSpace(string(out))
	if image == "" {
		return "", fmt.Errorf("winner container %s has no image", winner)
	}
	return image, nil
}

// winnerContainerNetwork returns the winner container's network so the helper
// shares its egress path. Empty/best-effort: the helper falls back to the
// default network if this cannot be determined.
func winnerContainerNetwork(ctx context.Context, exec vmexec.Executor, winner string) string {
	out, err := exec.Run(ctx, "docker", "inspect", "--format", "{{.HostConfig.NetworkMode}}", winner)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func judgeStateDir() string {
	return filepath.Join(config.StateDir(), "judge")
}

func persistJudgeRecord(record judgeRecord) (string, error) {
	dir := judgeStateDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create judge state dir: %w", err)
	}
	base := safeNameComponent(fmt.Sprintf("%s-%s-%s", record.Pipeline, record.Stage, record.Timestamp), 120)
	if base == "pipeline" {
		base = "judge-" + record.Timestamp
	}
	path := filepath.Join(dir, base+".json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal judge verdict: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write judge verdict: %w", err)
	}
	return path, nil
}

func printJudgeVerdict(stageName string, verdict JudgeVerdict, path, prResult string) {
	fmt.Printf("\n🏆 Judge %q selected winner: %s\n", stageName, verdict.Winner)
	if strings.TrimSpace(verdict.Reason) != "" {
		fmt.Printf("   reason: %s\n", oneLine(verdict.Reason))
	}
	if strings.TrimSpace(verdict.Summary) != "" {
		fmt.Printf("   summary: %s\n", oneLine(verdict.Summary))
	}
	if prResult != "" {
		fmt.Printf("   pr: %s\n", oneLine(prResult))
	}
	if path != "" {
		fmt.Printf("   verdict saved: %s\n", path)
	}
}

func printJudgeFailure(stageName string, perr error, path string, saveErr error) {
	fmt.Printf("\n❌ Judge %q failed to produce a valid verdict: %v\n", stageName, perr)
	if saveErr != nil {
		fmt.Printf("   (could not save raw output: %v)\n", saveErr)
		return
	}
	if path != "" {
		fmt.Printf("   raw output saved: %s\n", path)
	}
}
