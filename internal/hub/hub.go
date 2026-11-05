// Package hub talks to the Hugging Face Hub: repo listings and file downloads into the
// standard HF cache layout ($HF_HOME/hub/models--org--name/snapshots/<sha>/...), so files
// that huggingface_hub already downloaded are reused and the other way round.
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Endpoint can be overridden with HF_ENDPOINT (mirrors, tests).
func Endpoint() string {
	if e := os.Getenv("HF_ENDPOINT"); e != "" {
		return strings.TrimRight(e, "/")
	}
	return "https://huggingface.co"
}

// Token reads HF_TOKEN or the token file huggingface_hub writes. It is only sent to the Hub.
func Token() string {
	if t := os.Getenv("HF_TOKEN"); t != "" {
		return t
	}
	b, err := os.ReadFile(filepath.Join(home(), "token"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func home() string {
	if h := os.Getenv("HF_HOME"); h != "" {
		return h
	}
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "huggingface")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".cache", "huggingface")
}

// CacheDir is the hub cache folder.
func CacheDir() string {
	if c := os.Getenv("HF_HUB_CACHE"); c != "" {
		return c
	}
	return filepath.Join(home(), "hub")
}

// File is one file of a repo.
type File struct {
	Path string `json:"rfilename"`
	Size int64  `json:"size"`
}

// Repo is a repo at one commit.
type Repo struct {
	ID    string `json:"id"`
	SHA   string `json:"sha"`
	Files []File `json:"siblings"`
	Kind  string `json:"-"` // "models" or "datasets"
}

var client = &http.Client{Timeout: 0}

func get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	if t := Token(); t != "" && strings.HasPrefix(u, Endpoint()) {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	req.Header.Set("User-Agent", "lvw-bench")
	return client.Do(req)
}

// Info lists a model repo at a revision ("" = main).
func Info(ctx context.Context, repo, revision string) (*Repo, error) {
	return info(ctx, "models", repo, revision)
}

// DatasetInfo lists a dataset repo.
func DatasetInfo(ctx context.Context, repo, revision string) (*Repo, error) {
	return info(ctx, "datasets", repo, revision)
}

func info(ctx context.Context, kind, repo, revision string) (*Repo, error) {
	if revision == "" {
		revision = "main"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	u := fmt.Sprintf("%s/api/%s/%s/revision/%s?blobs=true", Endpoint(), kind, repo, url.PathEscape(revision))
	resp, err := get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", repo, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 401, 403:
		return nil, fmt.Errorf("%s is private or gated: set HF_TOKEN (HTTP %d)", repo, resp.StatusCode)
	case 404:
		return nil, fmt.Errorf("%s@%s not found on the Hub", repo, revision)
	default:
		return nil, fmt.Errorf("listing %s: HTTP %d", repo, resp.StatusCode)
	}
	var r Repo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	r.Kind = kind
	if r.ID == "" {
		r.ID = repo
	}
	return &r, nil
}

// Progress reports download progress.
type Progress func(file string, done, total int64)

func snapshotDir(kind, repo, sha string) string {
	prefix := map[string]string{"models": "models", "datasets": "datasets"}[kind]
	return filepath.Join(CacheDir(), prefix+"--"+strings.ReplaceAll(repo, "/", "--"), "snapshots", sha)
}

// LocalPath is where a repo file lives in the cache (it may not exist yet).
func LocalPath(r *Repo, file string) string {
	return filepath.Join(snapshotDir(r.Kind, r.ID, r.SHA), filepath.FromSlash(file))
}

// Download fetches one file into the cache unless it is already there, resuming a partial
// download. It returns the local path.
func Download(ctx context.Context, r *Repo, file string, progress Progress) (string, error) {
	dst := LocalPath(r, file)
	var size int64 = -1
	for _, f := range r.Files {
		if f.Path == file {
			size = f.Size
		}
	}
	if st, err := os.Stat(dst); err == nil && (size < 0 || st.Size() == size) {
		return dst, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	part := dst + ".incomplete"
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	kindPrefix := ""
	if r.Kind == "datasets" {
		kindPrefix = "/datasets"
	}
	u := fmt.Sprintf("%s%s/%s/resolve/%s/%s", Endpoint(), kindPrefix, r.ID, r.SHA, escapePath(file))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	if t := Token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	req.Header.Set("User-Agent", "lvw-bench")
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", file, err)
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case 206:
		flags |= os.O_APPEND
	case 200:
		have = 0
		flags |= os.O_TRUNC
	default:
		return "", fmt.Errorf("downloading %s: HTTP %d", file, resp.StatusCode)
	}
	out, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return "", err
	}
	total := size
	if total < 0 && resp.ContentLength > 0 {
		total = have + resp.ContentLength
	}
	buf := make([]byte, 1<<20)
	done := have
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return "", werr
			}
			done += int64(n)
			if progress != nil && time.Since(last) > 200*time.Millisecond {
				progress(file, done, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return "", fmt.Errorf("downloading %s: %w", file, rerr)
		}
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if progress != nil {
		progress(file, done, total)
	}
	if size >= 0 && done != size {
		return "", fmt.Errorf("downloading %s: got %d of %d bytes", file, done, size)
	}
	if err := os.Rename(part, dst); err != nil {
		return "", err
	}
	writeRef(r)
	return dst, nil
}

// writeRef records main -> sha like huggingface_hub does (best effort).
func writeRef(r *Repo) {
	dir := filepath.Join(filepath.Dir(filepath.Dir(snapshotDir(r.Kind, r.ID, r.SHA))), "refs")
	if os.MkdirAll(dir, 0o755) == nil {
		_ = os.WriteFile(filepath.Join(dir, "main"), []byte(r.SHA), 0o644)
	}
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// Remove deletes a repo's cached snapshot (used to free disk after a seed run).
func Remove(r *Repo) error {
	return os.RemoveAll(filepath.Dir(filepath.Dir(snapshotDir(r.Kind, r.ID, r.SHA))))
}
