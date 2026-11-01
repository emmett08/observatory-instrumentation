// Package installer prepares a bounded, reviewable repository adoption change.
package installer

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/emmett08/observatory-instrumentation/internal/config"
)

//go:embed templates/*.tmpl
var templates embed.FS

var shaPattern = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,199}$`)

type options struct {
	Directory, Repository, Protocol, ActionRef, Tool, ToolVersion, Branch string
	Command                                                               []string
}

type generatedFile struct {
	Path string
	Data []byte
}

type commandRunner func(directory, name string, arguments ...string) ([]byte, error)

// Run handles init (local files only) and adopt (explicit branch, push and PR).
func Run(args []string, stdout, stderr io.Writer) error {
	return run(args, stdout, stderr, execute)
}

func run(args []string, stdout, stderr io.Writer, command commandRunner) error {
	if len(args) == 0 || (args[0] != "init" && args[0] != "adopt") {
		return errors.New("installer requires init or adopt")
	}
	mode := args[0]
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var o options
	flags.StringVar(&o.Directory, "directory", ".", "existing repository directory")
	flags.StringVar(&o.Repository, "repository", "", "GitHub repository owner/name")
	flags.StringVar(&o.Protocol, "protocol", "", "declared study protocol ID")
	flags.StringVar(&o.ActionRef, "action-ref", "", "reviewed full 40-character instrumentation commit")
	flags.StringVar(&o.Tool, "tool", "", "selected tool name")
	flags.StringVar(&o.ToolVersion, "tool-version", "unknown", "declared tool version, or unknown")
	flags.StringVar(&o.Branch, "branch", "observatory/adoption", "new adoption branch (adopt only)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	o.Command = flags.Args()
	if err := validateOptions(&o); err != nil {
		return err
	}
	files, err := render(o)
	if err != nil {
		return err
	}
	if mode == "adopt" {
		return adopt(o, files, stdout, command)
	}
	created, err := install(o.Directory, files)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Prepared %d adoption files (%d new); review the generated workflow before running it.\n", len(files), len(created))
	return err
}

func validateOptions(o *options) error {
	if !repositoryPattern.MatchString(o.Repository) || strings.Contains(o.Repository, "..") {
		return errors.New("--repository must be a GitHub owner/name")
	}
	if !identifierPattern.MatchString(o.Protocol) {
		return errors.New("--protocol must be a nonempty study identifier")
	}
	if !shaPattern.MatchString(o.ActionRef) {
		return errors.New("--action-ref must be a reviewed full 40-character commit SHA")
	}
	o.ActionRef = strings.ToLower(o.ActionRef)
	if !identifierPattern.MatchString(o.Tool) {
		return errors.New("--tool must be a nonempty tool identifier")
	}
	if len(o.ToolVersion) == 0 || len(o.ToolVersion) > 256 {
		return errors.New("--tool-version must be a bounded nonempty declaration")
	}
	if len(o.Command) == 0 || o.Command[0] == "" || len(o.Command) > 200 {
		return errors.New("provide the selected command after --")
	}
	for _, value := range append(append([]string{}, o.Command...), o.ToolVersion) {
		if len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") || strings.Contains(value, "${{") {
			return errors.New("command arguments and version must be single-line literals without GitHub expressions")
		}
	}
	if o.Branch == "" || len(o.Branch) > 200 || strings.ContainsAny(o.Branch, "\x00\r\n") || strings.HasPrefix(o.Branch, "-") {
		return errors.New("invalid adoption branch")
	}
	root, err := filepath.Abs(o.Directory)
	if err != nil {
		return errors.New("invalid target directory")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("target must be an existing real directory, not a symlink")
	}
	// Resolve ancestors once so all subsequent destination checks use one root.
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return errors.New("cannot resolve target directory")
	}
	o.Directory = root
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func render(o options) ([]generatedFile, error) {
	registry := config.Config{
		Schema: config.Schema, ProtocolID: o.Protocol,
		Repositories: []config.Repository{{Repository: o.Repository, WorkflowPaths: []string{".github/workflows/observatory.yml"}}},
		Tasks:        []config.Task{},
		Discovery:    config.Discovery{LookbackDays: 7, MaxRunsPerRepository: 100},
	}
	if err := registry.Validate(); err != nil {
		return nil, fmt.Errorf("invalid generated study configuration: %w", err)
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return nil, err
	}
	files := []generatedFile{{".observatory/config.json", append(data, '\n')}}
	command := make([]string, len(o.Command))
	for i, arg := range o.Command {
		command[i] = shellQuote(arg)
	}
	values := struct {
		Protocol, Repository, ActionRef, Tool, ToolVersion, Command, IssueExample string
	}{o.Protocol, o.Repository, o.ActionRef, o.Tool, o.ToolVersion, strings.Join(command, " "), "https://github.com/" + o.Repository + "/issues/<number>"}
	for _, item := range []struct{ Template, Path string }{
		{"workflow.yml.tmpl", ".github/workflows/observatory.yml"},
		{"task.yml.tmpl", ".github/ISSUE_TEMPLATE/observatory-task.yml"},
		{"effort.yml.tmpl", ".github/ISSUE_TEMPLATE/effort-observation.yml"},
		{"adoption.md.tmpl", "docs/observatory.md"},
	} {
		source, err := templates.ReadFile("templates/" + item.Template)
		if err != nil {
			return nil, err
		}
		tmpl, err := template.New(item.Template).Delims("[[", "]]").Funcs(template.FuncMap{"shell": shellQuote, "yaml": strconv.Quote}).Parse(string(source))
		if err != nil {
			return nil, err
		}
		var output bytes.Buffer
		if err := tmpl.Execute(&output, values); err != nil {
			return nil, err
		}
		files = append(files, generatedFile{item.Path, output.Bytes()})
	}
	return files, nil
}

// destination checks every component under root without following symlinks.
// Installation assumes no concurrent untrusted process modifies the checkout.
func destination(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || relative == "." || filepath.Clean(relative) != relative {
		return "", errors.New("invalid generated destination")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	current := root
	for i, part := range parts {
		if part == ".." || part == "" {
			return "", errors.New("generated destination escapes target")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("cannot inspect destination %s", relative)
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return "", fmt.Errorf("destination %s contains a symlink or incompatible file", relative)
		}
	}
	return current, nil
}

func preflight(root string, files []generatedFile) ([]generatedFile, error) {
	var missing []generatedFile
	for _, file := range files {
		path, err := destination(root, file.Path)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			missing = append(missing, file)
			continue
		}
		if err != nil || info.Size() > 1024*1024 {
			return nil, fmt.Errorf("cannot safely inspect existing %s", file.Path)
		}
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, file.Data) {
			return nil, fmt.Errorf("refusing to overwrite existing %s; review and adapt it manually", file.Path)
		}
	}
	return missing, nil
}

func install(root string, files []generatedFile) ([]string, error) {
	missing, err := preflight(root, files)
	if err != nil {
		return nil, err
	}
	var created []string
	rollback := func() {
		for _, path := range created {
			_ = os.Remove(filepath.Join(root, path))
		}
	}
	for _, file := range missing {
		path, err := destination(root, file.Path)
		if err != nil {
			rollback()
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			rollback()
			return nil, fmt.Errorf("cannot create adoption directory for %s", file.Path)
		}
		// Publish a completed file with an atomic hard link. Link fails if the
		// destination appeared after preflight, so existing files are untouched.
		handle, err := os.CreateTemp(filepath.Dir(path), ".observatory-install-*")
		if err != nil {
			rollback()
			return nil, fmt.Errorf("cannot stage %s", file.Path)
		}
		temporary := handle.Name()
		_, writeErr := handle.Write(file.Data)
		syncErr := handle.Sync()
		modeErr := handle.Chmod(0o644)
		closeErr := handle.Close()
		if writeErr != nil || syncErr != nil || modeErr != nil || closeErr != nil {
			_ = os.Remove(temporary)
			rollback()
			return nil, fmt.Errorf("cannot finish writing %s", file.Path)
		}
		if err := os.Link(temporary, path); err != nil {
			_ = os.Remove(temporary)
			rollback()
			return nil, fmt.Errorf("cannot exclusively publish %s", file.Path)
		}
		created = append(created, file.Path)
		_ = os.Remove(temporary)
	}
	return created, nil
}

func execute(directory, name string, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, arguments...)
	cmd.Dir = directory
	var output bytes.Buffer
	cmd.Stdout = &output
	// No remote URL, credential-bearing stderr or private paths are echoed.
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s command failed; check local authentication and repository access", name)
	}
	if output.Len() > 1024*1024 {
		return nil, fmt.Errorf("%s output exceeded the supported limit", name)
	}
	return output.Bytes(), nil
}

func originMatches(remote, repository string) bool {
	remote = strings.TrimSpace(remote)
	var name string
	if strings.HasPrefix(remote, "https://github.com/") {
		name = strings.TrimPrefix(remote, "https://github.com/")
	} else if strings.HasPrefix(remote, "git@github.com:") {
		name = strings.TrimPrefix(remote, "git@github.com:")
	} else if strings.HasPrefix(remote, "ssh://git@github.com/") {
		name = strings.TrimPrefix(remote, "ssh://git@github.com/")
	} else {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(name, ".git"), repository)
}

func adopt(o options, files []generatedFile, stdout io.Writer, command commandRunner) error {
	call := func(name string, args ...string) ([]byte, error) { return command(o.Directory, name, args...) }
	root, err := call("git", "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(strings.TrimSpace(string(root))) != o.Directory {
		return errors.New("adopt requires the root of an existing Git checkout")
	}
	status, err := call("git", "status", "--porcelain", "--untracked-files=all")
	if err != nil || len(bytes.TrimSpace(status)) != 0 {
		return errors.New("adopt requires a clean checkout, including untracked files")
	}
	remote, err := call("git", "remote", "get-url", "origin")
	if err != nil || !originMatches(string(remote), o.Repository) {
		return errors.New("origin must identify the declared GitHub repository")
	}
	if _, err := call("git", "check-ref-format", "--branch", o.Branch); err != nil {
		return errors.New("invalid adoption branch")
	}
	// Inspect the default branch before any checkout mutation.
	metadata, err := call("gh", "api", "repos/"+o.Repository)
	if err != nil {
		return err
	}
	var repo struct {
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(metadata, &repo); err != nil || !strings.EqualFold(repo.FullName, o.Repository) || repo.DefaultBranch == "" {
		return errors.New("GitHub returned invalid repository metadata")
	}
	if _, err := call("git", "check-ref-format", "--branch", repo.DefaultBranch); err != nil {
		return errors.New("GitHub returned an invalid default branch")
	}
	if o.Branch == repo.DefaultBranch {
		return errors.New("adoption branch must differ from the default branch")
	}
	refs, err := call("git", "for-each-ref", "--format=%(refname)", "refs/heads/"+o.Branch, "refs/remotes/origin/"+o.Branch)
	if err != nil || len(bytes.TrimSpace(refs)) != 0 {
		return errors.New("adoption branch already exists or could not be checked")
	}
	remoteRefs, err := call("git", "ls-remote", "--heads", "origin", "refs/heads/"+o.Branch)
	if err != nil || len(bytes.TrimSpace(remoteRefs)) != 0 {
		return errors.New("remote adoption branch already exists or could not be checked")
	}
	if _, err := preflight(o.Directory, files); err != nil {
		return err
	}
	if _, err := call("git", "fetch", "origin", repo.DefaultBranch); err != nil {
		return err
	}
	if _, err := call("git", "switch", "-c", o.Branch, "FETCH_HEAD"); err != nil {
		return err
	}
	created, err := install(o.Directory, files)
	if err != nil {
		return fmt.Errorf("adoption branch retained for review: %w", err)
	}
	if len(created) == 0 {
		return errors.New("adoption files already match; no PR was created")
	}
	add := []string{"add", "--"}
	add = append(add, created...)
	if _, err := call("git", add...); err != nil {
		return err
	}
	if _, err := call("git", "commit", "-m", "Add Observatory execution and effort linkage"); err != nil {
		return err
	}
	// An empty expected lease permits only creation; even a concurrent remote
	// branch cannot be overwritten or advanced by this adoption command.
	if _, err := call("git", "push", "--set-upstream", "--force-with-lease=refs/heads/"+o.Branch+":", "origin", "HEAD:refs/heads/"+o.Branch); err != nil {
		return err
	}
	body := "Adds a pinned Observatory action, an explicit selected-command wrapper and per-job receipt publication. The shared protocol configuration supports automatic task linkage through GitHub closing issues and private run discovery.\n\nIncludes task and optional effort-observation issue forms. Human estimates remain unavailable until actual private activity and sampled responses exist. CI duration is tool runtime, not human labour.\n\nReview the generated command and add any required tool/dependency setup before merging. Other commands and jobs are outside this wrapper's observation scope.\n"
	handle, err := os.CreateTemp("", "observatory-adoption-*.md")
	if err != nil {
		return errors.New("cannot prepare pull request description")
	}
	defer os.Remove(handle.Name())
	if _, err := handle.WriteString(body); err != nil {
		_ = handle.Close()
		return errors.New("cannot write pull request description")
	}
	if err := handle.Close(); err != nil {
		return err
	}
	result, err := call("gh", "pr", "create", "--repo", o.Repository, "--base", repo.DefaultBranch, "--head", o.Branch, "--title", "Add Observatory instrumentation and task linkage", "--body-file", handle.Name())
	if err != nil {
		return fmt.Errorf("adoption branch was pushed; pull request creation failed: %w", err)
	}
	url := strings.TrimSpace(string(result))
	if !strings.HasPrefix(url, "https://github.com/"+o.Repository+"/pull/") || strings.ContainsAny(url, "\r\n") {
		return errors.New("pull request was requested; inspect GitHub for its URL")
	}
	_, err = fmt.Fprintln(stdout, url)
	return err
}
