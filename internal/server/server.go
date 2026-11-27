// Package server exposes SyncFault over a small local JSON HTTP API so tests written in any
// language can run scenarios and ask the Oracle for a verdict.
//
// The server binds to loopback only. POST /v1/runs can execute a shell command when the
// operator starts the server with AllowCmd, so it must never be reachable from a network.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/adapter/cmdexec"
	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/oracle"
	"github.com/Vishv07/syncfault/internal/runner"
	"github.com/Vishv07/syncfault/internal/scenario"
	"github.com/Vishv07/syncfault/internal/verify"
)

const maxBodyBytes = 1 << 20

var scenarioNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// Options configures the API handler.
type Options struct {
	// ScenariosDir is where scenario_name lookups read <name>.yaml from. Defaults to "scenarios".
	ScenariosDir string
	// AllowCmd permits requests that run a shell command through the "cmd" adapter.
	AllowCmd bool
	// MaxConcurrent caps simultaneous runs. Extra requests receive 429. Defaults to 2.
	MaxConcurrent int
}

type api struct {
	opts Options
	sem  chan struct{}
}

// New returns the HTTP handler serving the /v1 API.
func New(opts Options) http.Handler {
	if opts.ScenariosDir == "" {
		opts.ScenariosDir = "scenarios"
	}
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = 2
	}
	a := &api{opts: opts, sem: make(chan struct{}, opts.MaxConcurrent)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", a.health)
	mux.HandleFunc("GET /v1/catalog", a.catalog)
	mux.HandleFunc("POST /v1/runs", a.runScenario)
	mux.HandleFunc("POST /v1/verify", a.verifyState)
	return mux
}

// Serve listens on addr (loopback only) and serves until ctx is cancelled.
// The resolved listen URL is written to out once the socket is open.
func Serve(ctx context.Context, addr string, opts Options, out io.Writer) error {
	if err := requireLoopback(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           New(opts),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	fmt.Fprintf(out, "syncfault api listening on http://%s\n", ln.Addr())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("refusing to listen on %q: the API can run commands and must stay on loopback (127.0.0.1 or ::1)", addr)
	}
	return nil
}

func (a *api) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) catalog(w http.ResponseWriter, _ *http.Request) {
	adapters := append(runner.Adapters(), "cmd")
	sort.Strings(adapters)
	writeJSON(w, http.StatusOK, map[string]any{
		"recipes":     runner.Recipes(),
		"faults":      runner.Faults(),
		"adapters":    adapters,
		"scenarios":   a.scenarioNames(),
		"verdicts":    []string{"CLEAN", "STALL", "DUPLICATE", "WRONG_BYTES", "DATA_LOSS"},
		"cmd_enabled": a.opts.AllowCmd,
	})
}

func (a *api) scenarioNames() []string {
	entries, err := os.ReadDir(a.opts.ScenariosDir)
	if err != nil {
		return []string{}
	}
	names := []string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") || name == "suites.yaml" {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ".yaml"))
	}
	sort.Strings(names)
	return names
}

type runRequest struct {
	Adapter      string `json:"adapter"`
	ScenarioName string `json:"scenario_name"`
	ScenarioYAML string `json:"scenario_yaml"`
	Seed         uint64 `json:"seed"`
	SyncCmd      string `json:"sync_cmd"`
}

func (a *api) runScenario(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Adapter == "" {
		req.Adapter = "refgood"
	}

	raw, err := a.loadScenario(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sc, err := scenario.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scenario: "+err.Error())
		return
	}

	seed := req.Seed
	if seed == 0 {
		if len(sc.Runs.Seeds) == 0 {
			writeError(w, http.StatusBadRequest, "scenario has no seeds and no seed was given")
			return
		}
		seed = sc.Runs.Seeds[0]
	}

	cfg := runner.RunConfig{AdapterName: req.Adapter, Scenario: sc, Seed: seed}
	if req.Adapter == "cmd" {
		if !a.opts.AllowCmd {
			writeError(w, http.StatusForbidden, "the cmd adapter is disabled; start the server with --allow-cmd")
			return
		}
		if err := cmdexec.Validate(req.SyncCmd); err != nil {
			writeError(w, http.StatusBadRequest, "sync_cmd: "+err.Error())
			return
		}
		cfg.Target = cmdexec.New(req.SyncCmd)
	} else {
		if req.SyncCmd != "" {
			writeError(w, http.StatusBadRequest, "sync_cmd requires adapter \"cmd\"")
			return
		}
		if _, err := adapter.New(req.Adapter); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	select {
	case a.sem <- struct{}{}:
		defer func() { <-a.sem }()
	default:
		writeError(w, http.StatusTooManyRequests, "too many runs in flight, retry shortly")
		return
	}

	res, err := runner.ExecuteSingle(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "harness error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res.JSON())
}

func (a *api) loadScenario(req runRequest) (string, error) {
	switch {
	case req.ScenarioYAML != "" && req.ScenarioName != "":
		return "", errors.New("send either scenario_name or scenario_yaml, not both")
	case req.ScenarioYAML != "":
		return req.ScenarioYAML, nil
	case req.ScenarioName != "":
		if !scenarioNamePattern.MatchString(req.ScenarioName) {
			return "", errors.New("scenario_name may only contain letters, digits and underscores")
		}
		data, err := os.ReadFile(filepath.Join(a.opts.ScenariosDir, req.ScenarioName+".yaml"))
		if err != nil {
			return "", fmt.Errorf("unknown scenario_name %q", req.ScenarioName)
		}
		return string(data), nil
	default:
		return "", errors.New("scenario_name or scenario_yaml is required")
	}
}

type verifyRequest struct {
	Policy    string            `json:"policy"`
	Converged *bool             `json:"converged"`
	Entries   []journal.Entry   `json:"entries"`
	SideA     map[string]string `json:"side_a"`
	SideB     map[string]string `json:"side_b"`
	Remote    map[string]string `json:"remote"`
}

type verifyResponse struct {
	Verdict  string   `json:"verdict"`
	ExitCode int      `json:"exit_code"`
	Reason   string   `json:"reason"`
	Flags    []string `json:"flags"`
}

// verifyState runs the Oracle and Verifier on a journal and final trees supplied by the caller.
// It is a pure function of the request: nothing touches disk.
func (a *api) verifyState(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if !decode(w, r, &req) {
		return
	}
	policy := scenario.ConflictPolicy(req.Policy)
	switch policy {
	case "":
		policy = scenario.ConflictPolicyConflictCopy
	case scenario.ConflictPolicyDefault, scenario.ConflictPolicyLWW, scenario.ConflictPolicyConflictCopy:
	default:
		writeError(w, http.StatusBadRequest, `policy must be "default", "lww" or "conflict_copy"`)
		return
	}
	converged := true
	if req.Converged != nil {
		converged = *req.Converged
	}

	model := oracle.Compute(req.Entries, policy)
	res := verify.Evaluate(model, verify.StateSnapshot{
		SideA:  nonNil(req.SideA),
		SideB:  nonNil(req.SideB),
		Remote: nonNil(req.Remote),
	}, converged)

	flags := res.Flags
	if flags == nil {
		flags = []string{}
	}
	writeJSON(w, http.StatusOK, verifyResponse{
		Verdict:  string(res.Verdict),
		ExitCode: res.Verdict.Severity(),
		Reason:   res.Reason,
		Flags:    flags,
	})
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 1 MiB")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
