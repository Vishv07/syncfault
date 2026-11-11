package runner

import (
	"bytes"
	"context"
	"testing"
)

func TestRunSelfCheckGatePasses(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	var buf bytes.Buffer
	sum, err := RunSelfCheck(context.Background(), &buf)
	if err != nil {
		t.Fatalf("RunSelfCheck error: %v", err)
	}
	if !sum.Passed {
		t.Fatalf("expected SelfCheck gate to pass, got %+v\n%s", sum, buf.String())
	}
}
