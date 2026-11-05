// Package resolve turns what the user typed (an HF link or id, a local .gguf, a local
// .safetensors file or folder, or a server URL) into a Spec the backends can load.
package resolve

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Gitguy268/LvW-bench/internal/hub"
)

// Kinds.
const (
	KindGGUF        = "gguf"
	KindSafetensors = "safetensors"
	KindServer      = "server"
)

// Spec is a resolved model.
type Spec struct {
	Input     string   `json:"input"`
	Kind      string   `json:"format"`
	Repo      string   `json:"repo,omitempty"`
	Revision  string   `json:"revision,omitempty"`
	SHA       string   `json:"sha,omitempty"`
	File      string   `json:"file,omitempty"`  // GGUF file in the repo (first part)
	Parts     []string `json:"parts,omitempty"` // every GGUF part
	Subfolder string   `json:"subfolder,omitempty"`
	Local     string   `json:"local,omitempty"` // local file or folder
	BaseURL   string   `json:"base_url,omitempty"`
	Quant     string   `json:"quant,omitempty"`
	Adapter   bool     `json:"adapter,omitempty"`
	SizeBytes int64    `json:"size_bytes,omitempty"`
	Name      string   `json:"name"`
	hubRepo   *hub.Repo
}

// HubRepo returns the listed repo (nil for local files and servers).
func (s *Spec) HubRepo() *hub.Repo { return s.hubRepo }

// Options narrow the choice inside a repo.
type Options struct {
	Quant     string // GGUF quant to prefer, e.g. Q4_K_M
	File      string // exact file in the repo
	Subfolder string // weights in a subfolder
	Revision  string
	Format    string // force "gguf" or "safetensors"
}

var (
	reHFURL  = regexp.MustCompile(`^https?://(?:www\.)?(?:huggingface\.co|hf\.co)/([^/]+/[^/@]+)(?:/(?:blob|resolve|tree)/([^/]+)(?:/(.+))?)?/?$`)
	reRepoID = regexp.MustCompile(`^([A-Za-z0-9][\w.\-]*/[\w.\-]+)(?:@([^:]+))?(?::(.+))?$`)
	reQuant  = regexp.MustCompile(`(?i)(IQ\d_[A-Z]+|Q\d_K_[SML]|Q\d_K|Q\d_\d|Q\d|BF16|F16|F32|MXFP4)`)
	rePart   = regexp.MustCompile(`-(\d{5})-of-(\d{5})\.gguf$`)
)

// QuantPreference is used when no --quant is given.
var QuantPreference = []string{"Q4_K_M", "Q4_K_S", "Q5_K_M", "Q4_0", "Q8_0", "Q6_K", "Q5_0", "BF16", "F16"}

// Resolve resolves an input. Local paths win over repo ids with the same spelling.
func Resolve(ctx context.Context, input string, o Options) (*Spec, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return nil, fmt.Errorf("no model given")
	}
	if strings.HasPrefix(in, "~/") {
		h, _ := os.UserHomeDir()
		in = filepath.Join(h, in[2:])
	}
	if st, err := os.Stat(in); err == nil {
		return resolveLocal(in, st.IsDir(), o)
	}
	if m := reHFURL.FindStringSubmatch(in); m != nil {
		rev := m[2]
		if o.Revision != "" {
			rev = o.Revision
		}
		file := o.File
		if file == "" && strings.HasSuffix(m[3], ".gguf") {
			file = m[3]
		} else if m[3] != "" && o.Subfolder == "" && !strings.Contains(m[3], ".") {
			o.Subfolder = m[3]
		}
		o.File = file
		return resolveRepo(ctx, in, m[1], rev, o)
	}
	if u, err := url.Parse(in); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		base := strings.TrimRight(in, "/")
		return &Spec{Input: input, Kind: KindServer, BaseURL: base, Name: u.Host}, nil
	}
	id := strings.TrimPrefix(strings.TrimPrefix(in, "hf://models/"), "hf://")
	if m := reRepoID.FindStringSubmatch(id); m != nil {
		rev := m[2]
		if o.Revision != "" {
			rev = o.Revision
		}
		if m[3] != "" && o.File == "" {
			o.File = m[3]
		}
		return resolveRepo(ctx, input, m[1], rev, o)
	}
	return nil, fmt.Errorf("%q is not a local file, an HF link or id (org/name), or a server URL", input)
}

func resolveLocal(path string, isDir bool, o Options) (*Spec, error) {
	abs, _ := filepath.Abs(path)
	name := filepath.Base(abs)
	if !isDir {
		switch {
		case strings.HasSuffix(abs, ".gguf"):
			st, _ := os.Stat(abs)
			return &Spec{Input: path, Kind: KindGGUF, Local: abs, Quant: quantOf(name), Name: strings.TrimSuffix(name, ".gguf"), SizeBytes: st.Size()}, nil
		case strings.HasSuffix(abs, ".safetensors"):
			dir := filepath.Dir(abs)
			if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
				return nil, fmt.Errorf("%s: a .safetensors file needs its config.json and tokenizer next to it", path)
			}
			return &Spec{Input: path, Kind: KindSafetensors, Local: dir, Name: filepath.Base(dir)}, nil
		}
		return nil, fmt.Errorf("%s: expected a .gguf or .safetensors file", path)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var files []hub.File
	for _, e := range entries {
		if !e.IsDir() {
			info, _ := e.Info()
			files = append(files, hub.File{Path: e.Name(), Size: info.Size()})
		}
	}
	spec, err := choose(files, o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	spec.Input, spec.Name = path, name
	if spec.Kind == KindGGUF {
		spec.Local = filepath.Join(abs, spec.File)
		spec.File = ""
	} else {
		spec.Local = abs
	}
	return spec, nil
}

func resolveRepo(ctx context.Context, input, repo, rev string, o Options) (*Spec, error) {
	r, err := hub.Info(ctx, repo, rev)
	if err != nil {
		return nil, err
	}
	spec, err := choose(r.Files, o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", repo, err)
	}
	spec.Input, spec.Repo, spec.Revision, spec.SHA, spec.hubRepo = input, r.ID, rev, r.SHA, r
	spec.Name = repo[strings.Index(repo, "/")+1:]
	return spec, nil
}

// choose picks the format and file from a file list.
func choose(files []hub.File, o Options) (*Spec, error) {
	sub := strings.Trim(o.Subfolder, "/")
	inSub := func(p string) bool {
		if sub == "" {
			return true
		}
		return strings.HasPrefix(p, sub+"/")
	}
	var ggufs []hub.File
	hasST, hasCfg, hasAdapter := false, false, false
	var names []string
	for _, f := range files {
		names = append(names, f.Path)
		base := f.Path[strings.LastIndex(f.Path, "/")+1:]
		switch {
		case strings.HasSuffix(f.Path, ".gguf") && !strings.HasPrefix(strings.ToLower(base), "mmproj"):
			ggufs = append(ggufs, f)
		case strings.HasSuffix(f.Path, ".safetensors") && inSub(f.Path) && (sub != "" || !strings.Contains(f.Path, "/")):
			hasST = true
		case f.Path == join(sub, "config.json"):
			hasCfg = true
		case f.Path == join(sub, "adapter_config.json"):
			hasAdapter = true
		}
	}
	if o.File != "" {
		for _, f := range files {
			if f.Path == o.File {
				if strings.HasSuffix(f.Path, ".gguf") {
					return ggufSpec(files, f), nil
				}
			}
		}
		return nil, fmt.Errorf("file %q not found", o.File)
	}
	if o.Format != KindSafetensors && len(ggufs) > 0 {
		return ggufSpec(files, pickQuant(ggufs, o.Quant)), nil
	}
	if o.Format != KindGGUF && hasST && (hasCfg || hasAdapter) {
		return &Spec{Kind: KindSafetensors, Subfolder: sub, Adapter: hasAdapter && !hasCfg, Quant: "safetensors"}, nil
	}
	if o.Format != KindGGUF && hasAdapter {
		return &Spec{Kind: KindSafetensors, Subfolder: sub, Adapter: true}, nil
	}
	// Weights only in subfolders (e.g. ep1/ep2/ep3): say so.
	dirs := map[string]bool{}
	for _, f := range files {
		if i := strings.Index(f.Path, "/"); i > 0 && strings.HasSuffix(f.Path, ".safetensors") {
			dirs[f.Path[:i]] = true
		}
	}
	if len(dirs) > 0 && sub == "" {
		var ds []string
		for d := range dirs {
			ds = append(ds, d)
		}
		sort.Strings(ds)
		return nil, fmt.Errorf("weights are only in subfolders %v: pass --subfolder <name>", ds)
	}
	sort.Strings(names)
	if len(names) > 12 {
		names = append(names[:12], "…")
	}
	return nil, fmt.Errorf("no .gguf or .safetensors (+config.json) model found; files: %v", names)
}

func join(sub, f string) string {
	if sub == "" {
		return f
	}
	return sub + "/" + f
}

func quantOf(name string) string {
	if m := reQuant.FindString(name); m != "" {
		return strings.ToUpper(m)
	}
	return ""
}

func pickQuant(ggufs []hub.File, want string) hub.File {
	first := ggufs[:0:0]
	for _, f := range ggufs {
		if m := rePart.FindStringSubmatch(f.Path); m == nil || m[1] == "00001" {
			first = append(first, f)
		}
	}
	if len(first) == 0 {
		first = ggufs
	}
	if want != "" {
		for _, f := range first {
			if strings.EqualFold(quantOf(f.Path), want) || strings.Contains(strings.ToLower(f.Path), strings.ToLower(want)) {
				return f
			}
		}
	}
	for _, q := range QuantPreference {
		var hit *hub.File
		for i, f := range first {
			if strings.EqualFold(quantOf(f.Path), q) && (hit == nil || len(f.Path) < len(hit.Path)) {
				hit = &first[i] // shortest name: the plain build, not a "-hip-optimized" variant
			}
		}
		if hit != nil {
			return *hit
		}
	}
	best := first[0]
	for _, f := range first {
		if f.Size < best.Size {
			best = f
		}
	}
	return best
}

func ggufSpec(files []hub.File, f hub.File) *Spec {
	s := &Spec{Kind: KindGGUF, File: f.Path, Parts: []string{f.Path}, Quant: quantOf(f.Path), SizeBytes: f.Size}
	if m := rePart.FindStringSubmatch(f.Path); m != nil {
		prefix := strings.TrimSuffix(f.Path, m[0])
		s.Parts = nil
		s.SizeBytes = 0
		for _, g := range files {
			if strings.HasPrefix(g.Path, prefix+"-") && rePart.MatchString(g.Path) {
				s.Parts = append(s.Parts, g.Path)
				s.SizeBytes += g.Size
			}
		}
		sort.Strings(s.Parts)
	}
	return s
}

// Label is how the model shows on a leaderboard: "name · GGUF Q4_K_M" or "name/sub · safetensors".
func (s *Spec) Label() string {
	name := s.Name
	if s.Subfolder != "" {
		name += "/" + s.Subfolder
	}
	switch s.Kind {
	case KindServer:
		return name
	case KindSafetensors:
		if s.Adapter {
			return name + " · adapter"
		}
		return name + " · safetensors"
	}
	return strings.TrimSpace(name + " · GGUF " + s.Quant)
}
