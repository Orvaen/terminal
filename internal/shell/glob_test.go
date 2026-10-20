package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// globTree crée une arborescence de test et s'y place.
func globTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{
		"app.ts", "app.js", "index.ts", "README.md", ".env", "notes 1.txt", "notes 2.txt",
		"log1.log", "log2.log", "logA.log", "src/a.ts", "src/b.go", "src/lib/c.ts", "src/lib/deep/d.ts",
		".git/x.ts", "node/x.ts",
	} {
		p := filepath.Join(root, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o644)
	}
	old, _ := os.Getwd()
	os.Chdir(root)
	t.Cleanup(func() { os.Chdir(old) })
	return root
}

func expand(t *testing.T, line string) []string {
	t.Helper()
	got, err := expandLine(line)
	if err != nil {
		t.Fatalf("%s : %v", line, err)
	}
	return got
}

func TestGlobBasics(t *testing.T) {
	globTree(t)
	for line, want := range map[string][]string{
		"ls *.ts":             {"ls", "app.ts", "index.ts"},
		"rm log?.log":         {"rm", "log1.log", "log2.log", "logA.log"},
		"rm log[0-9].log":     {"rm", "log1.log", "log2.log"},
		"rm log[!0-9].log":    {"rm", "logA.log"},
		"ls src/*.ts":         {"ls", "src/a.ts"},
		"ls */*.go":           {"ls", "src/b.go"},
		"ls ./*.md":           {"ls", "./README.md"},
		`find . -name "*.ts"`: {"find", ".", "-name", "*.ts"}, // entre guillemets : intact
		"echo '*'":            {"echo", "*"},
		"ls *.rien":           {"ls", "*.rien"}, // aucun fichier : tel quel
		`cat "notes "*.txt`:   {"cat", "notes 1.txt", "notes 2.txt"},
		"ls .e*":              {"ls", ".env"}, // caché seulement avec un point au début
	} {
		if got := expand(t, line); !reflect.DeepEqual(got, want) {
			t.Errorf("%s :\n  obtenu  %q\n  attendu %q", line, got, want)
		}
	}
	if got := expand(t, "ls *"); contains(got, ".env") || contains(got, ".git") {
		t.Errorf("* ne doit pas prendre les fichiers cachés : %v", got)
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestGlobStar(t *testing.T) {
	globTree(t)
	got := expand(t, "ls src/**/*.ts")
	want := []string{"ls", "src/a.ts", "src/lib/c.ts", "src/lib/deep/d.ts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu %q", got)
	}
	got = expand(t, "ls **/*.ts")
	if !contains(got, "app.ts") || !contains(got, "node/x.ts") || contains(got, ".git/x.ts") {
		t.Fatalf("** depuis la racine, sans les dossiers cachés : %q", got)
	}
}

func TestBraces(t *testing.T) {
	globTree(t)
	for line, want := range map[string][]string{
		"cp app.{ts,js} dist/": {"cp", "app.ts", "app.js", "dist/"},
		"mkdir jour{1..3}":     {"mkdir", "jour1", "jour2", "jour3"},
		"echo {05..07}":        {"echo", "05", "06", "07"},
		"echo {3..1}":          {"echo", "3", "2", "1"},
		"echo {a..c}{1,2}":     {"echo", "a1", "a2", "b1", "b2", "c1", "c2"},
		"echo {0..10..5}":      {"echo", "0", "5", "10"},
		"echo x{a,{b,c}}y":     {"echo", "xay", "xby", "xcy"},
		`echo "{a,b}" {} {x}`:  {"echo", "{a,b}", "{}", "{x}"},
		"ls *.{ts,md}":         {"ls", "app.ts", "index.ts", "README.md"},
	} {
		if got := expand(t, line); !reflect.DeepEqual(got, want) {
			t.Errorf("%s :\n  obtenu  %q\n  attendu %q", line, got, want)
		}
	}
}

func TestGlobCaseInsensitiveOnWindowsMode(t *testing.T) {
	globTree(t)
	old := globCaseInsensitive
	globCaseInsensitive = true
	t.Cleanup(func() { globCaseInsensitive = old })
	if got := expand(t, "ls readme.*"); !reflect.DeepEqual(got, []string{"ls", "README.md"}) {
		t.Fatalf("obtenu %q", got)
	}
}

func TestGlobInCommandsAndRedirect(t *testing.T) {
	root := globTree(t)
	s, out, errOut, _ := testShell(t)
	os.Chdir(root)
	if code := s.RunLine("rm log*.log"); code != 0 {
		t.Fatalf("rm : %d %s", code, errOut)
	}
	if m, _ := filepath.Glob(filepath.Join(root, "log*")); len(m) != 0 {
		t.Fatalf("fichiers restants : %v", m)
	}
	s.RunLine("echo src/*.go")
	if strings.TrimSpace(out.String()) != "src/b.go" {
		t.Fatalf("echo : %q", out)
	}
	if code := s.RunLine("echo x > notes*.txt"); code != 2 || !strings.Contains(errOut.String(), "ambiguë") {
		t.Fatalf("redirection vers plusieurs fichiers : %d %q", code, errOut)
	}
}
