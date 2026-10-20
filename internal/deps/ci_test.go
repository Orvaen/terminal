package deps

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func needsOf(t *testing.T, root string) map[string]Need {
	t.Helper()
	p, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]Need{}
	for _, n := range p.Needs {
		m[n.Key] = n
	}
	return m
}

func TestNativeModulesFromLockfile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", `{"name":"app"}`)
	write(t, root, "package-lock.json", `{"lockfileVersion":3,"packages":{
	  "": {"name":"app"},
	  "node_modules/node-pty": {"hasInstallScript": true, "dependencies": {"node-addon-api": "^7"}},
	  "node_modules/native-keymap": {"hasInstallScript": true},
	  "node_modules/fsevents": {"hasInstallScript": true, "os": ["darwin"]},
	  "node_modules/bufferutil": {"hasInstallScript": true, "dependencies": {"node-gyp-build": "^4"}},
	  "node_modules/husky": {"hasInstallScript": true},
	  "node_modules/@esbuild/win32-x64": {"hasInstallScript": true},
	  "node_modules/@playwright/browser-chromium": {"hasInstallScript": true},
	  "node_modules/a/node_modules/kerberos": {"hasInstallScript": true, "dependencies": {"bindings": "1", "prebuild-install": "7"}},
	  "node_modules/lodash": {}
	}}`)
	if got := nativeModules(root); !reflect.DeepEqual(got, []string{"kerberos", "native-keymap", "node-pty"}) {
		t.Fatalf("modules natifs : %v", got)
	}
	n := needsOf(t, root)
	if _, ok := n["msvc"]; !ok {
		t.Fatal("MSVC doit être demandé")
	}
	if !strings.Contains(n["python"].Sources[0], "modules natifs : kerberos, native-keymap, node-pty") {
		t.Fatalf("source : %v", n["python"].Sources)
	}
}

func TestNativeModulesFromNodeModules(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", `{}`)
	write(t, root, "node_modules/sqlite3/binding.gyp", "{}")
	write(t, root, "node_modules/@vscode/spdlog/binding.gyp", "{}")
	write(t, root, "node_modules/lodash/index.js", "")
	if got := nativeModules(root); !reflect.DeepEqual(got, []string{"@vscode/spdlog", "sqlite3"}) {
		t.Fatalf("obtenu %v", got)
	}
	plain := t.TempDir()
	write(t, plain, "package.json", `{}`)
	if n := needsOf(t, plain); n["msvc"].Key != "" || n["python"].Key != "" {
		t.Fatal("sans module natif, ni Python ni MSVC")
	}
}

func TestReadCIWorkflows(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".nvmrc", "22.11.0\n")
	write(t, root, ".github/workflows/ci.yml", `name: CI
on: [push]
jobs:
  build:
    runs-on: windows-latest
    strategy:
      matrix:
        python: ['3.10', "3.12", 3.11]
        go:
          - '1.22'
          - '1.23'
    steps:
      - uses: actions/checkout@v4
      - name: Node
        with:
          node-version-file: .nvmrc
        uses: actions/setup-node@v4 # la clé with avant uses
      - uses: actions/setup-python@v5
        with:
          python-version: ${{ matrix.python }}
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: '21'
      - uses: dtolnay/rust-toolchain@1.80
      - uses: actions/setup-dotnet@v4
        with:
          dotnet-version: ${{ inputs.dotnet }}
      - uses: microsoft/setup-msbuild@v2
      - run: npm ci
`)
	n := needsOf(t, root)
	for key, want := range map[string]string{"node": "22.11.0", "python": "3.12", "go": "1.23", "java": "21", "rust": "1.80"} {
		if n[key].Version != want {
			t.Errorf("%s : %q, attendu %q (sources %v)", key, n[key].Version, want, n[key].Sources)
		}
	}
	if n["dotnet"].Version != "" {
		t.Errorf("une expression inconnue ne donne pas de version : %q", n["dotnet"].Version)
	}
	if n["node"].Pinned != "22.11.0" || n["node"].PinnedFrom != ".nvmrc" {
		t.Errorf("Node épinglé par .nvmrc : %+v", n["node"])
	}
	if _, ok := n["msvc"]; !ok || !n["msvc"].CIOnly() {
		t.Errorf("MSVC demandé par la CI seule : %+v", n["msvc"])
	}
	if n["node"].CIOnly() {
		t.Error(".nvmrc fait partie du projet : Node n'est pas « CI seule »")
	}
}

func TestCIExactNodeVersionIsPinned(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".github/workflows/a.yaml", "jobs:\n  b:\n    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: '20.18.1'\n      - uses: actions/setup-node@v4\n        with:\n          node-version: 18.x\n")
	n := needsOf(t, root)
	if n["node"].Pinned != "20.18.1" || n["node"].Version != "20.18.1" {
		t.Fatalf("%+v", n["node"])
	}
}

func TestDevcontainer(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".devcontainer/devcontainer.json", `{
	  // Image officielle
	  "image": "mcr.microsoft.com/devcontainers/typescript-node:1-22-bookworm",
	  /* features */
	  "features": {
	    "ghcr.io/devcontainers/features/python:1": { "version": "3.12" },
	    "ghcr.io/devcontainers/features/docker-in-docker:2": {},
	    "ghcr.io/devcontainers/features/sshd:1": {},
	  },
	  "name": "url // pas un commentaire"
	}`)
	write(t, root, ".devcontainer/go/devcontainer.json", `{"build": {"dockerfile": "Dockerfile"}}`)
	write(t, root, ".devcontainer/go/Dockerfile", "FROM mcr.microsoft.com/devcontainers/go:1.23\n")
	n := needsOf(t, root)
	for key, want := range map[string]string{"node": "22", "python": "3.12", "go": "1.23"} {
		if n[key].Version != want {
			t.Errorf("%s : %q, attendu %q", key, n[key].Version, want)
		}
	}
	if _, ok := n["docker"]; !ok {
		t.Error("docker-in-docker → Docker")
	}
}

func TestStripJSONC(t *testing.T) {
	got := stripJSONC(`{"a": "http://x", // c
	/* b */ "b": [1,2,],}`)
	if strings.Contains(got, "//") && !strings.Contains(got, "http://x") || strings.Contains(got, "/*") || strings.Contains(got, ",]") {
		t.Fatalf("obtenu %q", got)
	}
}

func TestPinnedMajorMismatch(t *testing.T) {
	oldLook, oldVer := LookPath, VersionOf
	t.Cleanup(func() { LookPath, VersionOf = oldLook, oldVer })
	LookPath = func(c string) (string, error) { return "/bin/" + c, nil }
	VersionOf = func(string, ...string) string { return "v24.1.0" }
	got := Check([]Need{
		{Tool: toolByKey("node"), Version: "22", Pinned: "22.11.0"},
		{Tool: toolByKey("node"), Version: "24", Pinned: "24.18.0"},
		{Tool: toolByKey("node"), Version: "20"},
	})
	want := []State{Mismatch, OK, OK}
	for i, w := range want {
		if got[i].State != w {
			t.Errorf("%d : %v, attendu %v", i, got[i].State, w)
		}
	}
}

func TestSourcesSummary(t *testing.T) {
	n := Need{Sources: []string{".nvmrc", ".github/workflows/a.yml", "package.json", ".github/workflows/b.yml"}}
	if got := n.SourcesSummary(); got != ".nvmrc, la CI (2 workflows), package.json" {
		t.Fatalf("obtenu %q", got)
	}
	_ = os.Getenv
	_ = filepath.Join
}
