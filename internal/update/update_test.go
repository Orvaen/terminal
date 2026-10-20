package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeGitHub sert une release r2 avec son zip x64 et ses sommes.
func fakeGitHub(t *testing.T, zipData []byte, sum string) *httptest.Server {
	t.Helper()
	const zipName = "orvaen-terminal-r2-x64.zip"
	if sum == "" {
		h := sha256.Sum256(zipData)
		sum = hex.EncodeToString(h[:])
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			json.NewEncoder(w).Encode(Release{Tag: "r2", Assets: []Asset{
				{Name: zipName, URL: srv.URL + "/dl/" + zipName},
				{Name: SumsFile, URL: srv.URL + "/dl/" + SumsFile},
			}})
		case "/dl/" + zipName:
			w.Write(zipData)
		case "/dl/" + SumsFile:
			fmt.Fprintf(w, "%s  %s\n", sum, zipName)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = old })
	return srv
}

func newRelease() map[string]string {
	return map[string]string{
		"terminal.exe": "terminal r2", "apt.exe": "apt r2", "docker.exe": "docker r2",
		"req.exe": "req r2", "terminal.ico": "icone",
	}
}

func installOld(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"terminal.exe", "apt.exe", "apt-get.exe", "docker.exe", "req.exe"} {
		os.WriteFile(filepath.Join(dir, n), []byte(n+" r1"), 0o755)
	}
	return dir
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func latest(t *testing.T) Release {
	t.Helper()
	rel, err := Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

func TestApplyReplacesFilesAndKeepsOld(t *testing.T) {
	fakeGitHub(t, makeZip(t, newRelease()), "")
	dir := installOld(t)

	if err := Apply(latest(t), dir, "amd64"); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		"terminal.exe": "terminal r2", "apt.exe": "apt r2", "apt-get.exe": "apt r2",
		"docker.exe": "docker r2", "req.exe": "req r2", "terminal.ico": "icone",
	} {
		if got := read(t, filepath.Join(dir, file)); got != want {
			t.Errorf("%s : %q, attendu %q", file, got, want)
		}
	}
	olds, _ := filepath.Glob(filepath.Join(dir, "*.old"))
	if len(olds) != 5 {
		t.Fatalf("5 anciennes versions attendues, obtenu %v", olds)
	}

	Cleanup(dir)
	if olds, _ := filepath.Glob(filepath.Join(dir, "*.old")); len(olds) != 0 {
		t.Fatalf("les .old auraient dû être supprimés : %v", olds)
	}
}

func TestApplyRejectsBadChecksum(t *testing.T) {
	fakeGitHub(t, makeZip(t, newRelease()), strings.Repeat("0", 64))
	dir := installOld(t)

	err := Apply(latest(t), dir, "amd64")
	if err == nil || !strings.Contains(err.Error(), "somme de contrôle") {
		t.Fatalf("erreur de somme attendue, obtenu %v", err)
	}
	if got := read(t, filepath.Join(dir, "terminal.exe")); got != "terminal.exe r1" {
		t.Fatalf("l'ancienne version doit rester intacte, obtenu %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 5 {
		t.Fatalf("aucun fichier ne doit être ajouté, obtenu %d fichiers", len(entries))
	}
}

func TestApplyIncompleteZipChangesNothing(t *testing.T) {
	files := newRelease()
	delete(files, "req.exe")
	fakeGitHub(t, makeZip(t, files), "")
	dir := installOld(t)

	if err := Apply(latest(t), dir, "amd64"); err == nil {
		t.Fatal("un zip sans req.exe doit être refusé")
	}
	if got := read(t, filepath.Join(dir, "apt.exe")); got != "apt.exe r1" {
		t.Fatalf("apt.exe ne doit pas changer, obtenu %q", got)
	}
	if news, _ := filepath.Glob(filepath.Join(dir, "*.new")); len(news) != 0 {
		t.Fatalf("les fichiers temporaires doivent être supprimés : %v", news)
	}
}

func TestApplyNoZipForArch(t *testing.T) {
	fakeGitHub(t, makeZip(t, newRelease()), "")
	if err := Apply(latest(t), t.TempDir(), "arm64"); err == nil {
		t.Fatal("pas de zip arm64 : erreur attendue")
	}
}

func TestReadSumsAcceptsBinaryMarker(t *testing.T) {
	p := filepath.Join(t.TempDir(), SumsFile)
	os.WriteFile(p, []byte("ABCDEF *a-x64.zip\r\n123  b-arm64.zip\n\n"), 0o644)
	sums, err := readSums(p)
	if err != nil {
		t.Fatal(err)
	}
	if sums["a-x64.zip"] != "abcdef" || sums["b-arm64.zip"] != "123" {
		t.Fatalf("sommes lues : %v", sums)
	}
}

func TestAvailableUsesSavedCheck(t *testing.T) {
	state := filepath.Join(t.TempDir(), "update.json")
	old := StatePath
	StatePath = func() string { return state }
	t.Cleanup(func() { StatePath = old })

	if _, ok := Available("r1"); ok {
		t.Fatal("aucune vérification enregistrée : rien à proposer")
	}
	saveCheck("r2")
	if v, ok := Available("r1"); !ok || v != "r2" {
		t.Fatalf("r2 attendue, obtenu %q %v", v, ok)
	}
	if _, ok := Available("r2"); ok {
		t.Fatal("déjà à jour : rien à proposer")
	}
	if _, ok := Available("dev"); ok {
		t.Fatal("une compilation locale ne doit pas afficher d'avis")
	}
}

func TestCleanupKeepsRecentStaging(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "terminal.exe.new"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "terminal.exe.abc.old"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "terminal.exe"), nil, 0o644)
	Cleanup(dir)
	if _, err := os.Stat(filepath.Join(dir, "terminal.exe.new")); err != nil {
		t.Fatal("un .new récent (mise à jour en cours) doit être gardé")
	}
	if _, err := os.Stat(filepath.Join(dir, "terminal.exe.abc.old")); err == nil {
		t.Fatal("le .old doit être supprimé")
	}
	if _, err := os.Stat(filepath.Join(dir, "terminal.exe")); err != nil {
		t.Fatal("terminal.exe ne doit pas être touché")
	}
}
