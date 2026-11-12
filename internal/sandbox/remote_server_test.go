package sandbox

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLoopbackRemoteServerHonorsNetcutMarker(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sb, err := New("remote-srv-test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Teardown()

	srv, err := StartRemoteServer(sb)
	if err != nil {
		t.Fatalf("StartRemoteServer failed: %v", err)
	}
	defer srv.Close()

	// Normal PUT and GET
	req, _ := http.NewRequest(http.MethodPut, srv.URL()+"hello.txt", bytes.NewReader([]byte("world")))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT failed: resp=%v err=%v", resp, err)
	}

	respGet, err := http.Get(srv.URL() + "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(respGet.Body)
	_ = respGet.Body.Close()
	if string(body) != "world" {
		t.Fatalf("expected 'world', got %q", string(body))
	}

	// Activate netcut marker and verify connection fails
	if err := os.WriteFile(filepath.Join(sb.Work, "fault-netcut.active"), []byte("cut\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	respCut, err := http.Get(srv.URL() + "hello.txt")
	if err == nil && respCut.StatusCode == http.StatusOK {
		t.Fatal("expected GET to fail while fault-netcut.active is present")
	}
}
