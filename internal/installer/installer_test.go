package installer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/emmett08/observatory-instrumentation/internal/config"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func tempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func arguments(mode, root string) []string {
	return []string{mode, "--directory", root, "--repository", "example/project", "--protocol", "study-v1", "--action-ref", testRevision, "--tool", "python3", "--", "python3", "-m", "unittest", "discover"}
}

func TestInitCreatesScopedAdoptionAndIsIdempotent(t *testing.T) {
	root := tempDir(t)
	var output bytes.Buffer
	if err := Run(arguments("init", root), &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	registry, err := config.Load(filepath.Join(root, ".observatory/config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if registry.ProtocolID != "study-v1" || len(registry.Tasks) != 0 || registry.Repositories[0].Repository != "example/project" || registry.Repositories[0].WorkflowPaths[0] != ".github/workflows/observatory.yml" {
		t.Fatalf("unexpected registry: %#v", registry)
	}
	workflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/observatory.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"emmett08/observatory-instrumentation@" + testRevision,
		"controller: unknown", "issues: read", "pull-requests: read", "persist-credentials: false",
		"if: always() && steps.evidence.outcome == 'success'",
		"--input instrumentation-config=.observatory/config.json",
		"--input task-resolution=\"$TASK_RESOLUTION\"",
		"-- 'python3' '-m' 'unittest' 'discover'",
		"${{ steps.evidence.outputs.resolution-path }}",
	} {
		if !bytes.Contains(workflow, []byte(required)) {
			t.Errorf("workflow lacks %q", required)
		}
	}
	for _, path := range []string{".github/ISSUE_TEMPLATE/observatory-task.yml", ".github/ISSUE_TEMPLATE/effort-observation.yml", "docs/observatory.md"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Error(err)
		}
	}
	before := string(workflow)
	output.Reset()
	if err := Run(arguments("init", root), &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "0 new") {
		t.Fatalf("idempotent installation changed files: %s", output.String())
	}
	after, _ := os.ReadFile(filepath.Join(root, ".github/workflows/observatory.yml"))
	if string(after) != before {
		t.Fatal("repeat installation changed workflow")
	}
}

func TestConflictPreflightLeavesEveryExistingFileUntouched(t *testing.T) {
	root := tempDir(t)
	path := filepath.Join(root, "docs/observatory.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing user guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(arguments("init", root), io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".observatory")); !os.IsNotExist(err) {
		t.Fatal("preflight conflict left partial generated files")
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != "existing user guide\n" {
		t.Fatal("installer changed user guide")
	}
}

func TestSymlinkedDirectoryAndDestinationCannotEscape(t *testing.T) {
	for _, kind := range []string{"directory", "file", "root"} {
		t.Run(kind, func(t *testing.T) {
			root, outside := tempDir(t), tempDir(t)
			if kind == "directory" {
				if err := os.Symlink(outside, filepath.Join(root, ".github")); err != nil {
					t.Fatal(err)
				}
			} else if kind == "file" {
				if err := os.MkdirAll(filepath.Join(root, ".observatory"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "untouched.json"), filepath.Join(root, ".observatory/config.json")); err != nil {
					t.Fatal(err)
				}
			} else {
				alias := filepath.Join(root, "alias")
				if err := os.Symlink(outside, alias); err != nil {
					t.Fatal(err)
				}
				root = alias
			}
			if err := Run(arguments("init", root), io.Discard, io.Discard); err == nil {
				t.Fatal("symlink was accepted")
			}
			items, _ := os.ReadDir(outside)
			if len(items) != 0 {
				t.Fatal("wrote outside target")
			}
		})
	}
}

func TestCommandArgumentsAreLiteralAndGitHubExpressionsRejected(t *testing.T) {
	root := tempDir(t)
	marker := filepath.Join(root, "must-not-exist")
	values := []string{"space separated", "a'b\"c", "$(touch " + marker + ")", "`touch " + marker + "`", "$HOME", "; touch " + marker}
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = shellQuote(value)
	}
	// Exercise the quoting with a real shell; expansions must remain data.
	got, err := exec.Command("bash", "-c", "printf '%s\\n' "+strings.Join(quoted, " ")).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(strings.Split(strings.TrimSuffix(string(got), "\n"), "\n"), values) {
		t.Fatalf("argument changed: %q", got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("command substitution was executed")
	}
	args := arguments("init", root)
	args = append(args, "${{ secrets.ANYTHING }}")
	if err := Run(args, io.Discard, io.Discard); err == nil {
		t.Fatal("GitHub expression was accepted as a literal command argument")
	}
}

func TestUnsafeInputIsRejectedBeforeWrites(t *testing.T) {
	for name, mutate := range map[string]func([]string) []string{
		"mutable action tag": func(a []string) []string { a[8] = "v1"; return a },
		"bad repository":     func(a []string) []string { a[4] = "../elsewhere"; return a },
		"newline argument":   func(a []string) []string { return append(a, "line\nbreak") },
		"missing command":    func(a []string) []string { return a[:12] },
	} {
		t.Run(name, func(t *testing.T) {
			root := tempDir(t)
			if err := Run(mutate(arguments("init", root)), io.Discard, io.Discard); err == nil {
				t.Fatal("invalid input accepted")
			}
			items, _ := os.ReadDir(root)
			if len(items) != 0 {
				t.Fatal("invalid request wrote files")
			}
		})
	}
}

type fakeCommands struct {
	root           string
	status         string
	origin         string
	remoteBranches string
	localBranches  string
	failName       string
	failPrefix     string
	calls          []string
	prBody         string
}

func (f *fakeCommands) run(directory, name string, args ...string) ([]byte, error) {
	if directory != f.root {
		return nil, errors.New("unexpected command directory")
	}
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	if f.failName == name && strings.HasPrefix(strings.Join(args, " "), f.failPrefix) {
		return nil, errors.New("simulated failure")
	}
	switch {
	case line == "git rev-parse --show-toplevel":
		return []byte(f.root + "\n"), nil
	case strings.HasPrefix(line, "git status "):
		return []byte(f.status), nil
	case line == "git remote get-url origin":
		remote := f.origin
		if remote == "" {
			remote = "https://github.com/example/project.git"
		}
		return []byte(remote + "\n"), nil
	case line == "gh api repos/example/project":
		return []byte(`{"full_name":"example/project","default_branch":"main"}`), nil
	case strings.HasPrefix(line, "git for-each-ref "):
		return []byte(f.localBranches), nil
	case strings.HasPrefix(line, "git ls-remote "):
		return []byte(f.remoteBranches), nil
	case strings.HasPrefix(line, "gh pr create "):
		for i, arg := range args {
			if arg == "--body-file" {
				body, err := os.ReadFile(args[i+1])
				if err != nil {
					return nil, err
				}
				f.prBody = string(body)
			}
		}
		return []byte("https://github.com/example/project/pull/8\n"), nil
	}
	return nil, nil
}

func TestAdoptCreatesOnlyNewBranchAndGeneratedFilesThenPR(t *testing.T) {
	root := tempDir(t)
	fake := &fakeCommands{root: root}
	var output bytes.Buffer
	if err := run(arguments("adopt", root), &output, io.Discard, fake.run); err != nil {
		t.Fatal(err)
	}
	if output.String() != "https://github.com/example/project/pull/8\n" {
		t.Fatal(output.String())
	}
	joined := strings.Join(fake.calls, "\n")
	for _, required := range []string{
		"git fetch origin main", "git switch -c observatory/adoption FETCH_HEAD",
		"git add -- .observatory/config.json .github/workflows/observatory.yml .github/ISSUE_TEMPLATE/observatory-task.yml .github/ISSUE_TEMPLATE/effort-observation.yml docs/observatory.md",
		"git push --set-upstream --force-with-lease=refs/heads/observatory/adoption: origin HEAD:refs/heads/observatory/adoption",
		"gh pr create --repo example/project --base main --head observatory/adoption",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("missing command: %s", required)
		}
	}
	if strings.Contains(joined, " merge ") || strings.Contains(joined, " add .") || strings.Contains(joined, " add -A") {
		t.Fatal("adoption attempted an unsafe broad write")
	}
	if !strings.Contains(fake.prBody, "Human estimates remain unavailable") || !strings.Contains(fake.prBody, "\n\n") {
		t.Fatalf("PR body lost provenance or newlines: %s", fake.prBody)
	}
}

func TestAdoptRefusesDirtyCheckoutMismatchedOriginAndExistingBranches(t *testing.T) {
	for _, problem := range []string{"dirty", "origin", "local branch", "remote branch"} {
		t.Run(problem, func(t *testing.T) {
			root := tempDir(t)
			fake := &fakeCommands{root: root}
			switch problem {
			case "dirty":
				fake.status = "?? private-notes.txt\n"
			case "origin":
				fake.origin = "https://github.com/someone/another.git"
			case "local branch":
				fake.localBranches = "refs/heads/observatory/adoption\n"
			case "remote branch":
				fake.remoteBranches = "abc123\trefs/heads/observatory/adoption\n"
			}
			if err := run(arguments("adopt", root), io.Discard, io.Discard, fake.run); err == nil {
				t.Fatal("unsafe adoption was accepted")
			}
			for _, call := range fake.calls {
				if strings.HasPrefix(call, "git switch") || strings.HasPrefix(call, "git push") || strings.HasPrefix(call, "gh pr create") {
					t.Fatalf("mutation after rejected preflight: %s", call)
				}
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("rejected adoption wrote files")
			}
		})
	}
}

func TestAdoptStopsAfterPushFailureWithoutCreatingPR(t *testing.T) {
	root := tempDir(t)
	fake := &fakeCommands{root: root, failName: "git", failPrefix: "push "}
	if err := run(arguments("adopt", root), io.Discard, io.Discard, fake.run); err == nil {
		t.Fatal("push failure was ignored")
	}
	for _, call := range fake.calls {
		if strings.HasPrefix(call, "gh pr create") {
			t.Fatal("PR requested after failed push")
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".observatory/config.json")); err != nil {
		t.Fatal("failed adoption should retain local reviewable files")
	}
}

func TestInitNeverInvokesNetworkOrGitCommands(t *testing.T) {
	called := false
	command := func(string, string, ...string) ([]byte, error) {
		called = true
		return nil, errors.New("unexpected invocation")
	}
	if err := run(arguments("init", tempDir(t)), io.Discard, io.Discard, command); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("local init invoked a command")
	}
}

func TestVersionedEffortFormPreservesUnknownsAndSamplingBoundary(t *testing.T) {
	root := tempDir(t)
	if err := Run(arguments("init", root), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	form, _ := os.ReadFile(filepath.Join(root, ".github/ISSUE_TEMPLATE/effort-observation.yml"))
	for _, text := range []string{"observatory.effort-observation/1", "label: Task issue URL", "label: Declared human person-minutes", "label: Measurement method", "label: Coverage", "label: Scope", "This issue never substitutes for a private sampled response", "zero only for explicitly observed no work", "Do not attach raw activity"} {
		if !bytes.Contains(form, []byte(text)) {
			t.Errorf("missing declared-evidence boundary: %q", text)
		}
	}
	if got := bytes.Count(form, []byte("        - unknown\n")); got != 3 {
		t.Errorf("method, coverage and scope must all offer unknown; got %d", got)
	}
	if got := bytes.Count(form, []byte("      default: 0\n")); got != 3 {
		t.Errorf("method, coverage and scope must default to unknown; got %d", got)
	}
}

func TestGeneratedConfigMatchesDocumentedSchema(t *testing.T) {
	root := tempDir(t)
	if err := Run(arguments("init", root), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".observatory/config.json"))
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["schema"] != config.Schema {
		t.Fatal(fmt.Sprintf("unexpected schema %v", raw["schema"]))
	}
	if tasks, ok := raw["tasks"].([]any); !ok || len(tasks) != 0 {
		t.Fatal("empty aliases must be an array")
	}
}

func TestEmptyPushLeaseCannotOverwriteExistingRemoteBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required for the adoption integration test")
	}
	root := tempDir(t)
	remote, checkout := filepath.Join(root, "remote.git"), filepath.Join(root, "checkout")
	git := func(directory string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = directory
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(root, "init", "--bare", remote)
	git(root, "init", "-b", "main", checkout)
	git(checkout, "remote", "add", "origin", remote)
	commit := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(checkout, "record.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		git(checkout, "add", "record.txt")
		git(checkout, "-c", "user.name=Installer test", "-c", "user.email=installer@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", content)
	}
	commit("first")
	git(checkout, "push", "origin", "HEAD:refs/heads/observatory/adoption")
	first := git(remote, "rev-parse", "refs/heads/observatory/adoption")
	commit("second")
	cmd := exec.Command("git", "push", "--force-with-lease=refs/heads/observatory/adoption:", "origin", "HEAD:refs/heads/observatory/adoption")
	cmd.Dir = checkout
	if err := cmd.Run(); err == nil {
		t.Fatal("create-only lease accepted an existing remote branch")
	}
	if actual := git(remote, "rev-parse", "refs/heads/observatory/adoption"); actual != first {
		t.Fatal("existing remote branch was changed")
	}
}
