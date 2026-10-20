package shell

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// readWith tape `keys` dans un éditeur branché sur le shell s et renvoie la ligne lue.
func readWith(t *testing.T, s *Shell, keys string, history ...string) (string, string, error) {
	t.Helper()
	var out bytes.Buffer
	e := &editor{
		in:       bufio.NewReader(strings.NewReader(keys)),
		out:      &out,
		width:    func() int { return 80 },
		complete: s.complete,
		history:  history,
	}
	line, err := e.readLine("$ ", 2)
	return line, out.String(), err
}

func fakeInstalled(s *Shell) {
	s.installed = func() []string {
		return []string{"git", "node", "vscode", "Git.Git", "Microsoft.VisualStudioCode", "Notepad++.Notepad++", "OpenJS.NodeJS.LTS", "Spotify.Spotify"}
	}
}

func TestEditorKeys(t *testing.T) {
	s := &Shell{}
	cases := []struct{ keys, want string }{
		{"abc\x1b[D\x1b[DX\r", "aXbc"},                             // ← ← puis insertion
		{"ab\x7fc\r", "ac"},                                        // Retour arrière
		{"hello\x1b[H>\x1b[F<\r", ">hello<"},                       // Début, Fin
		{"abcd\x1b[D\x1b[D\x1b[3~\r", "abd"},                       // Suppr
		{"apt remove git\x17node\r", "apt remove node"},            // Ctrl+W
		{"tout effacer\x15ok\r", "ok"},                             // Ctrl+U
		{"mot\x1bx\r", "x"},                                        // Échap efface la ligne
		{"é\x1b[D\x1b[Dà\r", "àé"},                                 // caractères accentués
		{"git commit\x1b[1;5D\x1b[1;5Dsudo \r", "sudo git commit"}, // Ctrl+←
	}
	for _, c := range cases {
		got, _, err := readWith(t, s, c.keys)
		if err != nil || got != c.want {
			t.Errorf("%q : obtenu %q (%v), attendu %q", c.keys, got, err, c.want)
		}
	}
}

func TestEditorHistory(t *testing.T) {
	s := &Shell{}
	got, _, _ := readWith(t, s, "\x1b[A\x1b[A\r", "premier", "second")
	if got != "premier" {
		t.Errorf("↑↑ : %q", got)
	}
	got, _, _ = readWith(t, s, "brouillon\x1b[A\x1b[B\r", "ancien")
	if got != "brouillon" {
		t.Errorf("↑↓ doit revenir au brouillon : %q", got)
	}
}

func TestEditorCtrlCAndEOF(t *testing.T) {
	s := &Shell{}
	if _, _, err := readWith(t, s, "commande\x03"); !errors.Is(err, errInterrupted) {
		t.Errorf("Ctrl+C : %v", err)
	}
	if _, _, err := readWith(t, s, "\x04"); !errors.Is(err, io.EOF) {
		t.Errorf("Ctrl+D sur ligne vide : %v", err)
	}
}

func TestTabAptRemove(t *testing.T) {
	s := &Shell{}
	fakeInstalled(s)

	got, _, _ := readWith(t, s, "apt remove nod\t\r")
	if got != "apt remove node " {
		t.Errorf("nom court unique : %q", got)
	}
	got, _, _ = readWith(t, s, "sudo apt remove spo\t\r")
	if got != "sudo apt remove Spotify.Spotify " {
		t.Errorf("identifiant winget, casse corrigée : %q", got)
	}
	got, _, _ = readWith(t, s, "apt purge vscode micro\t\r")
	if got != "apt purge vscode Microsoft.VisualStudioCode " {
		t.Errorf("deuxième paquet : %q", got)
	}
	// node et Notepad++ : rien de commun au-delà de « no »
	if got, _, _ = readWith(t, s, "apt remove no\t\r"); got != "apt remove no" {
		t.Errorf("ambigu : %q", got)
	}
	// git et Git.Git : le premier Tab complète le préfixe commun, le second liste les choix
	got, out, _ := readWith(t, s, "apt remove g\t\t\r")
	if got != "apt remove git" {
		t.Errorf("ambigu : %q", got)
	}
	if !strings.Contains(out, "git") || !strings.Contains(out, "Git.Git") {
		t.Errorf("le second Tab doit lister les choix : %q", out)
	}
}

func TestTabAptSubcommandsAndInstall(t *testing.T) {
	s := &Shell{}
	fakeInstalled(s)
	if got, _, _ := readWith(t, s, "apt rem\t\r"); got != "apt remove " {
		t.Errorf("sous-commande : %q", got)
	}
	if got, _, _ := readWith(t, s, "apt-get inst\t\r"); got != "apt-get install " {
		t.Errorf("apt-get : %q", got)
	}
	if got, _, _ := readWith(t, s, "apt install neo\t\r"); got != "apt install neovim " {
		t.Errorf("apt install propose les noms connus : %q", got)
	}
	if got, _, _ := readWith(t, s, "cd x && apt update; apt rem\t\r"); got != "cd x && apt update; apt remove " {
		t.Errorf("après un enchaînement : %q", got)
	}
	if got, _, _ := readWith(t, s, "docker engine st\t\r"); got != "docker engine st" {
		t.Errorf("docker engine st est ambigu (status, start, stop) : %q", got)
	}
	if got, _, _ := readWith(t, s, "docker engine sta\t\r"); got != "docker engine sta" {
		t.Errorf("status / start : %q", got)
	}
	if got, _, _ := readWith(t, s, "docker engine sto\t\r"); got != "docker engine stop " {
		t.Errorf("stop : %q", got)
	}
}

func TestTabPaths(t *testing.T) {
	s, _, _, work := testShell(t)
	os.MkdirAll(filepath.Join(work, "projets", "site web"), 0o755)
	os.WriteFile(filepath.Join(work, "projets", "notes.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(work, ".cache"), nil, 0o644)

	cases := []struct{ keys, want string }{
		{"cd pro\t\r", "cd projets/"},
		{"cd projets/s\t\r", `cd "projets/site web/`},
		{"cat projets/n\t\r", "cat projets/notes.txt "},
		{"cd projets/n\t\r", "cd projets/n"}, // cd : dossiers seulement
		{"ls .c\t\r", "ls .cache "},          // fichier caché seulement si on tape le point
	}
	for _, c := range cases {
		if got, _, _ := readWith(t, s, c.keys); got != c.want {
			t.Errorf("%q : obtenu %q, attendu %q", c.keys, got, c.want)
		}
	}
}

func TestSplitForCompletion(t *testing.T) {
	words, cur, start := splitForCompletion([]rune(`sudo apt remove "Mon Appli`))
	if !reflect.DeepEqual(words, []string{"sudo", "apt", "remove"}) || cur != "Mon Appli" || start != 16 {
		t.Fatalf("%q %q %d", words, cur, start)
	}
	words, cur, start = splitForCompletion([]rune("git status "))
	if len(words) != 2 || cur != "" || start != 11 {
		t.Fatalf("%q %q %d", words, cur, start)
	}
}

func TestRenderWrapsLongLines(t *testing.T) {
	var out bytes.Buffer
	e := &editor{out: &out, width: func() int { return 10 }}
	e.render("$ ", 2, []rune("abcdefghijklmnop"), 3) // 18 caractères sur des lignes de 10
	if e.row != 0 {
		t.Fatalf("curseur en colonne 5 de la première ligne, ligne %d", e.row)
	}
	e.render("$ ", 2, []rune("abcdefghijklmnop"), 16)
	if e.row != 1 {
		t.Fatalf("curseur sur la deuxième ligne attendu, ligne %d", e.row)
	}
	if !strings.Contains(out.String(), "\x1b[1A") {
		t.Fatal("le second rendu doit remonter d'une ligne avant de redessiner")
	}
}

func TestTabReq(t *testing.T) {
	s, _, _, work := testShell(t)
	os.WriteFile(filepath.Join(work, "api.http"), []byte("GET http://x"), 0o644)
	os.WriteFile(filepath.Join(work, "apropos.txt"), nil, 0o644)
	if got, _, _ := readWith(t, s, "req ap\t\r"); got != "req api.http " {
		t.Errorf("fichier .http : %q", got)
	}
	if got, _, _ := readWith(t, s, "req nou\t\r"); got != "req nouveau " {
		t.Errorf("sous-commande : %q", got)
	}
	if got, _, _ := readWith(t, s, "req liste a\t\r"); got != "req liste api.http " {
		t.Errorf("req liste : %q", got)
	}
}
