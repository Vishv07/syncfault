package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const scenariosDir = "../../scenarios"

func newTestServer(t *testing.T, opts Options) *httptest.Server {
	t.Helper()
	if opts.ScenariosDir == "" {
		opts.ScenariosDir = scenariosDir
	}
	ts := httptest.NewServer(New(opts))
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, ts *httptest.Server, path, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s response: %v", path, err)
	}
	return resp.StatusCode, out
}

func TestHealthAndCatalog(t *testing.T) {
	ts := newTestServer(t, Options{})

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("health status %d", resp.StatusCode)
	}

	resp2, err := http.Get(ts.URL + "/v1/catalog")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var cat struct {
		Recipes    []string `json:"recipes"`
		Faults     []string `json:"faults"`
		Adapters   []string `json:"adapters"`
		Scenarios  []string `json:"scenarios"`
		CmdEnabled bool     `json:"cmd_enabled"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Recipes) < 10 || len(cat.Faults) < 8 || len(cat.Scenarios) < 10 {
		t.Fatalf("catalog looks incomplete: %+v", cat)
	}
	if !contains(cat.Adapters, "cmd") || !contains(cat.Adapters, "refgood") {
		t.Fatalf("expected cmd and refgood adapters, got %v", cat.Adapters)
	}
	if cat.CmdEnabled {
		t.Fatal("cmd must be disabled unless AllowCmd is set")
	}
	if contains(cat.Scenarios, "suites") {
		t.Fatal("suites.yaml is not a scenario")
	}
}

func TestRunNamedScenarioOnRefgoodIsClean(t *testing.T) {
	ts := newTestServer(t, Options{})
	code, out := post(t, ts, "/v1/runs", `{"adapter":"refgood","scenario_name":"basic_crud_none","seed":3}`)
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	if out["verdict"] != "CLEAN" || out["exit_code"].(float64) != 0 || out["seed"].(float64) != 3 {
		t.Fatalf("unexpected result: %v", out)
	}
}

func TestRunInlineScenarioCatchesRefbad(t *testing.T) {
	ts := newTestServer(t, Options{})
	body, _ := json.Marshal(map[string]any{
		"adapter": "refbad",
		"seed":    1,
		"scenario_yaml": `name: inline_rename_sigkill
workload:
  recipe: rename_storm
  params:
    depth: 3
    files: 8
    file_size: { min: 256B, max: 1KiB }
  sides: [A]
fault:
  type: sigkill
  at:
    op_index: 10
runs:
  seeds: [1]
  timeout_s: 30
`,
	})
	code, out := post(t, ts, "/v1/runs", string(body))
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	if out["verdict"] != "DATA_LOSS" || out["exit_code"].(float64) != 40 {
		t.Fatalf("expected DATA_LOSS/40 from the sabotaged adapter, got %v", out)
	}
}

func TestCmdAdapterIsGated(t *testing.T) {
	const req = `{"adapter":"cmd","scenario_name":"basic_crud_none","seed":1,"sync_cmd":"cp -R {sideA}/. {sideB}/"}`

	closed := newTestServer(t, Options{})
	if code, out := post(t, closed, "/v1/runs", req); code != http.StatusForbidden {
		t.Fatalf("expected 403 without AllowCmd, got %d: %v", code, out)
	}

	open := newTestServer(t, Options{AllowCmd: true})
	code, out := post(t, open, "/v1/runs", req)
	if code != 200 || out["adapter"] != "cmd" {
		t.Fatalf("expected a cmd run when allowed, got %d: %v", code, out)
	}
	if out["verdict"] == "CLEAN" {
		t.Fatalf("a one-way copy with no delete handling must not be CLEAN: %v", out)
	}

	if code, _ := post(t, open, "/v1/runs", `{"adapter":"cmd","scenario_name":"basic_crud_none"}`); code != 400 {
		t.Fatalf("expected 400 for a missing sync_cmd, got %d", code)
	}
}

func TestRunRejectsBadRequests(t *testing.T) {
	ts := newTestServer(t, Options{})
	cases := map[string]string{
		"no scenario":        `{"adapter":"refgood"}`,
		"both scenario":      `{"scenario_name":"basic_crud_none","scenario_yaml":"name: x"}`,
		"traversal":          `{"scenario_name":"../../etc/passwd"}`,
		"unknown name":       `{"scenario_name":"does_not_exist"}`,
		"unknown adapter":    `{"adapter":"nope","scenario_name":"basic_crud_none"}`,
		"unknown field":      `{"scenario_name":"basic_crud_none","bogus":1}`,
		"not json":           `hello`,
		"bad yaml":           `{"scenario_yaml":"this is not a scenario"}`,
		"cmd text on others": `{"adapter":"refgood","scenario_name":"basic_crud_none","sync_cmd":"true"}`,
	}
	for name, body := range cases {
		code, out := post(t, ts, "/v1/runs", body)
		if code != 400 {
			t.Errorf("%s: expected 400, got %d (%v)", name, code, out)
		}
		if _, ok := out["error"]; !ok {
			t.Errorf("%s: expected an error field, got %v", name, out)
		}
	}
}

func TestRunRejectsOversizedBody(t *testing.T) {
	ts := newTestServer(t, Options{})
	big := `{"scenario_yaml":"` + strings.Repeat("a", maxBodyBytes+10) + `"}`
	code, _ := post(t, ts, "/v1/runs", big)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", code)
	}
}

func TestRunReturns429WhenAtCapacity(t *testing.T) {
	a := &api{opts: Options{ScenariosDir: scenariosDir}, sem: make(chan struct{}, 1)}
	a.sem <- struct{}{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/runs", strings.NewReader(`{"scenario_name":"basic_crud_none"}`))
	a.runScenario(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVerifyEndpoint(t *testing.T) {
	ts := newTestServer(t, Options{})

	created := `{"op_index":1,"side":"A","op":"create","path":"a.txt","content_hash_after":"h1"}`
	deleted := `{"op_index":3,"side":"A","op":"delete","path":"gone.txt","content_hash_before":"g1"}`
	madeGone := `{"op_index":2,"side":"A","op":"create","path":"gone.txt","content_hash_after":"g1"}`

	t.Run("clean", func(t *testing.T) {
		code, out := post(t, ts, "/v1/verify", `{"entries":[`+created+`],
			"side_a":{"a.txt":"h1"},"side_b":{"a.txt":"h1"},"remote":{"a.txt":"h1"}}`)
		if code != 200 || out["verdict"] != "CLEAN" {
			t.Fatalf("expected CLEAN, got %d %v", code, out)
		}
	})
	t.Run("data loss", func(t *testing.T) {
		code, out := post(t, ts, "/v1/verify", `{"entries":[`+created+`],"side_a":{},"side_b":{},"remote":{}}`)
		if code != 200 || out["verdict"] != "DATA_LOSS" || out["exit_code"].(float64) != 40 {
			t.Fatalf("expected DATA_LOSS, got %d %v", code, out)
		}
	})
	t.Run("wrong bytes", func(t *testing.T) {
		code, out := post(t, ts, "/v1/verify", `{"entries":[`+created+`],
			"side_a":{"a.txt":"torn"},"side_b":{"a.txt":"torn"},"remote":{"a.txt":"torn"}}`)
		if code != 200 || out["verdict"] != "WRONG_BYTES" {
			t.Fatalf("expected WRONG_BYTES, got %d %v", code, out)
		}
	})
	t.Run("resurrected delete", func(t *testing.T) {
		tree := `{"a.txt":"h1","gone.txt":"g1"}`
		code, out := post(t, ts, "/v1/verify", `{"entries":[`+created+`,`+madeGone+`,`+deleted+`],
			"side_a":`+tree+`,"side_b":`+tree+`,"remote":`+tree+`}`)
		if code != 200 || out["verdict"] != "DUPLICATE" {
			t.Fatalf("expected DUPLICATE, got %d %v", code, out)
		}
	})
	t.Run("stall when not converged", func(t *testing.T) {
		code, out := post(t, ts, "/v1/verify", `{"converged":false,"entries":[`+created+`],
			"side_a":{"a.txt":"h1"},"side_b":{"a.txt":"h1"},"remote":{"a.txt":"h1"}}`)
		if code != 200 || out["verdict"] != "STALL" {
			t.Fatalf("expected STALL, got %d %v", code, out)
		}
	})
	t.Run("bad policy", func(t *testing.T) {
		if code, _ := post(t, ts, "/v1/verify", `{"policy":"yolo"}`); code != 400 {
			t.Fatalf("expected 400, got %d", code)
		}
	})
}

func TestRequireLoopback(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:8475", "localhost:0", "[::1]:9000", "127.0.0.2:1"} {
		if err := requireLoopback(ok); err != nil {
			t.Errorf("%s should be allowed: %v", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:8475", ":8475", "192.168.1.5:80", "example.com:80", "8475"} {
		if err := requireLoopback(bad); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestServeListensOnLoopbackAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuffer{}
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, "127.0.0.1:0", Options{ScenariosDir: scenariosDir}, out) }()

	var url string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := out.String(); strings.Contains(s, "http://") {
			url = strings.TrimSpace(s[strings.Index(s, "http://"):])
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if url == "" {
		cancel()
		t.Fatal("server never reported its address")
	}

	resp, err := http.Get(url + "/v1/health")
	if err != nil {
		cancel()
		t.Fatalf("health request: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop after cancel")
	}

	if err := Serve(context.Background(), "0.0.0.0:0", Options{}, io.Discard); err == nil {
		t.Fatal("Serve must refuse a non-loopback address")
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
