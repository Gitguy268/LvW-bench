package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Gitguy268/LvW-bench/helper"
)

// Server is a model server LvW-bench started (or an external one it only talks to).
type Server struct {
	Client  *Client
	Backend string // llama, hf, openai, ollama, systemone
	LogPath string
	Version string // engine version, recorded with the result
	cmd     *exec.Cmd
	exited  chan error
}

// Close stops a server LvW-bench started.
func (s *Server) Close() {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	stop(s.cmd)
	select {
	case <-s.exited:
	case <-time.After(10 * time.Second):
		_ = s.cmd.Process.Kill()
		<-s.exited
	}
}

// External wraps a server that is already running.
func External(base, model, backend string) *Server {
	return &Server{Client: NewClient(base, model), Backend: backend}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// StateDir holds logs, the unpacked helper and saved results.
func StateDir() string {
	if d := os.Getenv("LVWB_HOME"); d != "" {
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "lvw-bench")
	}
	return filepath.Join(os.TempDir(), "lvw-bench")
}

// FindLlamaServer looks in LVWB_LLAMA_SERVER, PATH and the usual install places.
func FindLlamaServer() (string, error) {
	if p := os.Getenv("LVWB_LLAMA_SERVER"); p != "" {
		return p, nil
	}
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, p := range []string{"/opt/homebrew/bin/llama-server", "/usr/local/bin/llama-server", "/home/linuxbrew/.linuxbrew/bin/llama-server"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("llama-server not found: install llama.cpp (brew install llama.cpp, or a release from github.com/ggml-org/llama.cpp) or set LVWB_LLAMA_SERVER")
}

// LlamaOptions tune llama-server.
type LlamaOptions struct {
	Ctx      int // context per slot
	Parallel int
	GPU      bool
	Extra    []string
}

// StartLlama starts llama-server on a free loopback port for a local GGUF.
func StartLlama(ctx context.Context, gguf string, o LlamaOptions) (*Server, error) {
	bin, err := FindLlamaServer()
	if err != nil {
		return nil, err
	}
	if o.Ctx == 0 {
		o.Ctx = 4096
	}
	if o.Parallel == 0 {
		o.Parallel = 4
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	args := []string{"-m", gguf, "--host", "127.0.0.1", "--port", fmt.Sprint(port),
		"-c", fmt.Sprint(o.Ctx * o.Parallel), "-np", fmt.Sprint(o.Parallel), "--jinja", "--no-webui"}
	if o.GPU {
		args = append(args, "-ngl", "999")
	} else {
		args = append(args, "-ngl", "0")
	}
	args = append(args, o.Extra...)
	s, err := start(ctx, "llama", bin, args, port, nil)
	if err == nil {
		s.Version = llamaVersion(bin)
	}
	return s, err
}

// HelperOptions tune the Python helper.
type HelperOptions struct {
	Repo, Revision, Subfolder, Local string
	Adapter, Base                    string
	TrustRemoteCode                  bool
	DType                            string
	Plugin                           string
}

// StartHelper unpacks the embedded helper and runs it with uv on Python 3.12.
func StartHelper(ctx context.Context, o HelperOptions) (*Server, error) {
	uv, err := exec.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("safetensors models need uv (https://docs.astral.sh/uv/, brew install uv): %w", err)
	}
	dir := filepath.Join(StateDir(), "helper")
	if err := helper.Unpack(dir); err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	args := []string{"run", "--quiet", "--project", dir, "--python", "3.12", "python", filepath.Join(dir, "hf_serve.py"),
		"--port", fmt.Sprint(port)}
	if o.Local != "" {
		args = append(args, "--model", o.Local)
	} else {
		args = append(args, "--model", o.Repo)
		if o.Revision != "" {
			args = append(args, "--revision", o.Revision)
		}
	}
	if o.Subfolder != "" {
		args = append(args, "--subfolder", o.Subfolder)
	}
	if o.Base != "" {
		args = append(args, "--base", o.Base)
	}
	if o.TrustRemoteCode {
		args = append(args, "--trust-remote-code")
	}
	if o.DType != "" {
		args = append(args, "--dtype", o.DType)
	}
	if o.Plugin != "" {
		args = append(args, "--plugin", o.Plugin)
	}
	s, err := start(ctx, "hf", uv, args, port, []string{"PYTHONUNBUFFERED=1"})
	if err == nil {
		s.Version = helperVersion(s.Client.BaseURL)
	}
	return s, err
}

// llamaVersion asks llama-server for its build ("version: 1 (1b6935b)").
func llamaVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "version:") {
			return "llama.cpp " + strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "version:"))
		}
	}
	return "llama.cpp"
}

// helperVersion reads the torch / transformers versions the helper reports on /health.
func helperVersion(base string) string {
	resp, err := http.Get(strings.TrimSuffix(base, "/v1") + "/health")
	if err != nil {
		return "transformers"
	}
	defer resp.Body.Close()
	var h map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&h)
	return fmt.Sprintf("transformers %s, torch %s, %s", h["transformers"], h["torch"], h["device"])
}

func start(ctx context.Context, backend, bin string, args []string, port int, env []string) (*Server, error) {
	logDir := filepath.Join(StateDir(), "logs")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%d.log", backend, time.Now().Unix()))
	logf, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Env = append(os.Environ(), env...)
	setProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		logf.Close()
		return nil, fmt.Errorf("starting %s: %w", filepath.Base(bin), err)
	}
	s := &Server{Backend: backend, LogPath: logPath, cmd: cmd, exited: make(chan error, 1),
		Client: NewClient(fmt.Sprintf("http://127.0.0.1:%d", port), "")}
	go func() { s.exited <- cmd.Wait(); logf.Close() }()
	wctx, cancel := context.WithTimeout(ctx, 20*time.Minute) // first helper run installs torch
	defer cancel()
	if err := WaitReady(wctx, s.Client.BaseURL, s.exited); err != nil {
		s.Close()
		return nil, fmt.Errorf("%w (log: %s)\n%s", err, logPath, tail(logPath, 15))
	}
	return s, nil
}

func tail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
