// Package results defines the result file LvW-bench writes, and reads them back for the
// leaderboard, comparisons and exports.
package results

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Version of the bench (tasks + scoring). Results from other versions are kept apart.
const Version = "0.1.0"

// Model identifies exactly what ran.
type Model struct {
	Label     string `json:"label"`
	Input     string `json:"input"`
	Format    string `json:"format"`
	Repo      string `json:"repo,omitempty"`
	SHA       string `json:"sha,omitempty"`
	File      string `json:"file,omitempty"`
	Quant     string `json:"quant,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	FileSHA   string `json:"file_sha256,omitempty"`
	Backend   string `json:"backend"`
	Params    string `json:"params,omitempty"`
}

// ConfigScore is one task set's score inside a track.
type ConfigScore struct {
	N        int     `json:"n"`
	Accuracy float64 `json:"accuracy"`
	KL       float64 `json:"kl_from_gold,omitempty"`
	Brier    float64 `json:"brier,omitempty"`
	ECE      float64 `json:"ece,omitempty"`
	Coverage float64 `json:"coverage,omitempty"`
}

// Track is the result of one track.
type Track struct {
	Accuracy        float64                `json:"accuracy"`
	CILow           float64                `json:"ci_low"`
	CIHigh          float64                `json:"ci_high"`
	KL              float64                `json:"kl_from_gold,omitempty"`
	Brier           float64                `json:"brier,omitempty"`
	ECE             float64                `json:"ece,omitempty"`
	N               int                    `json:"n"`
	Coverage        float64                `json:"coverage,omitempty"`
	Adapter         string                 `json:"adapter,omitempty"`
	MeanOutTokens   float64                `json:"mean_output_tokens,omitempty"`
	TokensPerSecond float64                `json:"tokens_per_second,omitempty"`
	P50Millis       float64                `json:"p50_ms"`
	WallSeconds     float64                `json:"wall_seconds"`
	TruncationRate  float64                `json:"truncation_rate,omitempty"`
	NativeThinking  bool                   `json:"native_thinking,omitempty"`
	Errors          int                    `json:"errors,omitempty"`
	PerConfig       map[string]ConfigScore `json:"per_config"`
}

// Result is one run of one model.
type Result struct {
	Bench        string            `json:"bench"`
	BenchVersion string            `json:"bench_version"`
	Created      time.Time         `json:"created"`
	Model        Model             `json:"model"`
	Mode         string            `json:"mode"` // general or specialist
	Profile      string            `json:"profile"`
	Seed         int64             `json:"seed"`
	Tracks       map[string]*Track `json:"tracks"`
	System       map[string]string `json:"system"`
	Notes        string            `json:"notes,omitempty"`
}

// Prediction is one scored item, written next to the result for rescoring.
type Prediction struct {
	Track         string             `json:"track"`
	Config        string             `json:"config"`
	ID            string             `json:"id"`
	Question      string             `json:"question,omitempty"`
	Type          string             `json:"type,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Output        string             `json:"output,omitempty"`    // the scored text, whole
	Reasoning     string             `json:"reasoning,omitempty"` // separated thinking, clipped, not scored
	Extracted     string             `json:"extracted,omitempty"`
	Correct       bool               `json:"correct"`
	OutputTokens  int                `json:"output_tokens,omitempty"`
	Millis        float64            `json:"ms"`
	Truncated     bool               `json:"truncated,omitempty"`
	Error         string             `json:"error,omitempty"`
}

// Dir is where results are saved by default.
func Dir(state string) string { return filepath.Join(state, "results") }

// Slug is a file-safe name for a result.
func Slug(r *Result) string {
	s := strings.ToLower(r.Model.Label)
	s = strings.NewReplacer(" · ", "__", "/", "_", " ", "_", ":", "_").Replace(s)
	return fmt.Sprintf("%s__%s__%s", s, r.Profile, r.Created.UTC().Format("20060102-150405"))
}

// Save writes <dir>/<slug>.json and <slug>.predictions.jsonl.
func Save(dir string, r *Result, preds []Prediction) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(dir, Slug(r))
	b, _ := json.MarshalIndent(r, "", "  ")
	if err := os.WriteFile(base+".json", append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	f, err := os.Create(base + ".predictions.jsonl")
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, p := range preds {
		_ = enc.Encode(p)
	}
	return base + ".json", nil
}

// Load reads one result file.
func Load(path string) (*Result, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if r.Bench != "LvW-bench" {
		return nil, fmt.Errorf("%s: not an LvW-bench result", path)
	}
	return &r, nil
}

// LoadDir reads every result in a folder (predictions files are skipped), newest first.
func LoadDir(dir string) ([]*Result, error) {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []*Result
	for _, p := range paths {
		if r, err := Load(p); err == nil {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Row is one leaderboard row.
type Row struct {
	Rank   int
	Result *Result
	Track  *Track
}

// Leaderboard keeps the newest result per model label and sorts by accuracy (System One:
// accuracy, then lower KL). Only results of this bench version and profile count.
func Leaderboard(rs []*Result, track, profile string) []Row {
	seen := map[string]bool{}
	var rows []Row
	for _, r := range rs { // newest first
		t := r.Tracks[track]
		if t == nil || r.BenchVersion != Version || (profile != "" && r.Profile != profile) {
			continue
		}
		key := r.Model.Label + "|" + r.Mode
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, Row{Result: r, Track: t})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].Track, rows[j].Track
		if a.Accuracy != b.Accuracy {
			return a.Accuracy > b.Accuracy
		}
		return a.KL < b.KL
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}
