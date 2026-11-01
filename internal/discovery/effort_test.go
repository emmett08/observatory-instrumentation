package discovery

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func effortFixture(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	var config, links map[string]any
	if err := json.Unmarshal([]byte(`{
"schema":"observatory.effort-config/1","protocol_id":"study-v1","frame_id":"private-window",
"frames":[{"person_id":"private-person","start":"2026-11-01T09:00:00Z","end":"2026-11-01T10:00:00Z"}],
"sources":[{"person_id":"private-person","device_id":"private-device","afk_bucket":"private-afk","context_buckets":["private-ide"]}],
"frame_complete":false,"synthetic":false,"interval_minutes":15,
"links":[{"task_id":"task-one","repository":"owner/repo","issue_number":1,"branches":["prior-branch"],"pull_requests":[3],"projects":["declared-project"],"branch_links":[{"repository":"fork/repo","branch":"prior-branch"}]}]}`), &config); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{
"schema":"observatory.discovered-effort-links/1","protocol_id":"study-v1",
"links":[{"task_id":"task-one","repository":"owner/repo","issue_number":1,"branches":["new-branch"],"pull_requests":[4],"projects":[],"branch_links":[{"repository":"fork/repo","branch":"new-branch"}],"pull_request_urls":["https://github.com/owner/repo/pull/4"]},
{"task_id":"task-two","repository":"owner/repo","issue_number":2,"branches":[],"pull_requests":[5],"projects":[]}]}`), &links); err != nil {
		t.Fatal(err)
	}
	return config, links
}

func TestEffortMergePreservesPrivateScopeAndDeclaredAliases(t *testing.T) {
	config, discovered := effortFixture(t)
	before, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := mergeEffortLinks(config, discovered)
	if err != nil {
		t.Fatal(err)
	}
	for key, original := range config {
		if key != "links" && !reflect.DeepEqual(got[key], original) {
			t.Fatalf("private configuration field changed: %s", key)
		}
	}
	links := got["links"].([]any)
	if len(links) != 2 {
		t.Fatalf("expected two linked tasks, got %d", len(links))
	}
	one := links[0].(map[string]any)
	if !reflect.DeepEqual(one["projects"], []any{"declared-project"}) || !reflect.DeepEqual(one["branches"], []any{"new-branch", "prior-branch"}) {
		t.Fatal("declared project or branch history was lost")
	}
	if !reflect.DeepEqual(one["pull_requests"], []any{json.Number("3"), json.Number("4")}) || len(one["branch_links"].([]any)) != 2 {
		t.Fatal("PR or explicit fork-branch associations were lost")
	}
	after, _ := json.Marshal(config)
	if !bytes.Equal(before, after) {
		t.Fatal("merge modified its original configuration")
	}
}

func TestEffortMergeRefusesProtocolAndIdentityConflicts(t *testing.T) {
	tests := map[string]func(map[string]any, map[string]any){
		"protocol": func(_, discovered map[string]any) { discovered["protocol_id"] = "other-study" },
		"task-rebound": func(_, discovered map[string]any) {
			discovered["links"].([]any)[0].(map[string]any)["issue_number"] = float64(99)
		},
		"issue-relabelled": func(_, discovered map[string]any) {
			discovered["links"].([]any)[0].(map[string]any)["task_id"] = "replacement-task"
		},
		"duplicate-task": func(_, discovered map[string]any) {
			links := discovered["links"].([]any)
			discovered["links"] = append(links, links[0])
		},
		"unsupported-schema": func(_, discovered map[string]any) { discovered["schema"] = "unknown/9" },
		"boolean-issue": func(_, discovered map[string]any) {
			discovered["links"].([]any)[0].(map[string]any)["issue_number"] = true
		},
		"fractional-issue": func(_, discovered map[string]any) {
			discovered["links"].([]any)[0].(map[string]any)["issue_number"] = 1.5
		},
		"injected-url": func(_, discovered map[string]any) {
			discovered["links"].([]any)[0].(map[string]any)["pull_request_urls"] = []any{"https://attacker.invalid/private"}
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			config, discovered := effortFixture(t)
			change(config, discovered)
			if _, err := mergeEffortLinks(config, discovered); err == nil {
				t.Fatal("expected conflicting discovery to be rejected")
			}
		})
	}
}

func TestEffortMergeDeduplicatesRepeatedLinks(t *testing.T) {
	config, discovered := effortFixture(t)
	first, err := mergeEffortLinks(config, discovered)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mergeEffortLinks(first, discovered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("repeated discovery changed or duplicated links")
	}
}

func writeEffortFixture(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func effortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLinkEffortCLIProtectsTemplateAndFrozenBatch(t *testing.T) {
	dir := effortTempDir(t)
	config, discovered := effortFixture(t)
	template := filepath.Join(dir, "template.json")
	writeEffortFixture(t, template, config)
	writeEffortFixture(t, filepath.Join(dir, "effort-links.json"), discovered)
	args := []string{"--template", template, "--discovery", dir, "--output", template}
	if err := runLinkEffort(args, io.Discard, io.Discard); err == nil {
		t.Fatal("template overwrite was allowed")
	}
	frozen := filepath.Join(dir, "frozen")
	if err := os.Mkdir(frozen, 0o700); err != nil {
		t.Fatal(err)
	}
	writeEffortFixture(t, filepath.Join(frozen, "frame.json"), map[string]any{})
	args[len(args)-1] = filepath.Join(frozen, "nested", "updated.json")
	if err := runLinkEffort(args, io.Discard, io.Discard); err == nil {
		t.Fatal("writing inside a frozen batch was allowed")
	}
	args[len(args)-1] = filepath.Join(dir, "fresh.json")
	var output bytes.Buffer
	if err := runLinkEffort(args, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(output.Bytes(), []byte("private-person")) || bytes.Contains(output.Bytes(), []byte("private-device")) {
		t.Fatal("CLI printed private participant or device metadata")
	}
	before, _ := os.ReadFile(args[len(args)-1])
	discovered["links"].([]any)[0].(map[string]any)["pull_requests"] = []any{float64(7)}
	writeEffortFixture(t, filepath.Join(dir, "effort-links.json"), discovered)
	if err := runLinkEffort(args, io.Discard, io.Discard); err == nil {
		t.Fatal("different existing output was overwritten")
	}
	after, _ := os.ReadFile(args[len(args)-1])
	if !bytes.Equal(before, after) {
		t.Fatal("existing output changed after refused write")
	}
}

func TestLinkEffortCLIRefusesSymlinkOutput(t *testing.T) {
	dir := effortTempDir(t)
	config, discovered := effortFixture(t)
	template := filepath.Join(dir, "template.json")
	writeEffortFixture(t, template, config)
	writeEffortFixture(t, filepath.Join(dir, "effort-links.json"), discovered)
	output := filepath.Join(dir, "alias.json")
	if err := os.Symlink(template, output); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := runLinkEffort([]string{"--template", template, "--discovery", dir, "--output", output}, io.Discard, io.Discard); err == nil {
		t.Fatal("symlink output was allowed")
	}
}
