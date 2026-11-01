package discovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const maxJSONBytes = 16 * 1024 * 1024

// Decode only bounded JSON with unique object keys. API extension fields are
// permitted; only the explicitly selected fields are retained in evidence.
func decodeJSON(raw []byte, value any) error {
	if len(raw) > maxJSONBytes {
		return errors.New("JSON exceeds the 16 MiB limit")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 100 {
			return errors.New("JSON exceeds nesting limit")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				keys := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return err
					}
					name, ok := key.(string)
					if !ok || keys[name] {
						return errors.New("duplicate or invalid JSON key")
					}
					keys[name] = true
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
			case '[':
				for d.More() {
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
			default:
				return errors.New("invalid JSON delimiter")
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := walk(0); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON content")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return d.Decode(value)
}

func readJSON(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxJSONBytes+1))
	if err != nil {
		return err
	}
	return decodeJSON(raw, value)
}

func ensureNoSymlinks(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for p := absolute; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("output paths must not contain symlinks")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeImmutable(path, raw)
}

func preflightOutputs(directory string, values map[string]any) error {
	for _, name := range []string{"manifest.json", "effort-links.json", "discovery.json"} {
		path := filepath.Join(directory, name)
		if err := ensureNoSymlinks(path); err != nil {
			return err
		}
		existing, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		value, expected := values[name]
		if !expected {
			return errors.New("output directory contains evidence from another discovery")
		}
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		raw = append(raw, '\n')
		if !bytes.Equal(existing, raw) {
			return errors.New("output directory contains different evidence; choose a new directory")
		}
	}
	return nil
}

func writeImmutable(path string, raw []byte) error {
	if err := ensureNoSymlinks(path); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return errors.New("output exists with different evidence; select a new output path")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".observatory-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Link atomically fails if another process created the destination first.
	return os.Link(tmp, path)
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

type api interface {
	get(string, any) (string, error)
	graphql(any, any) (string, error)
}

type githubAPI struct{ calls int }

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.Len() {
		return 0, errors.New("API response limit exceeded")
	}
	return w.Buffer.Write(p)
}

func (a *githubAPI) call(arguments []string, input any, target any) (string, error) {
	a.calls++
	if a.calls > 500 {
		return "", errors.New("GitHub request limit reached")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", append([]string{"api", "--hostname", "github.com"}, arguments...)...)
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return "", err
		}
		cmd.Stdin = bytes.NewReader(raw)
	}
	out := &cappedBuffer{limit: maxJSONBytes}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", errors.New("GitHub API unavailable; check authentication, permissions and limits")
	}
	if err := decodeJSON(out.Bytes(), target); err != nil {
		return "", errors.New("GitHub returned invalid or oversized JSON")
	}
	return digest(out.Bytes()), nil
}
func (a *githubAPI) get(path string, target any) (string, error) {
	return a.call([]string{path}, nil, target)
}
func (a *githubAPI) graphql(request any, target any) (string, error) {
	return a.call([]string{"graphql", "--input", "-"}, request, target)
}
