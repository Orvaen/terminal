package shell

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParsePipeline(t *testing.T) {
	got, err := parsePipeline(`cat "a | b.txt" | grep -E 'x|y' | sort > tri.txt 2>&1`)
	if err != nil {
		t.Fatal(err)
	}
	want := []stage{
		{text: `cat "a | b.txt"`},
		{text: `grep -E 'x|y'`},
		{text: "sort", out: "tri.txt", errToOut: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu %#v", got)
	}

	got, err = parsePipeline(`sort < "mes données.txt" >> sortie.log 2> erreurs.log`)
	if err != nil {
		t.Fatal(err)
	}
	want = []stage{{text: "sort", in: "mes données.txt", out: "sortie.log", outAppend: true, err: "erreurs.log"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu %#v", got)
	}

	got, _ = parsePipeline(`make &> build.log`)
	if got[0].out != "build.log" || !got[0].bothFile || got[0].text != "make" {
		t.Fatalf("&> : %#v", got)
	}
	got, _ = parsePipeline(`echo 12>f`)
	if got[0].text != "echo 12" || got[0].out != "f" {
		t.Fatalf("12>f : le 2 fait partie du mot, obtenu %#v", got)
	}
}

func TestParsePipelineFallbacksAndErrors(t *testing.T) {
	for _, line := range []string{`echo $env:PATH`, "echo `date`", `Get-Process | Select -First 3 &`, `ls 1>&2`} {
		if _, err := parsePipeline(line); !errors.Is(err, errNeedsPowerShell) {
			t.Errorf("%q : PowerShell attendu, obtenu %v", line, err)
		}
	}
	if _, err := parsePipeline(`grep '$'`); err != nil {
		t.Errorf("un $ entre guillemets ne doit pas partir dans PowerShell : %v", err)
	}
	for _, line := range []string{`| grep x`, `ls |`, `ls >`, `sort <`, `cat "x`} {
		if _, err := parsePipeline(line); err == nil || errors.Is(err, errNeedsPowerShell) {
			t.Errorf("%q : erreur de syntaxe attendue, obtenu %v", line, err)
		}
	}
}

func skipNoUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("utilise sort, grep et sh")
	}
}

func TestPipelineBuiltinIntoProgram(t *testing.T) {
	skipNoUnix(t)
	s, out, errOut, _ := testShell(t)
	if code := s.RunLine(`echo "b\na" | sort`); code != 0 {
		t.Fatalf("code %d : %s", code, errOut)
	}
	// echo intégré n'interprète pas \n : une seule ligne
	if got := out.String(); got != "b\\na\n" {
		t.Fatalf("obtenu %q", got)
	}
}

func TestPipelineRedirections(t *testing.T) {
	skipNoUnix(t)
	s, out, errOut, work := testShell(t)
	os.WriteFile(filepath.Join(work, "noms.txt"), []byte("zoé\nali\nmax\nali\n"), 0o644)

	if code := s.RunLine(`sort < noms.txt | uniq -c | sort -rn > compte.txt`); code != 0 {
		t.Fatalf("code %d : %s", code, errOut)
	}
	data, _ := os.ReadFile(filepath.Join(work, "compte.txt"))
	if first := strings.Fields(strings.SplitN(string(data), "\n", 2)[0]); len(first) != 2 || first[0] != "2" || first[1] != "ali" {
		t.Fatalf("compte.txt : %q", data)
	}
	if out.Len() != 0 {
		t.Fatalf("rien ne doit s'afficher : %q", out)
	}

	s.RunLine(`echo un > log.txt && echo deux >> log.txt`)
	if data, _ := os.ReadFile(filepath.Join(work, "log.txt")); string(data) != "un\ndeux\n" {
		t.Fatalf("log.txt : %q", data)
	}

	s.RunLine(`sh -c "echo sortie; echo erreur >&2" > tout.txt 2>&1`)
	if data, _ := os.ReadFile(filepath.Join(work, "tout.txt")); !strings.Contains(string(data), "sortie") || !strings.Contains(string(data), "erreur") {
		t.Fatalf("2>&1 : %q", data)
	}
	if code := s.RunLine(`grep absent noms.txt | wc -l`); code != 0 {
		t.Fatalf("le code est celui de la dernière commande, obtenu %d", code)
	}
}

func TestPipelineLsOnePerLine(t *testing.T) {
	skipNoUnix(t)
	s, out, _, work := testShell(t)
	for _, n := range []string{"a.txt", "b.txt", "c.go"} {
		os.WriteFile(filepath.Join(work, n), nil, 0o644)
	}
	s.RunLine(`ls | grep txt`)
	if got := out.String(); got != "a.txt\nb.txt\n" {
		t.Fatalf("obtenu %q", got)
	}
}

func TestPipelineMissingInput(t *testing.T) {
	skipNoUnix(t)
	s, _, errOut, _ := testShell(t)
	if code := s.RunLine(`sort < absent.txt`); code != 1 || !strings.Contains(errOut.String(), "absent.txt") {
		t.Fatalf("code %d : %s", code, errOut)
	}
}

func TestNeedsCoreutils(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"ls", []string{"-la"}, false},
		{"ls", []string{"--color=auto"}, true},
		{"ls", []string{"-lt"}, true},
		{"rm", []string{"-rf", "x"}, false},
		{"rm", []string{"-i", "x"}, true},
		{"cat", []string{"a.txt"}, false},
		{"cat", nil, false},
		{"cat", []string{"-n", "a"}, true},
		{"echo", []string{"-n", "x"}, false},
		{"echo", []string{"-e", `a\tb`}, true},
		{"echo", []string{"-x"}, false},
		{"env", nil, false},
		{"env", []string{"A=1", "node"}, true},
		{"cd", []string{"--x"}, false},
		{"ls", []string{"--", "--weird"}, false},
	} {
		if got := needsCoreutils(tc.name, tc.args); got != tc.want {
			t.Errorf("%s %v : %v, attendu %v", tc.name, tc.args, got, tc.want)
		}
	}
}

func TestCatReadsInput(t *testing.T) {
	skipNoUnix(t)
	s, out, errOut, work := testShell(t)
	os.WriteFile(filepath.Join(work, "f.txt"), []byte("ligne\n"), 0o644)
	if code := s.RunLine(`cat < f.txt | cat`); code != 0 || out.String() != "ligne\n" {
		t.Fatalf("code %d, %q %q", code, out, errOut)
	}
}

// fakeCoreutils installe un faux Coreutils (scripts sh) et le déclare détecté.
func fakeCoreutils(t *testing.T, multicall bool) string {
	t.Helper()
	skipNoUnix(t)
	dir := t.TempDir()
	if multicall {
		os.WriteFile(filepath.Join(dir, "coreutils"), []byte("#!/bin/sh\necho \"COREUTILS $*\"\n"), 0o755)
	} else {
		os.WriteFile(filepath.Join(dir, "coreutils-manager"), []byte("#!/bin/sh\n"), 0o755)
		for _, n := range []string{"ls", "cat", "rm"} {
			os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\necho \"COREUTILS "+n+" $*\"\n"), 0o755)
		}
	}
	core.Lock()
	old := core.dir
	core.dir = dir
	core.Unlock()
	t.Cleanup(func() { core.Lock(); core.dir = old; core.Unlock() })
	return dir
}

func TestFindCoreutilsDir(t *testing.T) {
	dir := fakeCoreutils(t, false)
	notFound := func(string) (string, error) { return "", errors.New("absent") }
	if got := findCoreutilsDir(notFound, []string{t.TempDir(), dir}, nil); got != dir {
		t.Fatalf("trouvé via le PATH du registre : %q", got)
	}
	if got := findCoreutilsDir(notFound, nil, []string{t.TempDir()}); got != "" {
		t.Fatalf("rien attendu, obtenu %q", got)
	}
	viaPath := func(n string) (string, error) { return filepath.Join(dir, n), nil }
	if got := findCoreutilsDir(viaPath, nil, nil); got != dir {
		t.Fatalf("trouvé via le PATH : %q", got)
	}
}

func TestBuiltinsDelegateToCoreutils(t *testing.T) {
	fakeCoreutils(t, false)
	s, out, _, work := testShell(t)
	os.WriteFile(filepath.Join(work, "a.txt"), []byte("contenu\n"), 0o644)

	s.RunLine(`ls --color=auto`)
	if !strings.Contains(out.String(), "COREUTILS ls --color=auto") {
		t.Fatalf("ls --color doit passer par Coreutils : %q", out)
	}
	out.Reset()
	s.RunLine(`cat a.txt`)
	if out.String() != "contenu\n" {
		t.Fatalf("cat simple : commande intégrée attendue, obtenu %q", out)
	}
	out.Reset()
	s.RunLine(`cat -n a.txt | cat`)
	if !strings.Contains(out.String(), "COREUTILS cat -n a.txt") {
		t.Fatalf("cat -n : Coreutils attendu, obtenu %q", out)
	}
}

func TestCoreutilsMulticallAndRmGuard(t *testing.T) {
	fakeCoreutils(t, true)
	s, out, errOut, _ := testShell(t)
	s.RunLine(`ls -lt`)
	if !strings.Contains(out.String(), "COREUTILS ls -lt") {
		t.Fatalf("binaire multi-appel : %q", out)
	}
	out.Reset()
	if code := s.RunLine(`rm -ri ~`); code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "refus") {
		t.Fatalf("rm du dossier perso doit être refusé même via Coreutils : code %d, %q %q", code, out, errOut)
	}
}

func TestWithFirst(t *testing.T) {
	sep := string(os.PathListSeparator)
	got := withFirst(strings.Join([]string{"/a", "/core", "/b"}, sep), "/core")
	if got != strings.Join([]string{"/core", "/a", "/b"}, sep) {
		t.Fatalf("obtenu %q", got)
	}
}
