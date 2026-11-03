package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAliasVSCode(t *testing.T) {
	s, out, errOut, _ := testShell(t)
	// La ligne exacte qui échouait avant.
	if code := s.RunLine(`alias code='"C:\Program Files (x86)\Microsoft VS Code\Code.exe"'`); code != 0 {
		t.Fatalf("code %d, erreurs : %s", code, errOut)
	}
	want := `"C:\Program Files (x86)\Microsoft VS Code\Code.exe"`
	if got := s.aliases["code"]; got != want {
		t.Fatalf("alias enregistré : %q", got)
	}
	if got := s.expandAliases("code ."); got != want+" ." {
		t.Fatalf("développement : %q", got)
	}
	// Une fois développée, la ligne donne bien le chemin complet en un seul argument.
	_, args, err := s.expandCommand(s.expandAliases("code ."))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{`C:\Program Files (x86)\Microsoft VS Code\Code.exe`, "."}) {
		t.Fatalf("arguments : %#v", args)
	}
	out.Reset()
	s.RunLine("alias")
	if !strings.Contains(out.String(), `alias code='"C:\Program Files (x86)\Microsoft VS Code\Code.exe"'`) {
		t.Fatalf("liste : %q", out)
	}
}

func TestAliasExpansion(t *testing.T) {
	s, out, _, _ := testShell(t)
	s.RunLine(`alias ls='ls -la' ll=ls greet='echo bonjour' s='sudo '`)
	cases := map[string]string{
		"ls":          "ls -la",
		"ll /tmp":     "ls -la /tmp",
		"'ll'":        "'ll'",
		"echo ll":     "echo ll",
		"s greet":     "sudo echo bonjour",
		"greet monde": "echo bonjour monde",
	}
	for in, want := range cases {
		if got := s.expandAliases(in); got != want {
			t.Errorf("%q → %q, attendu %q", in, got, want)
		}
	}
	out.Reset()
	s.RunLine("greet monde && greet")
	if out.String() != "bonjour monde\nbonjour\n" {
		t.Fatalf("sortie : %q", out)
	}
	if code := s.RunLine("unalias greet"); code != 0 || s.aliases["greet"] != "" {
		t.Fatal("unalias")
	}
	if code := s.RunLine("alias greet"); code != 1 {
		t.Fatalf("alias inconnu : code %d", code)
	}
}

func TestTerminalRC(t *testing.T) {
	s, out, _, _ := testShell(t)
	rc := "# mes alias\nalias hi='echo salut'\n"
	if err := os.WriteFile(filepath.Join(homeDir(), ".terminalrc"), []byte(rc), 0o644); err != nil {
		t.Fatal(err)
	}
	s.loadRC()
	out.Reset()
	s.RunLine("hi")
	if out.String() != "salut\n" {
		t.Fatalf("sortie : %q", out)
	}
}
