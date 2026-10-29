package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultScript(t *testing.T) {
	dir, r, out := project(t, `{"default": "b", "scripts": {"a": "echo A", "b": "echo B"}}`)
	if code := Main(r, dir, nil); code != 0 || !strings.Contains(out.String(), "B\n") || strings.Contains(out.String(), "A\n") {
		t.Fatalf("run seul doit lancer le script par défaut : code %d, %s", code, out)
	}
	out.Reset()
	if code := Main(r, dir, []string{"--list"}); code != 0 || strings.Contains(out.String(), "> b") || !strings.Contains(out.String(), "run seul lance « b »") {
		t.Fatalf("run --list doit lister sans lancer : %s", out)
	}
	out.Reset()
	if code := Main(r, dir, []string{"--list", "a"}); code != 2 {
		t.Fatalf("run --list a : code %d, %s", code, out)
	}
}

func TestDefaultErrors(t *testing.T) {
	for _, tc := range []struct{ json, want string }{
		{`{"default": "x", "scripts": {"a": "y"}}`, `"x" n'existe pas`},
		{`{"default": 3, "scripts": {"a": "y"}}`, `attendu le nom d'un script`},
	} {
		if _, err := Parse([]byte(tc.json)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s : erreur %v, attendu %q", tc.json, err, tc.want)
		}
	}
	if f, err := Parse([]byte(`{"default": null, "scripts": {"a": "y"}}`)); err != nil || f.Default != "" {
		t.Fatalf("default null : %v %+v", err, f)
	}
}

func TestExportCreatesPackageJSON(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {"up": "make up", "dev": {"parallel": ["run up", "make logs"]}}}`)
	if code := Main(r, dir, []string{"--export"}); code != 0 {
		t.Fatalf("code %d : %s", code, out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	want := `{
  "private": true,
  "scripts": {
    "up": "terminal run up",
    "dev": "terminal run dev"
  }
}
`
	if string(got) != want {
		t.Fatalf("package.json :\n%s\nattendu :\n%s", got, want)
	}
	if !strings.Contains(out.String(), "package.json créé avec 2 script(s)") || !strings.Contains(out.String(), "npm run up") {
		t.Fatalf("sortie : %s", out)
	}
}

func TestExportMergesKeepsOrderIndentAndSkipsLoops(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {
	  "dev": "npm run dev",
	  "all": {"parallel": ["run dev", "make logs"]},
	  "up": "make up",
	  "lint": "eslint ."
	}}`)
	pkg := filepath.Join(dir, "package.json")
	os.WriteFile(pkg, []byte("{\r\n\t\"name\": \"app\",\r\n\t\"scripts\": {\r\n\t\t\"dev\": \"vite\",\r\n\t\t\"lint\": \"eslint src\"\r\n\t},\r\n\t\"dependencies\": {\r\n\t\t\"vite\": \"^5.0.0\"\r\n\t}\r\n}\r\n"), 0o644)

	if code := Main(r, dir, []string{"--export"}); code != 0 {
		t.Fatalf("code %d : %s", code, out)
	}
	got, _ := os.ReadFile(pkg)
	want := "{\r\n\t\"name\": \"app\",\r\n\t\"scripts\": {\r\n\t\t\"dev\": \"vite\",\r\n\t\t\"lint\": \"eslint src\",\r\n\t\t\"all\": \"terminal run all\",\r\n\t\t\"up\": \"terminal run up\"\r\n\t},\r\n\t\"dependencies\": {\r\n\t\t\"vite\": \"^5.0.0\"\r\n\t}\r\n}\r\n"
	if string(got) != want {
		t.Fatalf("package.json :\n%q\nattendu :\n%q", got, want)
	}
	for _, w := range []string{"Ajouté(s) à package.json : all, up", "gardé(s) : lint", "Pas exporté(s) : dev "} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("%q manquant :\n%s", w, out)
		}
	}

	// --force remplace lint, mais jamais dev (boucle npm run dev → terminal run dev → npm run dev).
	out.Reset()
	if code := Main(r, dir, []string{"--export", "--force"}); code != 0 {
		t.Fatalf("code %d : %s", code, out)
	}
	got, _ = os.ReadFile(pkg)
	if !strings.Contains(string(got), `"lint": "terminal run lint"`) || !strings.Contains(string(got), `"dev": "vite"`) {
		t.Fatalf("package.json après --force :\n%s", got)
	}

	// Une seconde fois : plus rien à faire, fichier inchangé.
	out.Reset()
	before, _ := os.ReadFile(pkg)
	Main(r, dir, []string{"--export"})
	after, _ := os.ReadFile(pkg)
	if string(before) != string(after) || !strings.Contains(out.String(), "Rien à ajouter") {
		t.Fatalf("second export : %s", out)
	}
}

func TestExportOptionErrors(t *testing.T) {
	dir, r, out := project(t, `{"scripts": {"a": "echo A"}}`)
	for _, args := range [][]string{{"--force"}, {"--export", "a"}, {"-p", "--export", "a"}} {
		out.Reset()
		if code := Main(r, dir, args); code != 2 {
			t.Errorf("%v : code %d, %s", args, code, out)
		}
	}
}

func TestRunsPackageScript(t *testing.T) {
	for _, tc := range []struct {
		line, target string
		want         bool
	}{
		{"npm run dev", "dev", true},
		{"npm run-script dev -- --port 3000", "dev", true},
		{"npm start", "start", true},
		{"yarn dev", "dev", true},
		{"pnpm run build", "build", true},
		{"npm run build", "dev", false},
		{"npm install", "install", false},
		{"make dev", "dev", false},
	} {
		if got := runsPackageScript(strings.Fields(tc.line), tc.target); got != tc.want {
			t.Errorf("%q / %s : %v", tc.line, tc.target, got)
		}
	}
}
