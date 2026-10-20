package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ctxFor(code int, git string) promptContext {
	return promptContext{User: "rayan", Host: "PC", Cwd: filepath.Join("/h", "Desktop", "vscode"), Home: filepath.FromSlash("/h"),
		Git: git, Time: "14:05", Code: code}
}

func TestThemesRenderWithoutColor(t *testing.T) {
	want := map[string]string{
		"classique":  "\nrayan@PC WIN ~/Desktop/vscode\n$ ",
		"minimal":    "\nvscode ❯ ",
		"chemin":     "\n~/Desktop/vscode main ❯ ",
		"git":        "\nrayan@PC ~/Desktop/vscode (main)\n$ ",
		"fleche":     "\n╭─ rayan dans ~/Desktop/vscode sur main à 14:05\n╰─❯ ",
		"blocs":      "\n vscode  main  ❯ ",
		"powershell": "\nPS " + filepath.Join("/h", "Desktop", "vscode") + "> ",
	}
	for name, w := range want {
		th, ok := themeByName(name)
		if !ok {
			t.Fatalf("thème %s absent", name)
		}
		if got := renderPrompt(th.Format, ctxFor(0, "main")); got != w {
			t.Errorf("%s :\n%q\nattendu\n%q", name, got, w)
		}
	}
}

func TestOptionalGroups(t *testing.T) {
	th, _ := themeByName("git")
	if got := renderPrompt(th.Format, ctxFor(0, "")); strings.Contains(got, "(") {
		t.Fatalf("hors d'un dépôt Git, pas de () : %q", got)
	}
	th, _ = themeByName("fleche")
	if got := renderPrompt(th.Format, ctxFor(2, "")); !strings.Contains(got, "✗ 2") || strings.Contains(got, " sur ") {
		t.Fatalf("code d'erreur affiché, branche absente : %q", got)
	}
	if got := renderPrompt("{inconnu} [[pas fermé", ctxFor(0, "")); got != "{inconnu} [[pas fermé" {
		t.Fatalf("un format mal écrit reste visible : %q", got)
	}
}

func TestAdminAndColors(t *testing.T) {
	c := ctxFor(1, "")
	c.Admin, c.Color = true, true
	got := renderPrompt("{admin} {symbole}", c)
	if !strings.Contains(got, "ADMIN") || !strings.Contains(got, "\x1b[1;31m#") {
		t.Fatalf("obtenu %q", got)
	}
	if w := visibleWidth(got); w != len("ADMIN # ") {
		t.Fatalf("largeur visible %d", w)
	}
	if w := visibleWidth("\x1b[90m╰─\x1b[0m\x1b[1;32m❯\x1b[0m "); w != 4 {
		t.Fatalf("╰─❯ : largeur %d, attendu 4", w)
	}
}

func TestGitBranch(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/feature/prompt\n"), 0o644)
	sub := filepath.Join(root, "src", "a")
	os.MkdirAll(sub, 0o755)
	if b := gitBranch(sub); b != "feature/prompt" {
		t.Fatalf("branche : %q", b)
	}
	os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("4c19243abcdef\n"), 0o644)
	if b := gitBranch(root); b != "4c19243" {
		t.Fatalf("HEAD détachée : %q", b)
	}
	// Worktree / sous-module : .git est un fichier
	wt := t.TempDir()
	os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+filepath.Join(root, ".git")+"\n"), 0o644)
	if b := gitBranch(wt); b != "4c19243" {
		t.Fatalf("worktree : %q", b)
	}
}

func TestPromptCommandSavesChoice(t *testing.T) {
	s, out, errOut, _ := testShell(t)
	cfg := filepath.Join(t.TempDir(), "terminal-config.json")
	old := configPath
	configPath = func() string { return cfg }
	t.Cleanup(func() { configPath = old })
	os.WriteFile(cfg, []byte(`{"autre_reglage": true}`), 0o644)

	if code := s.RunLine("prompt"); code != 0 || !strings.Contains(out.String(), "blocs") {
		t.Fatalf("liste : %d %q", code, out)
	}
	if code := s.RunLine("prompt minimal"); code != 0 {
		t.Fatalf("code %d : %s", code, errOut)
	}
	if tail, w := s.promptTail(); tail != "❯ " && !strings.HasSuffix(tail, "❯ ") || w != visibleWidth(tail) {
		t.Fatalf("ligne de saisie %q, largeur %d", tail, w)
	}
	data, _ := os.ReadFile(cfg)
	if !strings.Contains(string(data), `"prompt": "minimal"`) || !strings.Contains(string(data), "autre_reglage") {
		t.Fatalf("réglages : %s", data)
	}
	if promptFormat(loadPromptConfig()) != s.promptFmt {
		t.Fatal("le choix doit être relu par les prochaines fenêtres")
	}

	s.RunLine(`prompt perso "{dossier} >> "`)
	if !strings.HasSuffix(s.prompt(), "work >> ") {
		t.Fatalf("perso : %q", s.prompt())
	}
	if code := s.RunLine("prompt nimporte"); code != 1 || !strings.Contains(errOut.String(), "style inconnu") {
		t.Fatalf("style inconnu : %d %q", code, errOut)
	}
}
