package discovery

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// runLinkEffort prepares a new configuration. It never changes an existing
// participant batch, its predictions or its selected probability sample.
func runLinkEffort(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("link-effort", flag.ContinueOnError)
	flags.SetOutput(stderr)
	template := flags.String("template", "", "private, unfrozen effort configuration")
	discoveryDir := flags.String("discovery", "", "directory containing discovered effort links")
	output := flags.String("output", "", "new private configuration path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *template == "" || *discoveryDir == "" || *output == "" {
		return errors.New("link-effort requires --template, --discovery and --output")
	}
	inputPath, err := filepath.Abs(*template)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if inputPath == outputPath {
		return errors.New("write a new configuration; do not overwrite the template")
	}
	// Frozen files are deliberately not edited, even when the proposed values
	// would happen to be identical. A refreshed config belongs to a new batch.
	for dir := filepath.Dir(outputPath); ; dir = filepath.Dir(dir) {
		for _, name := range []string{"frame.json", "sample.json"} {
			if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
				return errors.New("output is inside a frozen effort batch; choose a new configuration directory")
			} else if !os.IsNotExist(err) {
				return errors.New("cannot inspect destination for frozen batch evidence")
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	var original, discovered map[string]any
	if err := readJSON(inputPath, &original); err != nil {
		return fmt.Errorf("read effort template: %w", err)
	}
	if err := readJSON(filepath.Join(*discoveryDir, "effort-links.json"), &discovered); err != nil {
		return fmt.Errorf("read discovered links: %w", err)
	}
	merged, err := mergeEffortLinks(original, discovered)
	if err != nil {
		return err
	}
	if err := writeJSON(outputPath, merged); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "Prepared a new private effort configuration; the core validates it before freezing.")
	return err
}

func mergeEffortLinks(original, discovered map[string]any) (map[string]any, error) {
	if original["schema"] != "observatory.effort-config/1" || discovered["schema"] != "observatory.discovered-effort-links/1" {
		return nil, errors.New("unsupported effort configuration or discovered-link schema")
	}
	protocol, ok := original["protocol_id"].(string)
	if !ok || !effortIdentifier(protocol) || discovered["protocol_id"] != protocol {
		return nil, errors.New("effort and discovery protocols must match")
	}
	oldLinks, ok := original["links"].([]any)
	if !ok {
		return nil, errors.New("effort configuration links must be a list")
	}
	newLinks, ok := discovered["links"].([]any)
	if !ok {
		return nil, errors.New("discovered links must be a list")
	}
	if len(oldLinks)+len(newLinks) > 2000 {
		return nil, errors.New("too many task links")
	}
	byTask, byIssue := map[string]map[string]any{}, map[string]string{}
	for _, source := range [][]any{oldLinks, newLinks} {
		seen := map[string]bool{}
		for _, raw := range source {
			link, err := validatedEffortLink(raw)
			if err != nil {
				return nil, err
			}
			task := link["task_id"].(string)
			if seen[task] {
				return nil, errors.New("duplicate task in effort links")
			}
			seen[task] = true
			issue := strings.ToLower(link["repository"].(string)) + "/" + link["issue_number"].(json.Number).String()
			if previous, exists := byIssue[issue]; exists && previous != task {
				return nil, errors.New("one issue is assigned conflicting task identities")
			}
			if previous, exists := byTask[task]; exists {
				previousIssue := strings.ToLower(previous["repository"].(string)) + "/" + previous["issue_number"].(json.Number).String()
				if previousIssue != issue {
					return nil, errors.New("an existing task cannot be rebound to another issue")
				}
				for _, field := range []string{"branches", "projects", "pull_requests", "branch_links", "pull_request_urls"} {
					merged, err := unionEffortValues(previous[field], link[field])
					if err != nil {
						return nil, err
					}
					if merged != nil || field == "branches" || field == "projects" || field == "pull_requests" {
						previous[field] = merged
					}
				}
			} else {
				byTask[task] = link
			}
			byIssue[issue] = task
		}
	}
	tasks := make([]string, 0, len(byTask))
	for task := range byTask {
		tasks = append(tasks, task)
	}
	sort.Strings(tasks)
	links := make([]any, 0, len(tasks))
	for _, task := range tasks {
		links = append(links, byTask[task])
	}
	result := make(map[string]any, len(original))
	for key, value := range original {
		result[key] = value
	}
	result["links"] = links
	return result, nil
}

var effortRepository = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func effortIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= 256 && !strings.ContainsFunc(value, func(r rune) bool { return r < 32 || r == 127 })
}

func effortInteger(value any) (json.Number, error) {
	var number int64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err != nil {
			return "", errors.New("invalid issue or pull-request number")
		}
		number = parsed
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed > 9007199254740991 || typed < 1 {
			return "", errors.New("invalid issue or pull-request number")
		}
		number = int64(typed)
	case int:
		number = int64(typed)
	case int64:
		number = typed
	default:
		return "", errors.New("invalid issue or pull-request number")
	}
	if number < 1 || number > 9007199254740991 {
		return "", errors.New("issue or pull-request number outside supported range")
	}
	return json.Number(strconv.FormatInt(number, 10)), nil
}

func validatedEffortLink(raw any) (map[string]any, error) {
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("task link must be an object")
	}
	allowed := map[string]bool{"task_id": true, "repository": true, "issue_number": true, "branches": true, "projects": true, "pull_requests": true, "branch_links": true, "pull_request_urls": true}
	for field := range value {
		if !allowed[field] {
			return nil, errors.New("unsupported task link field")
		}
	}
	task, ok := value["task_id"].(string)
	if !ok || !effortIdentifier(task) || task == "total" || task == "unattributed" {
		return nil, errors.New("invalid task identity")
	}
	repository, ok := value["repository"].(string)
	if !ok || !effortRepository.MatchString(repository) || len(repository) > 256 {
		return nil, errors.New("invalid task repository")
	}
	issue, err := effortInteger(value["issue_number"])
	if err != nil {
		return nil, err
	}
	link := map[string]any{"task_id": task, "repository": repository, "issue_number": issue}
	for _, field := range []string{"branches", "projects", "pull_request_urls", "pull_requests", "branch_links"} {
		rawValues, exists := value[field]
		if !exists && (field == "branch_links" || field == "pull_request_urls") {
			continue
		}
		values, ok := rawValues.([]any)
		if !ok || len(values) > 10000 {
			return nil, errors.New("invalid task link list")
		}
		checked := make([]any, 0, len(values))
		for _, rawItem := range values {
			switch field {
			case "pull_requests":
				item, err := effortInteger(rawItem)
				if err != nil {
					return nil, err
				}
				checked = append(checked, item)
			case "branch_links":
				item, ok := rawItem.(map[string]any)
				if !ok || len(item) != 2 {
					return nil, errors.New("invalid explicit repository/branch link")
				}
				repo, repoOK := item["repository"].(string)
				branch, branchOK := item["branch"].(string)
				if !repoOK || !branchOK || !effortRepository.MatchString(repo) || !effortIdentifier(branch) {
					return nil, errors.New("invalid explicit repository/branch link")
				}
				checked = append(checked, map[string]any{"repository": repo, "branch": branch})
			default:
				item, ok := rawItem.(string)
				if !ok || !effortIdentifier(item) {
					return nil, errors.New("invalid task link string")
				}
				if field == "pull_request_urls" && !regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[1-9][0-9]*$`).MatchString(item) {
					return nil, errors.New("invalid GitHub pull-request URL")
				}
				checked = append(checked, item)
			}
		}
		link[field] = checked
	}
	return link, nil
}

func unionEffortValues(left, right any) ([]any, error) {
	if left == nil && right == nil {
		return nil, nil
	}
	unique := map[string]any{}
	for _, raw := range []any{left, right} {
		if raw == nil {
			continue
		}
		values, ok := raw.([]any)
		if !ok {
			return nil, errors.New("invalid task link list")
		}
		for _, value := range values {
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			unique[string(encoded)] = value
		}
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]any, 0, len(keys))
	for _, key := range keys {
		result = append(result, unique[key])
	}
	return result, nil
}
