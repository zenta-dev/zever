package container

import (
	"io"
	"os"
	"testing"
)

// TestDefaultPoolWarn_InfoLinesAreDebugOnly proves the informational
// "pool shared" lines stay off stderr unless ZEVER_DEBUG is set.
func TestDefaultPoolWarn_InfoLinesAreDebugOnly(t *testing.T) {
	capture := func(msg string) string {
		old := os.Stderr
		rd, wr, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}

		os.Stderr = wr

		defaultPoolWarn(msg)

		os.Stderr = old
		_ = wr.Close()
		b, _ := io.ReadAll(rd)
		_ = rd.Close()

		return string(b)
	}

	info := "container: pool shared sqlite max_conns=0 services=[db]"
	mismatch := "container: db (shared pool): max_conns=5 ignored, shared pool max_conns=2"

	t.Setenv("ZEVER_DEBUG", "")

	if got := capture(info); got != "" {
		t.Errorf("info line printed without ZEVER_DEBUG: %q", got)
	}

	if got := capture(mismatch); got == "" {
		t.Error("mismatch warning must always print")
	}

	t.Setenv("ZEVER_DEBUG", "1")

	if got := capture(info); got == "" {
		t.Error("info line must print with ZEVER_DEBUG set")
	}
}
