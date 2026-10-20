package apt

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOptimise : un PC simulé (variables utilisateur, Defender, outils installés).
type fakePC struct {
	env       map[string]string
	ps        []string // scripts PowerShell lancés
	elevated  [][]string
	installed []string
}

func fakeOptimise(t *testing.T, tools ...string) *fakePC {
	t.Helper()
	fakeWinget(t, nil)
	pc := &fakePC{env: map[string]string{}}
	state := filepath.Join(t.TempDir(), "optimisations.json")

	oldState, oldUE, oldSUE, oldPS := optStatePath, userEnv, setUserEnv, runPowerShell
	oldElev, oldIsElev, oldCPU, oldDef := Elevate, isElevated, numCPU, defenderActive
	oldAsk, oldSc, oldLook, oldDocker, oldMSVC := askLine, installSccache, toolLookPath, dockerInstalled, msvcCompilerInstalled
	t.Cleanup(func() {
		optStatePath, userEnv, setUserEnv, runPowerShell = oldState, oldUE, oldSUE, oldPS
		Elevate, isElevated, numCPU, defenderActive = oldElev, oldIsElev, oldCPU, oldDef
		askLine, installSccache, toolLookPath, dockerInstalled, msvcCompilerInstalled = oldAsk, oldSc, oldLook, oldDocker, oldMSVC
	})
	optStatePath = func() string { return state }
	userEnv = func(n string) (string, bool) { v, ok := pc.env[n]; return v, ok }
	setUserEnv = func(n, v string, remove bool) error {
		if remove {
			delete(pc.env, n)
		} else {
			pc.env[n] = v
		}
		return nil
	}
	runPowerShell = func(script string) (string, error) { pc.ps = append(pc.ps, script); return "True", nil }
	Elevate = func(args ...string) int {
		pc.elevated = append(pc.elevated, args)
		// la fenêtre administrateur relance apt optimiser --defender-…
		return Run(args)
	}
	isElevated = func() bool { return false }
	numCPU = func() int { return 8 }
	defenderActive = func() bool { return true }
	askLine = func(string) string { return "" }
	installSccache = func() int { pc.installed = append(pc.installed, "sccache"); return 0 }
	have := map[string]bool{}
	for _, tl := range tools {
		have[tl] = true
	}
	toolLookPath = func(n string) (string, error) {
		if have[n] {
			return "/bin/" + n, nil
		}
		return "", exec.ErrNotFound
	}
	dockerInstalled = func() bool { return have["docker"] }
	msvcCompilerInstalled = func() bool { return false }

	// des caches d'outils
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	os.MkdirAll(filepath.Join(local, "npm-cache"), 0o755)
	os.MkdirAll(filepath.Join(local, "go-build"), 0o755)
	for _, k := range []string{"ORVAEN_DOCKER_IDLE", "CMAKE_BUILD_PARALLEL_LEVEL", "MAKEFLAGS", "RUSTC_WRAPPER",
		"CMAKE_C_COMPILER_LAUNCHER", "CMAKE_CXX_COMPILER_LAUNCHER"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	return pc
}

func TestOptimiseAppliesAndUndoes(t *testing.T) {
	pc := fakeOptimise(t, "docker", "cmake", "make", "cargo")
	pc.env["RUSTC_WRAPPER"] = "ancien"

	if code := Run([]string{"optimiser", "-y"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	want := map[string]string{"ORVAEN_DOCKER_IDLE": "3600", "CMAKE_BUILD_PARALLEL_LEVEL": "8", "RUSTC_WRAPPER": "sccache"}
	for k, v := range want {
		if pc.env[k] != v || os.Getenv(k) != v {
			t.Errorf("%s = %q (terminal %q), attendu %q", k, pc.env[k], os.Getenv(k), v)
		}
	}
	if _, ok := pc.env["MAKEFLAGS"]; ok {
		t.Error("MAKEFLAGS ne doit pas être réglé sans un oui explicite")
	}
	if len(pc.installed) != 1 {
		t.Error("sccache aurait dû être installé")
	}
	// Defender : par la fenêtre administrateur, avec les caches existants
	if len(pc.elevated) != 1 || pc.elevated[0][1] != "--defender-ajouter" {
		t.Fatalf("élévation : %q", pc.elevated)
	}
	if len(pc.ps) != 1 || !strings.Contains(pc.ps[0], "Add-MpPreference -ExclusionPath") || !strings.Contains(pc.ps[0], "npm-cache") {
		t.Fatalf("PowerShell : %q", pc.ps)
	}

	// deuxième passage : tout est déjà fait, rien n'est relancé
	pc.elevated, pc.ps = nil, nil
	Run([]string{"optimiser", "-y"})
	if len(pc.elevated) != 0 {
		t.Errorf("Defender relancé alors que c'est déjà fait : %q", pc.elevated)
	}

	// annuler : tout revient comme avant
	if code := Run([]string{"optimiser", "annuler"}); code != 0 {
		t.Fatalf("annuler : code %d", code)
	}
	if pc.env["RUSTC_WRAPPER"] != "ancien" {
		t.Errorf("RUSTC_WRAPPER doit retrouver sa valeur d'avant : %q", pc.env["RUSTC_WRAPPER"])
	}
	for _, k := range []string{"ORVAEN_DOCKER_IDLE", "CMAKE_BUILD_PARALLEL_LEVEL"} {
		if _, ok := pc.env[k]; ok {
			t.Errorf("%s aurait dû être retiré", k)
		}
	}
	if len(pc.ps) != 1 || !strings.Contains(pc.ps[0], "Remove-MpPreference") {
		t.Fatalf("Defender non rétabli : %q", pc.ps)
	}
	if code := Run([]string{"optimiser", "annuler"}); code != 0 {
		t.Fatal("un second annuler ne doit rien faire")
	}
}

func TestOptimiseSkipsWhatDoesNotApply(t *testing.T) {
	pc := fakeOptimise(t) // aucun outil
	defenderActive = func() bool { return false }
	if code := Run([]string{"optimiser", "-y"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(pc.env) != 0 || len(pc.elevated) != 0 || len(pc.installed) != 0 {
		t.Fatalf("rien ne devait changer : %v %q %v", pc.env, pc.elevated, pc.installed)
	}
}

func TestOptimiseDefenderFailureIsNotRecorded(t *testing.T) {
	pc := fakeOptimise(t)
	runPowerShell = func(script string) (string, error) {
		pc.ps = append(pc.ps, script)
		return "", errors.New("accès refusé")
	}
	if code := Run([]string{"optimiser", "-y"}); code == 0 {
		t.Fatal("l'échec de Defender doit se voir dans le code de sortie")
	}
	if st := loadOptState(); len(st.Defender) != 0 {
		t.Fatalf("un échec ne doit pas être noté comme fait : %v", st.Defender)
	}
}
