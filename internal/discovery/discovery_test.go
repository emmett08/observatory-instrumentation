package discovery

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emmett08/observatory-instrumentation/internal/config"
)

type fakeAPI struct {
	getFn   func(string) (any, error)
	graphFn func(any) (any, error)
	calls   int
}

func fakeDecode(value, target any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digest(raw), decodeJSON(raw, target)
}
func (f *fakeAPI) get(path string, target any) (string, error) {
	f.calls++
	if f.getFn == nil {
		return "", errors.New("unexpected REST call")
	}
	v, e := f.getFn(path)
	if e != nil {
		return "", e
	}
	return fakeDecode(v, target)
}
func (f *fakeAPI) graphql(request, target any) (string, error) {
	f.calls++
	if f.graphFn == nil {
		return "", errors.New("unexpected GraphQL call")
	}
	v, e := f.graphFn(request)
	if e != nil {
		return "", e
	}
	return fakeDecode(v, target)
}

func tempRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func testConfig() *config.Config {
	return &config.Config{Schema: config.Schema, ProtocolID: "study-v1", Repositories: []config.Repository{{Repository: "owner/repo", WorkflowPaths: []string{".github/workflows/observatory.yml"}}}, Tasks: []config.Task{}, Discovery: config.Discovery{LookbackDays: 7, MaxRunsPerRepository: 100}}
}
func issueNode(repo string, id, number int64) map[string]any {
	return map[string]any{"number": number, "url": fmt.Sprintf("https://github.com/%s/issues/%d", repo, number), "repository": map[string]any{"nameWithOwner": repo, "databaseId": id}}
}
func graphPage(repo string, number int64, nodes []any) map[string]any {
	return map[string]any{"data": map[string]any{"repository": map[string]any{"nameWithOwner": repo, "databaseId": int64(123), "pullRequest": map[string]any{"number": number, "url": fmt.Sprintf("https://github.com/%s/pull/%d", repo, number), "headRefName": "topic", "headRepository": map[string]any{"nameWithOwner": repo}, "closingIssuesReferences": map[string]any{"totalCount": len(nodes), "nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}}}}
}
func singleGraph(request any) (any, error) {
	number := request.(map[string]any)["variables"].(map[string]any)["number"].(int64)
	return graphPage("owner/repo", number, []any{issueNode("owner/repo", 123, 9)}), nil
}
func runNode(id int64, status string, conclusion any, prs ...int64) map[string]any {
	p := []any{}
	for _, n := range prs {
		p = append(p, map[string]any{"number": n})
	}
	return map[string]any{"id": id, "run_attempt": 1, "status": status, "conclusion": conclusion, "path": ".github/workflows/observatory.yml", "head_sha": strings.Repeat("a", 40), "created_at": "2026-11-01T10:00:00Z", "pull_requests": p}
}
func testWindow() (time.Time, time.Time) {
	return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
}
func configPath(t *testing.T, root string, cfg *config.Config) string {
	t.Helper()
	p := filepath.Join(root, "config.json")
	if err := writeJSON(p, cfg); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveLinksIssueWithoutInferringHumanController(t *testing.T) {
	root := tempRoot(t)
	cfg := testConfig()
	cp := configPath(t, root, cfg)
	event := filepath.Join(root, "event.json")
	if err := writeJSON(event, map[string]any{"number": int64(4), "repository": map[string]any{"full_name": "owner/repo"}, "pull_request": map[string]any{"number": int64(4)}, "sender": map[string]any{"login": "human-looking-account"}}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "resolution.json")
	ghOut := filepath.Join(root, "outputs")
	var stdout bytes.Buffer
	client := &fakeAPI{graphFn: singleGraph}
	err := runResolve([]string{"--config", cp, "--repository", "owner/repo", "--event-path", event, "--event-name", "pull_request", "--output", out, "--github-output", ghOut}, &stdout, io.Discard, client)
	if err != nil {
		t.Fatal(err)
	}
	var r resolution
	if err := readJSON(out, &r); err != nil {
		t.Fatal(err)
	}
	if r.TaskID != "github-issue-123-9" || r.Status != "resolved" || r.Evidence.Number != 4 || r.Source != "github_graphql_closing_issues" {
		t.Fatalf("bad resolution: %+v", r)
	}
	raw, _ := os.ReadFile(out)
	if strings.Contains(string(raw), "human-looking-account") {
		t.Fatal("unallowlisted actor data retained")
	}
	outputs, _ := os.ReadFile(ghOut)
	if !strings.Contains(string(outputs), "task-id=github-issue-123-9\n") {
		t.Fatal("missing action output")
	}
}

func TestResolveMissingAmbiguousCrossRepositoryAndFailedLinksStayUnknown(t *testing.T) {
	for _, tc := range []struct {
		name    string
		nodes   []any
		failure bool
		reason  string
	}{{"missing", []any{}, false, "no_closing_issue"}, {"ambiguous", []any{issueNode("owner/repo", 123, 9), issueNode("owner/repo", 123, 10)}, false, "multiple_closing_issues"}, {"cross-repository", []any{issueNode("owner/other", 456, 9)}, false, "cross_repository_issue"}, {"api-error", nil, true, "link_api_unavailable_or_incomplete"}} {
		t.Run(tc.name, func(t *testing.T) {
			root := tempRoot(t)
			cp := configPath(t, root, testConfig())
			event := filepath.Join(root, "event.json")
			writeJSON(event, map[string]any{"number": 4, "repository": map[string]any{"full_name": "owner/repo"}, "pull_request": map[string]any{}})
			client := &fakeAPI{graphFn: func(any) (any, error) {
				if tc.failure {
					return nil, errors.New("private auth details must not escape")
				}
				return graphPage("owner/repo", 4, tc.nodes), nil
			}}
			out := filepath.Join(root, "resolution.json")
			if err := runResolve([]string{"--config", cp, "--repository", "owner/repo", "--event-path", event, "--event-name", "pull_request", "--output", out}, io.Discard, io.Discard, client); err != nil {
				t.Fatal(err)
			}
			var r resolution
			readJSON(out, &r)
			if r.TaskID != "" || r.Status != "unknown" || r.Reason != tc.reason {
				t.Fatalf("unknown became a task: %+v", r)
			}
		})
	}
}

func TestExplicitRegisteredTaskRemainsDeclaredAndUnregisteredRejected(t *testing.T) {
	root := tempRoot(t)
	cfg := testConfig()
	cfg.Tasks = []config.Task{{TaskID: "alias", Repository: "owner/repo", IssueNumber: 9}}
	cp := configPath(t, root, cfg)
	out := filepath.Join(root, "resolution.json")
	client := &fakeAPI{}
	args := []string{"--config", cp, "--repository", "owner/repo", "--task-id", "alias", "--output", out}
	if err := runResolve(args, io.Discard, io.Discard, client); err != nil {
		t.Fatal(err)
	}
	var r resolution
	readJSON(out, &r)
	if r.Status != "declared" || r.Source != "caller_declared_registry" || client.calls != 0 {
		t.Fatal("declaration mislabelled as observation")
	}
	args[5] = "invented"
	if err := runResolve(args, io.Discard, io.Discard, client); err == nil {
		t.Fatal("unregistered explicit task accepted")
	}
}

func TestResolveRejectsSpoofedEventIdentity(t *testing.T) {
	root := tempRoot(t)
	cp := configPath(t, root, testConfig())
	event := filepath.Join(root, "event.json")
	writeJSON(event, map[string]any{"number": 4, "repository": map[string]any{"full_name": "attacker/repo"}, "pull_request": map[string]any{}})
	err := runResolve([]string{"--config", cp, "--repository", "owner/repo", "--event-path", event, "--event-name", "pull_request", "--output", filepath.Join(root, "result.json")}, io.Discard, io.Discard, &fakeAPI{})
	if err == nil {
		t.Fatal("mismatched event accepted")
	}
}

func TestDiscoveryRetainsFailedCancelledSkippedActiveAndUnknownRuns(t *testing.T) {
	nodes := []any{runNode(1, "completed", "failure", 4), runNode(2, "completed", "cancelled", 4), runNode(3, "completed", "skipped", 4), runNode(4, "in_progress", nil), runNode(5, "queued", nil, 4)}
	other := runNode(6, "completed", "success", 4)
	other["path"] = ".github/workflows/unselected.yml"
	nodes = append(nodes, other)
	client := &fakeAPI{graphFn: singleGraph, getFn: func(path string) (any, error) {
		if strings.Contains(path, "/actions/runs?") {
			return map[string]any{"total_count": len(nodes), "workflow_runs": nodes}, nil
		}
		return []any{}, nil
	}}
	since, until := testWindow()
	d, m, links := discover(testConfig(), "hash", since, until, client)
	if !d.Complete || len(d.Runs) != 5 || len(m.Tasks) != 1 || len(m.Tasks[0].Runs) != 4 || len(links.Links) != 1 {
		t.Fatalf("unexpected discovery: %+v %+v", d, m)
	}
	if d.Runs[3].Attribution != "no_pr_association" || len(d.Runs[3].TaskIDs) != 0 {
		t.Fatal("unassociated run did not remain unknown")
	}
	if m.Tasks[0].TaskID != links.Links[0].TaskID || m.Tasks[0].TaskID != "github-issue-123-9" {
		t.Fatal("receipt and effort task identities differ")
	}
	if len(d.PRs) != 1 {
		t.Fatal("same PR lookup was not reused")
	}
}

func TestDiscoveryRunLimitIsIncompleteAndKnownRunsRemain(t *testing.T) {
	cfg := testConfig()
	cfg.Discovery.MaxRunsPerRepository = 1
	client := &fakeAPI{graphFn: singleGraph, getFn: func(string) (any, error) {
		return map[string]any{"total_count": 2, "workflow_runs": []any{runNode(1, "completed", "success", 4), runNode(2, "completed", "cancelled", 4)}}, nil
	}}
	since, until := testWindow()
	d, m, _ := discover(cfg, "hash", since, until, client)
	if d.Complete || len(d.Runs) != 1 || len(m.Tasks) != 1 || d.Problems[0].Reason != "configured_run_limit_reached" {
		t.Fatalf("truncation hidden: %+v", d)
	}
}

func TestDiscoveryDuplicatePagesCannotClaimCompleteCoverage(t *testing.T) {
	page1 := []any{}
	page2 := []any{}
	for i := int64(1); i <= 100; i++ {
		page1 = append(page1, runNode(i, "completed", "success", 4))
	}
	for i := int64(91); i <= 140; i++ {
		page2 = append(page2, runNode(i, "completed", "success", 4))
	}
	cfg := testConfig()
	cfg.Discovery.MaxRunsPerRepository = 200
	client := &fakeAPI{graphFn: singleGraph, getFn: func(path string) (any, error) {
		rows := page1
		if strings.Contains(path, "page=2&") {
			rows = page2
		}
		return map[string]any{"total_count": 150, "workflow_runs": rows}, nil
	}}
	since, until := testWindow()
	d, _, _ := discover(cfg, "hash", since, until, client)
	if d.Complete || len(d.Runs) != 140 {
		t.Fatal("duplicate pagination reported complete")
	}
	found := false
	for _, p := range d.Problems {
		if p.Reason == "incomplete_run_pages" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing identities were not recorded")
	}
}

func TestPartialCommitAssociationNeverBecomesUnambiguous(t *testing.T) {
	client := &fakeAPI{graphFn: singleGraph, getFn: func(path string) (any, error) {
		if strings.Contains(path, "/actions/runs?") {
			return map[string]any{"total_count": 1, "workflow_runs": []any{runNode(1, "completed", "success")}}, nil
		}
		if strings.Contains(path, "page=2") {
			return nil, errors.New("second page unavailable")
		}
		rows := []any{}
		for i := 0; i < 100; i++ {
			rows = append(rows, map[string]any{"number": int64(4), "base": map[string]any{"repo": map[string]any{"full_name": "owner/repo"}}})
		}
		return rows, nil
	}}
	since, until := testWindow()
	d, _, _ := discover(testConfig(), "hash", since, until, client)
	if d.Complete || d.Runs[0].Attribution != "pr_association_lookup_incomplete" {
		t.Fatal("partial associations became conclusive")
	}
}

func TestDiscoveryFailureWritesCoverageWithoutFabricatedManifest(t *testing.T) {
	root := tempRoot(t)
	cp := configPath(t, root, testConfig())
	out := filepath.Join(root, "evidence")
	client := &fakeAPI{getFn: func(string) (any, error) { return nil, errors.New("unavailable") }}
	err := runDiscover([]string{"--config", cp, "--output-dir", out, "--since", "2026-11-01T00:00:00Z", "--until", "2026-11-02T00:00:00Z"}, io.Discard, io.Discard, client, time.Now())
	if err == nil {
		t.Fatal("API failure succeeded")
	}
	var d discoveryEvidence
	if err := readJSON(filepath.Join(out, "discovery.json"), &d); err != nil {
		t.Fatal(err)
	}
	if d.Complete || d.ManifestAvailable || len(d.Runs) != 0 {
		t.Fatal("API failure became plausible empty coverage")
	}
	if _, err := os.Stat(filepath.Join(out, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("invented manifest")
	}
}

func TestRetainedDiscoveryIsImmutableAndRepeatable(t *testing.T) {
	root := tempRoot(t)
	cp := configPath(t, root, testConfig())
	out := filepath.Join(root, "evidence")
	nodes := []any{runNode(1, "completed", "success", 4)}
	client := &fakeAPI{graphFn: singleGraph, getFn: func(string) (any, error) {
		return map[string]any{"total_count": len(nodes), "workflow_runs": nodes}, nil
	}}
	args := []string{"--config", cp, "--output-dir", out, "--since", "2026-11-01T00:00:00Z", "--until", "2026-11-02T00:00:00Z"}
	clock := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if err := runDiscover(args, io.Discard, io.Discard, client, clock); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(out, "discovery.json"))
	if err := runDiscover(args, io.Discard, io.Discard, client, clock.Add(time.Hour)); err == nil {
		t.Fatal("later query replaced original observation time")
	}
	nodes = []any{}
	if err := runDiscover(args, io.Discard, io.Discard, client, clock); err == nil {
		t.Fatal("different discovery overwrote evidence")
	}
	after, _ := os.ReadFile(filepath.Join(out, "discovery.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("historical discovery changed")
	}
}

func TestJSONRejectsDuplicateKeysAndAPIIdentityMismatch(t *testing.T) {
	var value any
	if err := decodeJSON([]byte(`{"task":"a","task":"b"}`), &value); err == nil {
		t.Fatal("duplicate key accepted")
	}
	client := &fakeAPI{graphFn: func(any) (any, error) {
		return graphPage("owner/other", 4, []any{issueNode("owner/other", 123, 9)}), nil
	}}
	if _, err := fetchPR(client, "owner/repo", 4); err == nil {
		t.Fatal("wrong repository response accepted")
	}
	client.graphFn = func(any) (any, error) { return graphPage("owner/repo", 4, []any{issueNode("owner/repo", 999, 9)}), nil }
	if _, err := fetchPR(client, "owner/repo", 4); err == nil {
		t.Fatal("same-name different-ID repository evidence accepted")
	}
}

func TestIssuePaginationRejectsCursorLoopsAndTotalChanges(t *testing.T) {
	client := &fakeAPI{graphFn: func(any) (any, error) {
		r := graphPage("owner/repo", 4, []any{issueNode("owner/repo", 123, 9)})
		closing := r["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)["closingIssuesReferences"].(map[string]any)
		closing["totalCount"] = 2
		closing["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "same"}
		return r, nil
	}}
	if _, err := fetchPR(client, "owner/repo", 4); err == nil {
		t.Fatal("duplicate issue pagination accepted")
	}
}
