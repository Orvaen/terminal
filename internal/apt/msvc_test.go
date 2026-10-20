package apt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Orvaen/terminal/internal/deps"
)

// fakeVS simule une installation de Visual Studio : avec ou sans MSVC, avec ou sans Spectre.
func fakeVS(t *testing.T, hasMSVC bool, hasSpectre *bool) *[][]string {
	t.Helper()
	var installer [][]string
	vswhere = func(requires ...string) string {
		if !hasMSVC {
			return ""
		}
		for _, r := range requires {
			if strings.Contains(r, "Spectre") && !*hasSpectre {
				return ""
			}
		}
		return `C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools`
	}
	runVSInstaller = func(args ...string) int {
		installer = append(installer, args)
		*hasSpectre = true
		return 0
	}
	return &installer
}

func TestBuildEssentialIncludesSpectre(t *testing.T) {
	calls := fakeWinget(t, nil)
	Run([]string{"install", "build-essential"})
	if len(*calls) != 1 {
		t.Fatalf("appels %v", *calls)
	}
	line := strings.Join((*calls)[0], " ")
	for _, want := range []string{"--add Microsoft.VisualStudio.Workload.VCTools", "--add Microsoft.VisualStudio.Component.VC.Runtimes.x86.x64.Spectre"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q manque dans %s", want, line)
		}
	}
}

func TestSpectreAddedToExistingMSVC(t *testing.T) {
	calls := fakeWinget(t, nil)
	spectre := false
	installer := fakeVS(t, true, &spectre)

	if code := Run([]string{"install", "msvc-spectre"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("winget ne doit pas réinstaller MSVC : %v", *calls)
	}
	got := strings.Join((*installer)[0], " ")
	want := `modify --installPath C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools --add Microsoft.VisualStudio.Component.VC.Runtimes.x86.x64.Spectre`
	if !strings.HasPrefix(got, want) || !strings.Contains(got, "--passive") {
		t.Fatalf("installeur : %s", got)
	}

	// Déjà là : rien à faire.
	Run([]string{"install", "spectre"})
	if len(*installer) != 1 {
		t.Fatalf("l'installeur ne doit pas être relancé : %v", *installer)
	}
	// build-essential sur un MSVC complet : rien à faire non plus.
	Run([]string{"install", "build-essential"})
	if len(*calls) != 0 || len(*installer) != 1 {
		t.Fatalf("winget %v, installeur %v", *calls, *installer)
	}
}

func TestDepsInstallFixesMSB8040AndRetries(t *testing.T) {
	fakeWinget(t, nil)
	spectre := false
	installer := fakeVS(t, true, &spectre)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{}`), 0o644)

	oldLook, oldRefresh, oldStep := deps.LookPath, refreshPath, runStep
	t.Cleanup(func() { deps.LookPath, refreshPath, runStep = oldLook, oldRefresh, oldStep })
	deps.LookPath = func(c string) (string, error) {
		if c == "node" {
			return "/bin/node", nil
		}
		return "", errors.New("absent")
	}
	refreshPath = func() {}
	attempts := 0
	runStep = func(dir string, args []string) (int, string) {
		attempts++
		if !spectre {
			return 1, "gyp ERR! ...\nerror MSB8040: Spectre-mitigated libraries are required for this project.\n"
		}
		return 0, ""
	}

	if code := Run([]string{"deps", "install", root, "-y"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if attempts != 2 || len(*installer) != 1 {
		t.Fatalf("essais %d, installeur %v", attempts, *installer)
	}
}

func TestOtherStepFailuresAreNotRetried(t *testing.T) {
	fakeWinget(t, nil)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)
	oldLook, oldStep := deps.LookPath, runStep
	t.Cleanup(func() { deps.LookPath, runStep = oldLook, oldStep })
	deps.LookPath = func(c string) (string, error) { return "/bin/" + c, nil }
	attempts := 0
	runStep = func(string, []string) (int, string) { attempts++; return 2, "réseau indisponible" }
	if code := Run([]string{"deps", "install", root, "-y"}); code != 2 || attempts != 1 {
		t.Fatalf("code %d, essais %d", code, attempts)
	}
}
