package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseForms(t *testing.T) {
	f, err := Parse([]byte(`{
	  "name": "ignoré",
	  "scripts": {
	    "up": "make up",
	    "check": ["make up", "make test f=00"],
	    "api": { "run": "npm run dev", "cwd": "api", "env": { "PORT": "3000" }, "description": "L'API" },
	    "dev": { "parallel": ["run api", "make logs"] }
	  }
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Names(); !reflect.DeepEqual(got, []string{"up", "check", "api", "dev"}) {
		t.Fatalf("ordre du fichier attendu, obtenu %v", got)
	}
	api, _ := f.Get("api")
	if api.Cwd != "api" || api.Env["PORT"] != "3000" || api.Description != "L'API" || !reflect.DeepEqual(api.Steps, []string{"npm run dev"}) {
		t.Fatalf("api : %+v", api)
	}
	check, _ := f.Get("check")
	if len(check.Steps) != 2 {
		t.Fatalf("check : %+v", check)
	}
	dev, _ := f.Get("dev")
	if !reflect.DeepEqual(dev.Parallel, []string{"run api", "make logs"}) {
		t.Fatalf("dev : %+v", dev)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ json, want string }{
		{`{}`, `pas de section "scripts"`},
		{`{"scripts": {"a": 3}}`, `script "a"`},
		{`{"scripts": {"a": {"run": "x", "parallel": ["y"]}}}`, "ne vont pas ensemble"},
		{`{"scripts": {"a": {"cwd": "x"}}}`, `il faut "run"`},
		{`{"scripts": {"a": {"rnu": "x"}}}`, "rnu"},
		{`{"scripts": {"a": ""}}`, "commande vide"},
		{`{"scripts": {"a b": "x"}}`, "pas d'espace"},
		{`{"scripts": {"a": "x", "a": "y"}}`, "défini deux fois"},
		{`{"scripts": [1]}`, "doit être un objet"},
		{`{"scripts": {`, "JSON invalide"},
	} {
		_, err := Parse([]byte(tc.json))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s : erreur %v, attendu %q", tc.json, err, tc.want)
		}
	}
}

func TestParseAcceptsBOM(t *testing.T) {
	if _, err := Parse([]byte("\xef\xbb\xbf{\"scripts\": {\"a\": \"x\"}}")); err != nil {
		t.Fatal(err)
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "api", "src")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(root, FileName), []byte(`{"scripts":{"a":"x"}}`), 0o644)
	got, err := Find(sub)
	if err != nil || got != filepath.Join(root, FileName) {
		t.Fatalf("obtenu %q %v", got, err)
	}
	if _, err := Find(t.TempDir()); err != ErrNotFound {
		t.Fatalf("ErrNotFound attendu, obtenu %v", err)
	}
}

// project écrit un terminal.json et renvoie un Runner qui exécute les commandes avec sh,
// et « run x » en relançant Main (comme le ferait terminal -c).
func project(t *testing.T, content string) (string, *Runner, *syncBuffer) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("tests d'exécution avec sh")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644)
	out := &syncBuffer{}
	r := &Runner{In: strings.NewReader(""), Out: out, Err: out}
	self, _ := os.Executable()
	r.Command = func(line string) *exec.Cmd {
		if rest, ok := strings.CutPrefix(line, "run "); ok {
			// sous-processus : ce binaire de test, qui lance Main via TestHelperRun
			// (les arguments passent après --, car le Runner remplace l'environnement)
			return exec.Command(self, append([]string{"-test.run=^TestHelperRun$", "--", "helper-run"}, strings.Fields(rest)...)...)
		}
		return exec.Command("sh", "-c", line)
	}
	return dir, r, out
}

// syncBuffer : tampon partagé par plusieurs écrivains (sortie et erreurs des commandes).
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}

// TestHelperRun n'est pas un vrai test : il sert de « terminal -c "run …" » aux autres tests.
func TestHelperRun(t *testing.T) {
	var args []string
	for i, a := range os.Args {
		if a == "helper-run" {
			args = os.Args[i+1:]
		}
	}
	if args == nil {
		t.Skip("utilisé seulement comme sous-processus")
	}
	cwd, _ := os.Getwd()
	r := &Runner{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	r.Command = func(line string) *exec.Cmd { return exec.Command("sh", "-c", line) }
	os.Exit(Main(r, cwd, args))
}

func TestRunSingleWithCwdEnvAndArgs(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {
	  "show": { "run": "echo $GREETING $(basename $PWD)", "cwd": "api", "env": { "GREETING": "salut" } }
	}}`)
	os.Mkdir(filepath.Join(dir, "api"), 0o755)
	if code := Main(r, dir, []string{"show", "--", "en plus"}); code != 0 {
		t.Fatalf("code %d : %s", code, out)
	}
	if !strings.Contains(out.String(), "salut api en plus") {
		t.Fatalf("sortie : %s", out)
	}
}

func TestRunStepsStopAtFirstFailure(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {"all": ["echo un", "exit 3", "echo jamais"]}}`)
	if code := Main(r, dir, []string{"all"}); code != 3 {
		t.Fatalf("code 3 attendu, obtenu %d", code)
	}
	if strings.Contains(out.String(), "jamais\n") || !strings.Contains(out.String(), "un\n") {
		t.Fatalf("sortie : %s", out)
	}
}

func TestRunNodeModulesBinInPath(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {"tool": "montool"}}`)
	bin := filepath.Join(dir, "node_modules", ".bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "montool"), []byte("#!/bin/sh\necho outil local\n"), 0o755)
	if code := Main(r, dir, []string{"tool"}); code != 0 || !strings.Contains(out.String(), "outil local") {
		t.Fatalf("code %d, sortie : %s", code, out)
	}
}

func TestParallelPrefixesOutput(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {
	  "a": "echo depuis-a",
	  "b": "echo depuis-b",
	  "both": { "parallel": ["run a", "run b", "echo direct"] }
	}}`)
	if code := Main(r, dir, []string{"both"}); code != 0 {
		t.Fatalf("code %d : %s", code, out)
	}
	for _, want := range []string{"[a   ] depuis-a", "[b   ] depuis-b", "[echo] direct", "[a   ] terminé"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("%q manquant dans :\n%s", want, out)
		}
	}
}

func TestParallelFailureStopsOthers(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {
	  "long": "exec sleep 30",
	  "bad": "sleep 0.2; exit 4"
	}}`)
	start := time.Now()
	code := Main(r, dir, []string{"-p", "long", "bad"})
	if code != 4 {
		t.Fatalf("code 4 attendu, obtenu %d : %s", code, out)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("la commande longue aurait dû être arrêtée")
	}
	if !strings.Contains(out.String(), "a échoué (code 4)") || !strings.Contains(out.String(), "arrêté") {
		t.Fatalf("sortie : %s", out)
	}
}

func TestSequenceAndUnknown(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {"a": "echo A", "b": "echo B"}}`)
	if code := Main(r, dir, []string{"a", "b"}); code != 0 || strings.Index(out.String(), "A\n") > strings.Index(out.String(), "B\n") {
		t.Fatalf("code %d, sortie : %s", code, out)
	}
	out.Reset()
	if code := Main(r, dir, []string{"a", "nope"}); code != 1 || strings.Contains(out.String(), "A\n") {
		t.Fatalf("un script inconnu doit tout annuler avant de lancer quoi que ce soit : %s", out)
	}
	if !strings.Contains(out.String(), "script inconnu : nope") || !strings.Contains(out.String(), "a, b") {
		t.Fatalf("sortie : %s", out)
	}
}

func TestListAndMissingFile(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {"up": "make up", "dev": {"parallel": ["run up", "make logs"], "description": "Tout lancer"}}}`)
	if Main(r, dir, nil) != 0 {
		t.Fatal(out)
	}
	for _, want := range []string{"up   make up", "en même temps : run up | make logs", "Tout lancer"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("%q manquant :\n%s", want, out)
		}
	}
	out.Reset()
	if Main(r, t.TempDir(), []string{"x"}) != 1 || !strings.Contains(out.String(), "run --init") {
		t.Fatalf("sortie : %s", out)
	}
}

func TestInitImportsMakefileAndPackageJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte(".PHONY: up test\nVAR := 1\nup:\n\tdocker compose up\ntest: up\n\t@echo\n%.o: %.c\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts": {"dev": "vite", "test": "vitest"}}`), 0o644)
	out := &bytes.Buffer{}
	r := &Runner{Out: out, Err: out}
	if code := r.Init(dir); code != 0 {
		t.Fatal(out)
	}
	f, err := Load(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Names(); !reflect.DeepEqual(got, []string{"up", "test", "dev"}) {
		t.Fatalf("scripts : %v", got)
	}
	if s, _ := f.Get("test"); s.Steps[0] != "make test" {
		t.Fatalf("test doit venir du Makefile en premier : %v", s.Steps)
	}
	if r.Init(dir) != 1 {
		t.Fatal("ne doit pas écraser un terminal.json existant")
	}
}

func TestTasksForLabels(t *testing.T) {
	got := tasksFor([]string{"run api", "npm run web", "run api", "make logs"})
	want := []string{"api", "npm", "api#2", "make"}
	for i, tk := range got {
		if tk.label != want[i] {
			t.Errorf("étiquette %d : %q, attendu %q", i, tk.label, want[i])
		}
	}
}

func TestSetEnvIgnoresCase(t *testing.T) {
	env := setEnv([]string{"Path=a", "X=1"}, "PATH", "b")
	if !reflect.DeepEqual(env, []string{"X=1", "Path=b"}) {
		t.Fatalf("obtenu %v", env)
	}
}
