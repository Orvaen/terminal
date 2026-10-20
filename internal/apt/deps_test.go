package apt

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Orvaen/terminal/internal/deps"
)

func TestDepsInstallInstallsMissingToolsThenProjectDeps(t *testing.T) {
	calls := fakeWinget(t, nil)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "Makefile"), []byte("all:\n\techo ok\n"), 0o644)
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"engines":{"node":">=20"}}`), 0o644)
	os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{}`), 0o644)

	oldLook, oldRefresh, oldStep, oldDocker := deps.LookPath, refreshPath, runStep, dockerInstalled
	t.Cleanup(func() { deps.LookPath, refreshPath, runStep, dockerInstalled = oldLook, oldRefresh, oldStep, oldDocker })
	deps.LookPath = func(string) (string, error) { return "", errors.New("absent") }
	dockerInstalled = func() bool { return false }
	refreshPath = func() {}
	var ran []string
	runStep = func(dir string, args []string) int {
		ran = append(ran, filepath.Base(dir)+": "+strings.Join(args, " "))
		return 0
	}

	if code := Run([]string{"deps", "install", root, "-y"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	want := [][]string{
		{"install", "--id", "Git.Git", "--exact", acceptSource, acceptPackage},
		{"install", "--id", "ezwinports.make", "--exact", acceptSource, acceptPackage},
		{"install", "--id", "OpenJS.NodeJS.LTS", "--exact", acceptSource, acceptPackage},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("installations : %v", *calls)
	}
	if !reflect.DeepEqual(ran, []string{filepath.Base(root) + ": npm ci"}) {
		t.Fatalf("dépendances du projet : %v", ran)
	}
}

func TestDepsListInstallsNothing(t *testing.T) {
	calls := fakeWinget(t, nil)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.23\n"), 0o644)
	oldLook := deps.LookPath
	t.Cleanup(func() { deps.LookPath = oldLook })
	deps.LookPath = func(string) (string, error) { return "", errors.New("absent") }

	if code := Run([]string{"deps", root}); code != 0 || len(*calls) != 0 {
		t.Fatalf("apt deps n'installe rien : code %d, appels %v", code, *calls)
	}
}

func TestDepsInstallSkipsCIOnlyToolsUnlessAsked(t *testing.T) {
	calls := fakeWinget(t, nil)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755)
	os.WriteFile(filepath.Join(root, ".github", "workflows", "ci.yml"),
		[]byte("jobs:\n  a:\n    steps:\n      - uses: actions/setup-dotnet@v4\n        with:\n          dotnet-version: '10.0'\n"), 0o644)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.23\n"), 0o644)

	oldLook, oldRefresh, oldStep, oldMSVC := deps.LookPath, refreshPath, runStep, msvcInstalled
	t.Cleanup(func() { deps.LookPath, refreshPath, runStep, msvcInstalled = oldLook, oldRefresh, oldStep, oldMSVC })
	deps.LookPath = func(string) (string, error) { return "", errors.New("absent") }
	refreshPath = func() {}
	runStep = func(string, []string) int { return 0 }
	msvcInstalled = func() bool { return false }

	Run([]string{"deps", "install", root, "-y"})
	if len(*calls) != 1 || (*calls)[0][2] != "GoLang.Go" {
		t.Fatalf("seul Go (demandé par go.mod) doit être installé : %v", *calls)
	}
	*calls = nil
	Run([]string{"deps", "install", "--ci", root, "-y"})
	ids := []string{}
	for _, c := range *calls {
		ids = append(ids, c[2])
	}
	if strings.Join(ids, ",") != "GoLang.Go,Microsoft.DotNet.SDK.10" {
		t.Fatalf("avec --ci, .NET aussi : %v", ids)
	}
}
