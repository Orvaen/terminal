package ui

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func capture(t *testing.T, interactive bool) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	oldOut, oldInter, oldColor := Out, Interactive, color
	Out, Interactive, color = &buf, func() bool { return interactive }, false
	t.Cleanup(func() { Out, Interactive, color = oldOut, oldInter, oldColor })
	return &buf
}

func TestBarOutsideConsolePrintsOneLine(t *testing.T) {
	buf := capture(t, false)
	b := NewBar("composer.phar", 3*1024*1024)
	io.Copy(io.Discard, b.Reader(strings.NewReader(strings.Repeat("x", 3*1024*1024))))
	b.Finish()
	b.Finish() // une seule fois
	got := buf.String()
	if strings.Count(got, "\n") != 1 || strings.Contains(got, "\r") {
		t.Fatalf("une seule ligne, sans animation : %q", got)
	}
	if !strings.Contains(got, "composer.phar") || !strings.Contains(got, "3,0 Mo") || !strings.Contains(got, "━") {
		t.Fatalf("ligne finale : %q", got)
	}
}

func TestBarInConsoleAnimates(t *testing.T) {
	buf := capture(t, true)
	b := NewBar("node.msi", 1000)
	b.Write(make([]byte, 400))
	b.mu.Lock()
	b.lastDraw = time.Time{}
	b.mu.Unlock()
	b.Write(make([]byte, 100))
	got := buf.String()
	if !strings.Contains(got, "\r\x1b[2K") || !strings.Contains(got, "500 o/1000 o") || !strings.Contains(got, "╸") {
		t.Fatalf("barre animée : %q", got)
	}
	b.Abort()
}

func TestSpinner(t *testing.T) {
	buf := capture(t, false)
	s := StartSpinner("Mise à jour de winget…")
	s.Done("winget")
	if got := buf.String(); got != "✓ winget\n" {
		t.Fatalf("hors console : %q", got)
	}
	buf.Reset()
	Interactive = func() bool { return true }
	s = StartSpinner("travail")
	time.Sleep(120 * time.Millisecond)
	s.Fail("échec")
	if got := buf.String(); !strings.Contains(got, "travail") || !strings.HasSuffix(got, "✗ échec\n") {
		t.Fatalf("console : %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	for n, want := range map[int64]string{0: "0 o", 1023: "1023 o", 1536: "1,5 Ko", 45600000: "43,5 Mo", 3 << 30: "3,0 Go"} {
		if got := HumanBytes(n); got != want {
			t.Errorf("%d : %q, attendu %q", n, got, want)
		}
	}
}
