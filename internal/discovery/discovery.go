package discovery

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/emmett08/observatory-instrumentation/internal/config"
)

// Run handles metadata-only discovery. It never downloads or executes subject code.
func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected resolve, discover or link-effort")
	}
	switch args[0] {
	case "resolve":
		return runResolve(args[1:], stdout, stderr, &githubAPI{})
	case "discover":
		return runDiscover(args[1:], stdout, stderr, &githubAPI{}, time.Now().UTC())
	case "link-effort":
		return runLinkEffort(args[1:], stdout, stderr)
	default:
		return errors.New("unknown discovery command")
	}
}

type resolution struct {
	Schema       string        `json:"schema"`
	RecordedAt   string        `json:"recorded_at"`
	ProtocolID   string        `json:"protocol_id"`
	TaskID       string        `json:"task_id"`
	Repository   string        `json:"repository"`
	Status       string        `json:"status"`
	Reason       string        `json:"reason"`
	ConfigSHA256 string        `json:"config_sha256"`
	Source       string        `json:"source"`
	Evidence     *pullEvidence `json:"evidence,omitempty"`
}

func loadConfig(path string) (*config.Config, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, config.MaxConfigBytes+1))
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	return cfg, digest(raw), nil
}

func allowedRepo(cfg *config.Config, name string) bool {
	for _, r := range cfg.Repositories {
		if strings.EqualFold(r.Repository, name) {
			return true
		}
	}
	return false
}

func runResolve(args []string, stdout, stderr io.Writer, client api) error {
	f := flag.NewFlagSet("resolve", flag.ContinueOnError)
	f.SetOutput(stderr)
	path := f.String("config", "", "instrumentation configuration")
	repo := f.String("repository", "", "GitHub owner/repository")
	event := f.String("event-path", "", "GitHub event JSON")
	eventName := f.String("event-name", "", "GitHub event name")
	out := f.String("output", "", "new resolution JSON")
	ghOut := f.String("github-output", "", "GitHub step output file")
	explicit := f.String("task-id", "", "explicit registered task alias")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *path == "" || *repo == "" || *out == "" {
		return errors.New("resolve requires --config, --repository and --output")
	}
	cfg, hash, err := loadConfig(*path)
	if err != nil {
		return err
	}
	if !allowedRepo(cfg, *repo) {
		return errors.New("repository is not registered in configuration")
	}
	r := resolution{Schema: "observatory.task-resolution/1", RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), ProtocolID: cfg.ProtocolID, Repository: *repo, ConfigSHA256: hash, Status: "unknown", Reason: "event_has_no_unambiguous_task", Source: "unavailable"}
	if *explicit != "" {
		matched := false
		for _, task := range cfg.Tasks {
			if task.TaskID == *explicit && strings.EqualFold(task.Repository, *repo) {
				matched = true
			}
		}
		if !matched {
			return errors.New("explicit task must be registered for this repository")
		}
		r.TaskID = *explicit
		r.Status = "declared"
		r.Reason = "registered_caller_declaration"
		r.Source = "caller_declared_registry"
	} else if *eventName == "pull_request" {
		if *event == "" {
			return errors.New("pull_request resolution requires --event-path")
		}
		var e struct {
			Number     int64 `json:"number"`
			Repository struct {
				Name string `json:"full_name"`
			} `json:"repository"`
			PR *struct {
				Number int64 `json:"number"`
			} `json:"pull_request"`
		}
		if err := readJSON(*event, &e); err != nil {
			return fmt.Errorf("invalid GitHub event: %w", err)
		}
		if !strings.EqualFold(e.Repository.Name, *repo) || e.PR == nil || e.Number <= 0 || (e.PR.Number != 0 && e.PR.Number != e.Number) {
			return errors.New("GitHub event repository/PR identity mismatch")
		}
		pr, err := fetchPR(client, *repo, e.Number)
		if err != nil {
			r.Reason = "link_api_unavailable_or_incomplete"
		} else {
			r.Evidence = &pr
			r.Source = pr.Source
			r.TaskID, r.Reason, _ = issueTask(cfg, *repo, pr)
			if r.TaskID != "" {
				r.Status = "resolved"
			}
		}
	}
	if err := writeJSON(*out, r); err != nil {
		return err
	}
	if *ghOut != "" {
		absolute, err := filepath.Abs(*out)
		if err != nil {
			return err
		}
		if strings.ContainsAny(absolute, "\r\n") {
			return errors.New("resolution path cannot contain newlines")
		}
		file, err := os.OpenFile(*ghOut, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(file, "protocol-id=%s\ntask-id=%s\nresolution-path=%s\n", r.ProtocolID, r.TaskID, absolute)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	_, err = fmt.Fprintf(stdout, "Task attribution: %s (%s). GitHub account identity does not identify the human/controller.\n", r.Status, r.Reason)
	return err
}

type problem struct {
	Repository string `json:"repository"`
	Scope      string `json:"scope"`
	Reason     string `json:"reason"`
}
type run struct {
	Repository        string   `json:"repository"`
	ID                int64    `json:"run_id"`
	Attempt           int64    `json:"run_attempt"`
	Status            string   `json:"status"`
	Conclusion        *string  `json:"conclusion"`
	Path              string   `json:"path"`
	SHA               string   `json:"head_sha"`
	CreatedAt         string   `json:"created_at"`
	PRs               []int64  `json:"pull_requests"`
	TaskIDs           []string `json:"task_ids"`
	Attribution       string   `json:"attribution_status"`
	AssociationHashes []string `json:"association_response_sha256"`
}
type discoveryEvidence struct {
	Schema            string         `json:"schema"`
	QueriedAt         string         `json:"queried_at"`
	ProtocolID        string         `json:"protocol_id"`
	ConfigSHA256      string         `json:"config_sha256"`
	Since             string         `json:"since"`
	Until             string         `json:"until"`
	Complete          bool           `json:"complete"`
	Problems          []problem      `json:"problems"`
	Runs              []run          `json:"selected_runs"`
	ManifestAvailable bool           `json:"manifest_available"`
	PRs               []pullEvidence `json:"pull_request_links"`
	RunPageHashes     []string       `json:"run_page_response_sha256"`
	Limitations       []string       `json:"limitations"`
}
type manifestTask struct {
	TaskID     string  `json:"task_id"`
	Repository string  `json:"repository"`
	Issue      int64   `json:"issue_number"`
	PRs        []int64 `json:"pull_requests"`
	Runs       []int64 `json:"run_ids"`
}
type manifest struct {
	Schema     string         `json:"schema"`
	ProtocolID string         `json:"protocol_id"`
	Tasks      []manifestTask `json:"tasks"`
}
type effortLink struct {
	TaskID      string              `json:"task_id"`
	Repository  string              `json:"repository"`
	Issue       int64               `json:"issue_number"`
	PRs         []int64             `json:"pull_requests"`
	Branches    []string            `json:"branches"`
	Projects    []string            `json:"projects"`
	BranchLinks []map[string]string `json:"branch_links,omitempty"`
}
type effortLinks struct {
	Schema     string       `json:"schema"`
	ProtocolID string       `json:"protocol_id"`
	Links      []effortLink `json:"links"`
}

func appendInt(xs []int64, v int64) []int64 {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
func appendString(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

func discover(cfg *config.Config, hash string, since, until time.Time, client api) (discoveryEvidence, manifest, effortLinks) {
	d := discoveryEvidence{Schema: "observatory.discovery/1", ProtocolID: cfg.ProtocolID, ConfigSHA256: hash, Since: since.Format(time.RFC3339), Until: until.Format(time.RFC3339), Complete: true, Problems: []problem{}, Runs: []run{}, PRs: []pullEvidence{}, RunPageHashes: []string{}, Limitations: []string{
		"Discovery covers only configured repositories, workflow paths and the declared creation-time window; old runs rerun outside that window require a wider window or explicit collection.",
		"Issue and PR relationships are observations at query time, not proof of historical human attention, acceptance or truthful receipt declarations.",
		"Only one same-repository closing issue supplies automatic invocation attribution; ambiguous, missing and cross-repository links remain unknown.",
		"Failed, cancelled, skipped and active executions are retained. Missing uploads do not establish zero tool use.",
	}}
	m := manifest{Schema: "observatory.linked-pilot/1", ProtocolID: cfg.ProtocolID, Tasks: []manifestTask{}}
	el := effortLinks{Schema: "observatory.discovered-effort-links/1", ProtocolID: cfg.ProtocolID, Links: []effortLink{}}
	tasks := map[string]*manifestTask{}
	links := map[string]*effortLink{}
	prCache := map[string]pullEvidence{}
	failedPR := map[string]bool{}
	addProblem := func(repo, scope, reason string) {
		d.Complete = false
		d.Problems = append(d.Problems, problem{repo, scope, reason})
	}
	register := func(id, repo string, issueNumber, runID, prNumber int64, pr *pullEvidence) {
		if tasks[id] == nil {
			tasks[id] = &manifestTask{id, repo, issueNumber, []int64{}, []int64{}}
			links[id] = &effortLink{TaskID: id, Repository: repo, Issue: issueNumber, PRs: []int64{}, Branches: []string{}, Projects: []string{}}
		}
		task, link := tasks[id], links[id]
		task.Runs = appendInt(task.Runs, runID)
		if prNumber > 0 {
			task.PRs = appendInt(task.PRs, prNumber)
			link.PRs = appendInt(link.PRs, prNumber)
		}
		if pr != nil && pr.HeadBranch != "" && pr.HeadRepository != "" {
			if strings.EqualFold(pr.HeadRepository, repo) {
				link.Branches = appendString(link.Branches, pr.HeadBranch)
			} else {
				entry := map[string]string{"repository": pr.HeadRepository, "branch": pr.HeadBranch}
				exists := false
				for _, v := range link.BranchLinks {
					if v["repository"] == entry["repository"] && v["branch"] == entry["branch"] {
						exists = true
					}
				}
				if !exists {
					link.BranchLinks = append(link.BranchLinks, entry)
				}
			}
		}
	}
	for _, repository := range cfg.Repositories {
		repo := repository.Repository
		paths := map[string]bool{}
		for _, p := range repository.WorkflowPaths {
			paths[p] = true
		}
		seen := map[int64]bool{}
		expectedTotal := -1
		selected := 0
		finished := false
		for page := 1; page <= 10; page++ {
			var response struct {
				Total int `json:"total_count"`
				Runs  []struct {
					ID         int64   `json:"id"`
					Attempt    int64   `json:"run_attempt"`
					Status     string  `json:"status"`
					Conclusion *string `json:"conclusion"`
					Path       string  `json:"path"`
					SHA        string  `json:"head_sha"`
					CreatedAt  string  `json:"created_at"`
					PRs        []struct {
						Number int64 `json:"number"`
					} `json:"pull_requests"`
				} `json:"workflow_runs"`
			}
			created := since.Format(time.RFC3339) + ".." + until.Format(time.RFC3339)
			endpoint := fmt.Sprintf("repos/%s/actions/runs?per_page=100&page=%d&created=%s", repo, page, url.QueryEscape(created))
			pageHash, err := client.get(endpoint, &response)
			if err != nil {
				addProblem(repo, "workflow_runs", "api_unavailable_or_incomplete")
				finished = true
				break
			}
			d.RunPageHashes = append(d.RunPageHashes, pageHash)
			if response.Total < 0 || len(response.Runs) > 100 {
				addProblem(repo, "workflow_runs", "invalid_page")
				finished = true
				break
			}
			if expectedTotal < 0 {
				expectedTotal = response.Total
			} else if expectedTotal != response.Total {
				addProblem(repo, "workflow_runs", "run_count_changed_during_pagination")
			}
			for _, raw := range response.Runs {
				if raw.ID <= 0 {
					addProblem(repo, "workflow_runs", "invalid_run_identity")
					continue
				}
				if seen[raw.ID] {
					addProblem(repo, "workflow_runs", "duplicate_run_during_pagination")
					continue
				}
				seen[raw.ID] = true
				if !paths[raw.Path] {
					continue
				}
				createdAt, err := time.Parse(time.RFC3339, raw.CreatedAt)
				if err != nil || raw.ID <= 0 || raw.Attempt <= 0 || len(raw.Status) > 64 || !shaPattern.MatchString(raw.SHA) {
					addProblem(repo, "workflow_runs", "invalid_run_identity")
					continue
				}
				if createdAt.Before(since) || createdAt.After(until) {
					addProblem(repo, fmt.Sprint(raw.ID), "run_outside_requested_window")
					continue
				}
				if selected >= cfg.Discovery.MaxRunsPerRepository {
					addProblem(repo, "workflow_runs", "configured_run_limit_reached")
					finished = true
					break
				}
				selected++
				r := run{Repository: repo, ID: raw.ID, Attempt: raw.Attempt, Status: raw.Status, Conclusion: raw.Conclusion, Path: raw.Path, SHA: raw.SHA, CreatedAt: raw.CreatedAt, PRs: []int64{}, TaskIDs: []string{}, Attribution: "unknown", AssociationHashes: []string{}}
				associationsComplete := true
				for _, p := range raw.PRs {
					if p.Number <= 0 {
						addProblem(repo, fmt.Sprint(raw.ID), "invalid_run_pr_identity")
						associationsComplete = false
						continue
					}
					r.PRs = appendInt(r.PRs, p.Number)
				}
				if len(r.PRs) == 0 {
					ps, hashes, err := commitPRs(client, repo, raw.SHA)
					r.PRs = ps
					r.AssociationHashes = hashes
					if err != nil {
						addProblem(repo, fmt.Sprint(raw.ID), "commit_pr_lookup_incomplete")
						associationsComplete = false
					}
				}
				// Registered aliases may appear in manually dispatched receipts. Their
				// relation remains a configuration declaration, never an API claim.
				for _, t := range cfg.Tasks {
					if strings.EqualFold(t.Repository, repo) {
						register(t.TaskID, repo, t.IssueNumber, raw.ID, 0, nil)
					}
				}
				if len(r.PRs) > 50 {
					addProblem(repo, fmt.Sprint(raw.ID), "run_pr_limit_reached")
					r.Attribution = "too_many_pr_associations"
					d.Runs = append(d.Runs, r)
					continue
				}
				allResolved := associationsComplete && len(r.PRs) > 0
				for _, number := range r.PRs {
					key := fmt.Sprintf("%s#%d", strings.ToLower(repo), number)
					pr, ok := prCache[key]
					if !ok && !failedPR[key] {
						pr, err = fetchPR(client, repo, number)
						if err != nil {
							failedPR[key] = true
							addProblem(repo, key, "closing_issue_lookup_incomplete")
						} else {
							prCache[key] = pr
							d.PRs = append(d.PRs, pr)
						}
					}
					if failedPR[key] {
						allResolved = false
						continue
					}
					id, reason, item := issueTask(cfg, repo, pr)
					if id == "" {
						allResolved = false
						r.Attribution = reason
						continue
					}
					r.TaskIDs = appendString(r.TaskIDs, id)
					register(id, repo, item.Number, raw.ID, number, &pr)
				}
				if !associationsComplete {
					r.Attribution = "pr_association_lookup_incomplete"
				} else if allResolved && len(r.TaskIDs) == 1 {
					r.Attribution = "unambiguous_closing_issue"
				} else if len(r.TaskIDs) > 1 {
					r.Attribution = "multiple_task_associations"
				} else if len(r.PRs) == 0 {
					r.Attribution = "no_pr_association"
				}
				sort.Slice(r.PRs, func(i, j int) bool { return r.PRs[i] < r.PRs[j] })
				sort.Strings(r.TaskIDs)
				d.Runs = append(d.Runs, r)
			}
			if finished {
				break
			}
			if len(response.Runs) < 100 {
				if len(seen) != expectedTotal {
					addProblem(repo, "workflow_runs", "incomplete_run_pages")
				}
				finished = true
				break
			}
		}
		if !finished {
			addProblem(repo, "workflow_runs", "github_1000_run_search_limit_reached")
		}
	}
	keys := []string{}
	for key := range tasks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t, l := tasks[key], links[key]
		sort.Slice(t.PRs, func(i, j int) bool { return t.PRs[i] < t.PRs[j] })
		sort.Slice(t.Runs, func(i, j int) bool { return t.Runs[i] < t.Runs[j] })
		sort.Slice(l.PRs, func(i, j int) bool { return l.PRs[i] < l.PRs[j] })
		sort.Strings(l.Branches)
		m.Tasks = append(m.Tasks, *t)
		el.Links = append(el.Links, *l)
	}
	if len(m.Tasks) > 100 {
		addProblem("", "tasks", "core_manifest_task_limit_reached")
		m.Tasks = nil
		el.Links = []effortLink{}
	}
	sort.Slice(d.Runs, func(i, j int) bool {
		if d.Runs[i].Repository != d.Runs[j].Repository {
			return d.Runs[i].Repository < d.Runs[j].Repository
		}
		return d.Runs[i].ID < d.Runs[j].ID
	})
	sort.Slice(d.PRs, func(i, j int) bool {
		if d.PRs[i].Repository != d.PRs[j].Repository {
			return d.PRs[i].Repository < d.PRs[j].Repository
		}
		return d.PRs[i].Number < d.PRs[j].Number
	})
	d.ManifestAvailable = len(m.Tasks) > 0
	return d, m, el
}

func runDiscover(args []string, stdout, stderr io.Writer, client api, clock time.Time) error {
	f := flag.NewFlagSet("discover", flag.ContinueOnError)
	f.SetOutput(stderr)
	path := f.String("config", "", "instrumentation configuration")
	out := f.String("output-dir", "", "new evidence directory")
	start := f.String("since", "", "UTC creation-time lower bound")
	end := f.String("until", "", "UTC creation-time upper bound")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *path == "" || *out == "" {
		return errors.New("discover requires --config and --output-dir")
	}
	cfg, hash, err := loadConfig(*path)
	if err != nil {
		return err
	}
	until := clock.UTC().Truncate(time.Second)
	if *end != "" {
		until, err = parseUTC(*end)
		if err != nil {
			return err
		}
	}
	since := until.Add(-time.Duration(cfg.Discovery.LookbackDays) * 24 * time.Hour)
	if *start != "" {
		since, err = parseUTC(*start)
		if err != nil {
			return err
		}
	}
	if !since.Before(until) || until.Sub(since) > 90*24*time.Hour {
		return errors.New("discovery window must be positive and at most 90 days")
	}
	d, m, links := discover(cfg, hash, since, until, client)
	d.QueriedAt = clock.UTC().Format(time.RFC3339Nano)
	// Reject any differing existing evidence before writing any new files.
	prospective := map[string]any{"effort-links.json": links, "discovery.json": d}
	if d.ManifestAvailable {
		prospective["manifest.json"] = m
	}
	if err := preflightOutputs(*out, prospective); err != nil {
		return err
	}
	// Write the final coverage document last, so a partial filesystem write never
	// looks like a complete retained discovery. Existing different output fails.
	if d.ManifestAvailable {
		if err := writeJSON(filepath.Join(*out, "manifest.json"), m); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(*out, "effort-links.json"), links); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(*out, "discovery.json"), d); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Retained %d workflow runs and %d task bindings; coverage complete: %t.\n", len(d.Runs), len(m.Tasks), d.Complete)
	if !d.Complete {
		return errors.New("discovery is incomplete; retained coverage explains limits and failures")
	}
	return nil
}

func parseUTC(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil || !strings.HasSuffix(value, "Z") {
		return time.Time{}, errors.New("time bounds must use UTC RFC3339 ending in Z")
	}
	return t, nil
}
