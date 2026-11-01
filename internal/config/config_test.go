package config

import (
	"os"
	"path/filepath"
	"testing"
)

const valid = `{"schema":"observatory.instrumentation/1","protocol_id":"study-v1","repositories":[{"repository":"owner/repo","workflow_paths":[".github/workflows/ci.yml"]}],"tasks":[],"discovery":{"lookback_days":7,"max_runs_per_repository":100}}`

func TestStrictConfiguration(t *testing.T) {
	for _, raw := range []string{valid[:len(valid)-1] + `,"protocol_id":"other"}`, valid + ` {}`, `{"schema":"observatory.instrumentation/1"}`} {
		p := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatalf("accepted invalid registry %s", raw)
		}
	}
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(valid), 0600)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.TaskID("owner/repo", 123, 7); got != "github-issue-123-7" {
		t.Fatal(got)
	}
	c.Tasks = []Task{{TaskID: "registered-work", Repository: "Owner/Repo", IssueNumber: 7}}
	if err = c.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := c.TaskID("owner/repo", 123, 7); got != "registered-work" {
		t.Fatal(got)
	}
	c.Tasks = append(c.Tasks, Task{TaskID: "other-work", Repository: "owner/repo", IssueNumber: 7})
	if c.Validate() == nil {
		t.Fatal("accepted conflicting aliases")
	}
}

func TestUnsafeScopes(t *testing.T) {
	for _, path := range []string{"../ci.yml", ".github/workflows/../../escape.yml", "https://example/ci.yml"} {
		if WorkflowPath.MatchString(path) {
			t.Fatal(path)
		}
	}
	if UniqueJSON([]byte(`{"nested":{"x":1,"x":2}}`)) == nil {
		t.Fatal("duplicate nested key")
	}
}
