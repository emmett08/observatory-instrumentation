// Package config defines the reviewed study registry shared by recording and collection.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	Schema                = "observatory.instrumentation/1"
	CoreRevision          = "32420a941a97221709447f8b71d6715216c566e9"
	IntegrationRepository = "emmett08/observatory-instrumentation"
	GoVersion             = "1.26.10"
	MaxConfigBytes        = 1 << 20
)

type Repository struct {
	Repository    string   `json:"repository"`
	WorkflowPaths []string `json:"workflow_paths"`
}
type Task struct {
	TaskID      string `json:"task_id"`
	Repository  string `json:"repository"`
	IssueNumber int64  `json:"issue_number"`
}
type Discovery struct {
	LookbackDays         int `json:"lookback_days"`
	MaxRunsPerRepository int `json:"max_runs_per_repository"`
}
type Config struct {
	Schema       string       `json:"schema"`
	ProtocolID   string       `json:"protocol_id"`
	Repositories []Repository `json:"repositories"`
	Tasks        []Task       `json:"tasks"`
	Discovery    Discovery    `json:"discovery"`
}

var Identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var RepositoryName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var WorkflowPath = regexp.MustCompile(`^\.github/workflows/[A-Za-z0-9][A-Za-z0-9_.-]*\.ya?ml$`)

func (c *Config) Validate() error {
	if c.Schema != Schema || !Identifier.MatchString(c.ProtocolID) {
		return fmt.Errorf("invalid instrumentation schema or protocol ID")
	}
	if len(c.Repositories) == 0 || len(c.Repositories) > 20 {
		return fmt.Errorf("declare 1–20 repositories")
	}
	repositories := map[string]bool{}
	for _, r := range c.Repositories {
		key := strings.ToLower(r.Repository)
		if !RepositoryName.MatchString(r.Repository) || repositories[key] {
			return fmt.Errorf("invalid or duplicate repository")
		}
		repositories[key] = true
		if len(r.WorkflowPaths) == 0 || len(r.WorkflowPaths) > 50 {
			return fmt.Errorf("declare 1–50 workflow paths per repository")
		}
		seen := map[string]bool{}
		for _, p := range r.WorkflowPaths {
			if len(p) > 256 || !WorkflowPath.MatchString(p) || seen[p] {
				return fmt.Errorf("invalid or duplicate workflow path")
			}
			seen[p] = true
		}
	}
	if c.Tasks == nil || len(c.Tasks) > 100 {
		return fmt.Errorf("tasks must be an array of at most 100 aliases")
	}
	ids, issues := map[string]bool{}, map[string]bool{}
	for _, t := range c.Tasks {
		key := fmt.Sprintf("%s/%d", strings.ToLower(t.Repository), t.IssueNumber)
		if !Identifier.MatchString(t.TaskID) || t.TaskID == "total" || t.TaskID == "unattributed" || t.TaskID == "unknown" || strings.HasPrefix(t.TaskID, "github-issue-") {
			return fmt.Errorf("invalid or reserved task alias")
		}
		if !repositories[strings.ToLower(t.Repository)] || t.IssueNumber <= 0 || t.IssueNumber > 2147483647 || ids[t.TaskID] || issues[key] {
			return fmt.Errorf("invalid or conflicting task binding")
		}
		ids[t.TaskID], issues[key] = true, true
	}
	if c.Discovery.LookbackDays < 1 || c.Discovery.LookbackDays > 90 || c.Discovery.MaxRunsPerRepository < 1 || c.Discovery.MaxRunsPerRepository > 1000 {
		return fmt.Errorf("discovery must bound lookback to 1–90 days and runs to 1–1000")
	}
	return nil
}

func (c *Config) TaskID(repository string, repositoryID, issue int64) string {
	for _, t := range c.Tasks {
		if strings.EqualFold(t.Repository, repository) && t.IssueNumber == issue {
			return t.TaskID
		}
	}
	return fmt.Sprintf("github-issue-%d-%d", repositoryID, issue)
}

// UniqueJSON rejects duplicate object keys before typed decoding can discard evidence.
func UniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 50 {
			return fmt.Errorf("JSON nesting exceeds limit")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'):
			keys := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || keys[s] {
					return fmt.Errorf("duplicate or invalid JSON key")
				}
				keys[s] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			_, err = d.Token()
		case json.Delim('['):
			for d.More() {
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
		}
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse validates the exact byte sequence that a caller hashes as its registry.
func Parse(raw []byte) (*Config, error) {
	if len(raw) > MaxConfigBytes {
		return nil, fmt.Errorf("configuration exceeds size limit")
	}
	if err := UniqueJSON(raw); err != nil {
		return nil, err
	}
	var c Config
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
