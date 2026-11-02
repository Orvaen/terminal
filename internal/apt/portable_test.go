package apt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// fakePortable : pas administrateur, node/go absents, nodejs.org et go.dev imités.
func fakePortable(t *testing.T, elevated bool) (calls *[][]string, pathAdds *[]string) {
	t.Helper()
	calls = fakeWinget(t, nil)
	nodeZip := zipWith(t, map[string]string{
		"node-v26.13.0-win-" + nodeZipArch() + "/node.exe": "node",
		"node-v26.13.0-win-" + nodeZipArch() + "/npm.cmd":  "@echo npm",
	})
	goZip := zipWith(t, map[string]string{"go/bin/go.exe": "go", "go/VERSION": "go1.26.3"})
	s256 := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	nodeName := "node-v26.13.0-win-" + nodeZipArch() + ".zip"
	goName := "go1.26.3.windows-" + runtime.GOARCH + ".zip"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/node/index.json":
			fmt.Fprintf(w, `[{"version":"v27.0.0","lts":false,"files":["win-%[1]s-zip"]},{"version":"v26.13.0","lts":"Krypton","files":["win-%[1]s-zip","win-%[1]s-msi"]}]`, nodeZipArch())
		case "/node/v26.13.0/SHASUMS256.txt":
			fmt.Fprintf(w, "%s  %s\n", s256(nodeZip), nodeName)
		case "/node/v26.13.0/" + nodeName:
			w.Write(nodeZip)
		case "/go/json":
			fmt.Fprintf(w, `[{"version":"go1.27rc1","stable":false,"files":[]},{"version":"go1.26.3","stable":true,"files":[{"filename":"%s","os":"windows","arch":"%s","kind":"archive","sha256":"%s"},{"filename":"go1.26.3.windows-%s.msi","os":"windows","arch":"%s","kind":"installer","sha256":"x"}]}]`,
				goName, runtime.GOARCH, s256(goZip), runtime.GOARCH, runtime.GOARCH)
		case "/go/dl/" + goName:
			w.Write(goZip)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	var adds []string
	oldE, oldEl, oldND, oldGD, oldGX := portableEnabled, isElevated, nodeDist, goDownloads, goDist
	oldLP, oldAdd, oldRm, oldPath := toolLookPath, addUserPath, removeUserPath, os.Getenv("PATH")
	t.Cleanup(func() {
		portableEnabled, isElevated, nodeDist, goDownloads, goDist = oldE, oldEl, oldND, oldGD, oldGX
		toolLookPath, addUserPath, removeUserPath = oldLP, oldAdd, oldRm
		os.Setenv("PATH", oldPath)
	})
	portableEnabled = true
	isElevated = func() bool { return elevated }
	nodeDist, goDownloads, goDist = srv.URL+"/node", srv.URL+"/go/json", srv.URL+"/go/dl/"
	toolLookPath = func(string) (string, error) { return "", errors.New("absent") }
	addUserPath = func(d string) error { adds = append(adds, d); return nil }
	removeUserPath = func(string) error { return nil }
	t.Setenv("APPDATA", t.TempDir())
	return calls, &adds
}

func TestInstallNpmWithoutAdminIsPortable(t *testing.T) {
	calls, adds := fakePortable(t, false)
	if code := Run([]string{"install", "npm"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("winget (et donc l'élévation) n'aurait pas dû être appelé : %v", *calls)
	}
	dir := filepath.Join(toolsRoot(), "node")
	if _, err := os.Stat(filepath.Join(dir, "npm.cmd")); err != nil {
		t.Fatalf("npm absent du dossier portable : %v", err)
	}
	if v, _ := os.ReadFile(filepath.Join(dir, ".version")); string(v) != "v26.13.0\n" {
		t.Fatalf("version %q", v)
	}
	want := []string{dir, filepath.Join(os.Getenv("APPDATA"), "npm")}
	if !reflect.DeepEqual(*adds, want) {
		t.Fatalf("PATH : %v, attendu %v", *adds, want)
	}

	// Déjà là : pas de nouveau téléchargement, et apt remove le retire sans winget.
	if code := Run([]string{"install", "node"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if code := Run([]string{"remove", "nodejs"}); code != 0 || len(*calls) != 0 {
		t.Fatalf("remove : code %d, winget %v", code, *calls)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("le dossier portable aurait dû être supprimé")
	}
}

func TestInstallGoWithoutAdminIsPortable(t *testing.T) {
	calls, _ := fakePortable(t, false)
	if code := Run([]string{"install", "golang"}); code != 0 || len(*calls) != 0 {
		t.Fatalf("code %d, winget %v", code, *calls)
	}
	if _, err := os.Stat(filepath.Join(toolsRoot(), "go", "bin", "go.exe")); err != nil {
		t.Fatal(err)
	}
}

func TestInstallAsAdminKeepsWinget(t *testing.T) {
	calls, _ := fakePortable(t, true)
	Run([]string{"install", "node"})
	want := [][]string{{"install", "--id", "OpenJS.NodeJS.LTS", "--exact", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v", *calls)
	}
}

func TestInstallWithoutAdminTriesUserScope(t *testing.T) {
	calls, _ := fakePortable(t, false)
	Run([]string{"install", "Spotify.Spotify"})
	want := [][]string{{"install", "Spotify.Spotify", "--scope", "user", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v", *calls)
	}
}

func TestUserScopeFallsBackToMachine(t *testing.T) {
	calls, _ := fakePortable(t, false)
	winget = func(args ...string) int {
		*calls = append(*calls, args)
		for _, a := range args {
			if a == "--scope" {
				return int(int32(-1978335216)) // 0x8A150010
			}
		}
		return 0
	}
	if code := Run([]string{"install", "Spotify.Spotify"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*calls) != 2 || reflect.DeepEqual((*calls)[1], (*calls)[0]) {
		t.Fatalf("appels %v", *calls)
	}
}
