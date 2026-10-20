package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testShell crée un shell sans couleur dans un dossier temporaire servant aussi de dossier perso.
func testShell(t *testing.T) (*Shell, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	old, _ := os.Getwd()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var out, errOut bytes.Buffer
	s := &Shell{out: &out, errOut: &errOut, width: func() int { return 40 }}
	return s, &out, &errOut, work
}

func TestSplitChain(t *testing.T) {
	segs, err := splitChain(`cd "a;b" && ls || echo 'x && y'; git log | more`)
	if err != nil {
		t.Fatal(err)
	}
	want := []segment{
		{`cd "a;b"`, false, opAnd},
		{"ls", false, opOr},
		{"echo 'x && y'", false, opSeq},
		{"git log | more", true, opEnd},
	}
	if !reflect.DeepEqual(segs, want) {
		t.Fatalf("obtenu %#v", segs)
	}
	for _, bad := range []string{"&& ls", "ls &&", `echo "abc`} {
		if _, err := splitChain(bad); err == nil {
			t.Errorf("%q aurait dû échouer", bad)
		}
	}
}

func TestTokenize(t *testing.T) {
	got, err := tokenize(`cp "mon fichier.txt" ~/docs '~' a\b "dit \"oui\""`, "/h")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cp", "mon fichier.txt", "/h/docs", "~", `a\b`, `dit "oui"`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu %q", got)
	}
}

func TestJoinArgsRoundTrip(t *testing.T) {
	args := []string{"apt", "install", "mon paquet", `a"b`, "x&y"}
	line := joinArgs(args)
	segs, err := splitChain(line)
	if err != nil || len(segs) != 1 || segs[0].special {
		t.Fatalf("ligne %q mal reconstruite : %v %#v", line, err, segs)
	}
	got, _ := tokenize(segs[0].text, "")
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("obtenu %q", got)
	}
}

func TestConvertPath(t *testing.T) {
	cases := map[string]string{
		"/c/Users/Rayan": `C:\Users\Rayan`,
		"/d":             `D:\`,
		"src/app":        `src\app`,
		`C:\Windows`:     `C:\Windows`,
		"/cache":         `\cache`,
	}
	for in, want := range cases {
		if got := convertPath(in, "windows"); got != want {
			t.Errorf("convertPath(%q) = %q, attendu %q", in, got, want)
		}
	}
	if got := convertPath("/c/x", "linux"); got != "/c/x" {
		t.Errorf("hors Windows le chemin doit rester intact : %q", got)
	}
}

func TestDisplayPath(t *testing.T) {
	cases := []struct{ cwd, home, want string }{
		{`C:\Users\Rayan\Desktop`, `C:\Users\Rayan`, "~/Desktop"},
		{`c:\users\rayan`, `C:\Users\Rayan`, "~"},
		{`C:\Users\Rayane`, `C:\Users\Rayan`, "C:/Users/Rayane"},
		{`D:\projets`, `C:\Users\Rayan`, "D:/projets"},
	}
	for _, c := range cases {
		if got := displayPath(c.cwd, c.home); got != c.want {
			t.Errorf("displayPath(%q) = %q, attendu %q", c.cwd, got, c.want)
		}
	}
}

func TestFileBuiltins(t *testing.T) {
	s, out, errOut, work := testShell(t)

	run := func(line string, wantCode int) {
		t.Helper()
		if code := s.RunLine(line); code != wantCode {
			t.Fatalf("%q : code %d, attendu %d (stderr : %s)", line, code, wantCode, errOut.String())
		}
	}

	run("mkdir -p a/b/c && touch a/b/c/f.txt a/racine.txt", 0)
	run("cp -r a copie", 0)
	if _, err := os.Stat(filepath.Join(work, "copie", "b", "c", "f.txt")); err != nil {
		t.Fatalf("cp -r : %v", err)
	}
	run("mv copie/racine.txt renomme.txt", 0)
	if _, err := os.Stat(filepath.Join(work, "renomme.txt")); err != nil {
		t.Fatalf("mv : %v", err)
	}
	run("cp -r a a/dedans", 1) // copie d'un dossier dans lui-même refusée

	out.Reset()
	run("ls", 0)
	if got := out.String(); !strings.Contains(got, "a/") || !strings.Contains(got, "renomme.txt") {
		t.Fatalf("ls : %q", got)
	}

	run("rm a", 1) // dossier sans -r
	run("rm -rf a copie renomme.txt inexistant", 0)
	entries, _ := os.ReadDir(work)
	if len(entries) != 0 {
		t.Fatalf("rm -rf aurait dû tout supprimer, reste %d entrées", len(entries))
	}

	errOut.Reset()
	run("rm -rf ~", 1) // dossier perso protégé
	if !strings.Contains(errOut.String(), "refus") {
		t.Fatalf("message attendu : %q", errOut.String())
	}
}

func TestChainingSemantics(t *testing.T) {
	s, out, _, work := testShell(t)

	s.RunLine("cd dossier-inexistant && touch ne-doit-pas-exister")
	if _, err := os.Stat(filepath.Join(work, "ne-doit-pas-exister")); err == nil {
		t.Fatal("&& aurait dû s'arrêter après l'échec de cd")
	}
	out.Reset()
	s.RunLine("cd dossier-inexistant || echo secours; echo fin")
	if got := out.String(); got != "secours\nfin\n" {
		t.Fatalf("obtenu %q", got)
	}
}

func TestCdAndPwd(t *testing.T) {
	s, out, _, work := testShell(t)
	s.RunLine("mkdir sous && cd sous && pwd")
	if got := strings.TrimSpace(out.String()); !strings.HasSuffix(got, filepath.Join("work", "sous")) {
		t.Fatalf("pwd : %q", got)
	}
	s.RunLine("cd -")
	if cwd, _ := os.Getwd(); cwd != work {
		t.Fatalf("cd - : %q, attendu %q", cwd, work)
	}
	s.RunLine("cd")
	if cwd, _ := os.Getwd(); cwd != filepath.Dir(work) {
		t.Fatalf("cd sans argument doit aller au dossier perso : %q", cwd)
	}
}

func TestLsColumns(t *testing.T) {
	s, out, _, _ := testShell(t)
	s.RunLine("touch un deux trois quatre cinq six sept")
	out.Reset()
	s.RunLine("ls")
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) >= 7 || len(lines) == 0 {
		t.Fatalf("ls devrait afficher en colonnes : %q", out.String())
	}
	for _, l := range lines {
		if len(l) > 40 {
			t.Fatalf("ligne plus large que le terminal : %q", l)
		}
	}
}

func TestNeedsAdmin(t *testing.T) {
	if needsAdmin([]string{"apt", "install", "git"}) {
		t.Error("apt install ne doit pas demander les droits admin")
	}
	if !needsAdmin([]string{"apt", "full-upgrade"}) {
		t.Error("apt full-upgrade doit demander les droits admin")
	}
	if needsAdmin([]string{"docker", "ps"}) {
		t.Error("docker ne doit pas demander les droits admin")
	}
	if !needsAdmin([]string{"notepad", "hosts"}) {
		t.Error("une commande quelconque doit être élevée")
	}
}

func TestExit(t *testing.T) {
	s, out, _, _ := testShell(t)
	if code := s.RunLine("exit 3; echo jamais"); code != 3 || !s.exiting {
		t.Fatalf("code %d, exiting %v", code, s.exiting)
	}
	if out.Len() != 0 {
		t.Fatalf("rien ne devait s'exécuter après exit : %q", out.String())
	}
}

func TestGitUnixTools(t *testing.T) {
	root := t.TempDir()
	git := filepath.Join(root, "Git")
	os.MkdirAll(filepath.Join(git, "cmd"), 0o755)
	os.MkdirAll(filepath.Join(git, "usr", "bin"), 0o755)
	os.WriteFile(filepath.Join(git, "usr", "bin", "sh.exe"), nil, 0o755)
	want := filepath.Join(git, "usr", "bin")

	viaGit := func() (string, error) { return filepath.Join(git, "cmd", "git.exe"), nil }
	if got := gitUnixDir(viaGit, ""); got != want {
		t.Errorf("depuis git.exe : %q", got)
	}
	noGit := func() (string, error) { return "", os.ErrNotExist }
	if got := gitUnixDir(noGit, root); got != want {
		t.Errorf("depuis Program Files : %q", got)
	}
	if got := gitUnixDir(noGit, t.TempDir()); got != "" {
		t.Errorf("sans Git : %q", got)
	}

	sep := string(os.PathListSeparator)
	path := "/a" + sep + "/b"
	if got := withUnixTools(path, "/git/usr/bin"); got != path+sep+"/git/usr/bin" {
		t.Errorf("ajout en dernier : %q", got)
	}
	if got := withUnixTools(path+sep+"/git/usr/bin", "/git/usr/bin"); got != path+sep+"/git/usr/bin" {
		t.Errorf("pas de doublon : %q", got)
	}
}

func TestSetMakeShell(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sh.exe"), nil, 0o755)
	t.Setenv("SHELL", "")
	setMakeShell(dir)
	if got := os.Getenv("SHELL"); got != filepath.ToSlash(filepath.Join(dir, "sh.exe")) {
		t.Fatalf("SHELL = %q", got)
	}
	t.Setenv("SHELL", "C:/mon/shell.exe")
	setMakeShell(dir)
	if os.Getenv("SHELL") != "C:/mon/shell.exe" {
		t.Fatal("un SHELL choisi par l'utilisateur ne doit pas être remplacé")
	}
}
