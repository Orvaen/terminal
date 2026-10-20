package deps

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// monorepo ressemble à un vrai projet : API Node, tests Hurl, Makefile qui pilote Docker…
func monorepo(t *testing.T) string {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)
	write(t, root, "Makefile", "up:\n\tdocker compose up -d --wait\n")
	write(t, root, "docker-compose.yml", "services: {}\n")
	write(t, root, "api/package.json", `{"engines": {"node": ">=20.11"}, "packageManager": "pnpm@9.1.0"}`)
	write(t, root, "api/pnpm-lock.yaml", "")
	write(t, root, "api/.nvmrc", "v22\n")
	write(t, root, "api/node_modules/dep/package.json", `{"engines": {"node": ">=99"}}`)
	write(t, root, "tests/00-games.hurl", "GET http://x\n")
	write(t, root, "worker/go.mod", "module x\n\ngo 1.23.4\n")
	write(t, root, "ml/pyproject.toml", "[project]\nrequires-python = \">=3.11\"\n\n[tool.uv]\n")
	write(t, root, "ml/uv.lock", "")
	write(t, root, "svc/global.json", `{"sdk": {"version": "8.0.100"}}`)
	write(t, root, "svc/Svc.sln", "")
	write(t, root, "svc/src/Svc.csproj", "<Project/>")
	write(t, root, "java/pom.xml", "<project><properties><maven.compiler.release>17</maven.compiler.release></properties></project>")
	write(t, root, "a/b/c/d/package.json", `{}`) // trop profond
	return root
}

func TestDetectMonorepo(t *testing.T) {
	p, err := Detect(monorepo(t))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Need{}
	var order []string
	for _, n := range p.Needs {
		got[n.Key] = n
		order = append(order, n.Key)
	}
	wantOrder := []string{"git", "docker", "make", "node", "pnpm", "go", "python", "uv", "java", "maven", "dotnet", "hurl"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("outils : %v\nattendu : %v", order, wantOrder)
	}
	for key, want := range map[string]string{"node": "22", "go": "1.23.4", "python": "3.11", "java": "17", "dotnet": "8.0.100", "pnpm": "9.1.0"} {
		if got[key].Version != want {
			t.Errorf("%s : version %q, attendu %q", key, got[key].Version, want)
		}
	}
	if got["node"].VersionFrom != "api/.nvmrc" {
		t.Errorf(".nvmrc (22) est plus exigeant que engines (20.11) : %+v", got["node"])
	}
	if !reflect.DeepEqual(got["hurl"].Sources, []string{"tests/*.hurl"}) {
		t.Errorf("hurl : %v", got["hurl"].Sources)
	}
	if !strings.Contains(strings.Join(got["docker"].Sources, ","), "Makefile") {
		t.Errorf("docker doit aussi venir du Makefile : %v", got["docker"].Sources)
	}

	var steps []string
	for _, s := range p.Steps {
		steps = append(steps, s.Dir+": "+s.Label)
	}
	wantSteps := []string{"api: pnpm install", "ml: uv sync", "svc: dotnet restore", "worker: go mod download"}
	if !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("étapes : %v\nattendu : %v", steps, wantSteps)
	}
	if !p.Steps[0].Done {
		t.Error("node_modules existe : pnpm install est déjà fait")
	}
}

func TestDetectToolVersionsAndWrappers(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".tool-versions", "nodejs 20.11.0\npython 3.12.1\n# commentaire\nunknown 1\n")
	write(t, root, "mise.toml", "[tools]\ngo = \"1.22\"\n")
	write(t, root, "pom.xml", "<project/>")
	write(t, root, "mvnw.cmd", "")
	p, _ := Detect(root)
	keys := map[string]string{}
	for _, n := range p.Needs {
		keys[n.Key] = n.Version
	}
	if keys["node"] != "20.11.0" || keys["python"] != "3.12.1" || keys["go"] != "1.22" {
		t.Fatalf("versions : %v", keys)
	}
	if _, ok := keys["maven"]; ok {
		t.Fatal("avec mvnw, Maven n'est pas nécessaire")
	}
}

func TestCleanVersion(t *testing.T) {
	for in, want := range map[string]string{
		">=18.17.0": "18.17.0", "v20": "20", "^3.10": "3.10", "lts/*": "", "~1.22": "1.22",
		"temurin-21": "21", "3.12.1\n": "3.12.1", ">=3.9,<4": "3.9", "": "",
	} {
		if got := cleanVersion(in); got != want {
			t.Errorf("%q : %q, attendu %q", in, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	oldLook, oldVer, oldDocker, oldMSVC := LookPath, VersionOf, DockerInstalled, MSVCInstalled
	t.Cleanup(func() { LookPath, VersionOf, DockerInstalled, MSVCInstalled = oldLook, oldVer, oldDocker, oldMSVC })
	MSVCInstalled = func() bool { return true }
	installed := map[string]string{
		"node":   `C:\Program Files\nodejs\node.exe`,
		"go":     `C:\Go\bin\go.exe`,
		"python": `C:\Users\r\AppData\Local\Microsoft\WindowsApps\python.exe`, // faux python du Store
		"java":   `C:\jdk\bin\java.exe`,
	}
	LookPath = func(c string) (string, error) {
		if p, ok := installed[c]; ok {
			return filepath.ToSlash(p), nil
		}
		return "", errors.New("absent")
	}
	VersionOf = func(path string, args ...string) string {
		switch {
		case strings.Contains(path, "node"):
			return "v24.1.0\n"
		case strings.Contains(path, "go.exe"):
			return "go version go1.22.5 windows/amd64\n"
		case strings.Contains(path, "java"):
			return "Picked up JAVA_TOOL_OPTIONS: x\nopenjdk version \"21.0.2\" 2024-01-16\n"
		}
		return ""
	}
	DockerInstalled = func() bool { return true }

	needs := []Need{
		{Tool: toolByKey("node"), Version: "20"},
		{Tool: toolByKey("go"), Version: "1.23"},
		{Tool: toolByKey("python")},
		{Tool: toolByKey("docker")},
		{Tool: toolByKey("composer")},
		{Tool: toolByKey("java"), Version: "17"},
		{Tool: toolByKey("cpp")},
		{Tool: Tool{Key: "x", Label: "Sans paquet", Commands: []string{"x"}}},
	}
	got := Check(needs)
	want := []struct {
		state State
		have  string
	}{{OK, "24.1.0"}, {Outdated, "1.22.5"}, {Missing, ""}, {OK, "Docker léger"}, {Missing, ""}, {OK, "21.0.2"},
		{OK, "MSVC (Build Tools)"}, {NoPackage, ""}}
	for i, w := range want {
		if got[i].State != w.state || got[i].Have != w.have {
			t.Errorf("%s : état %v %q, attendu %v %q", got[i].Key, got[i].State, got[i].Have, w.state, w.have)
		}
	}
}

func TestPackageFor(t *testing.T) {
	for _, tc := range []struct{ key, version, want string }{
		{"java", "17", "openjdk-17-jdk"}, {"java", "21", "openjdk-21-jdk"}, {"java", "", "java"},
		{"dotnet", "8.0.100", "dotnet-sdk-8.0"}, {"dotnet", "", "dotnet"}, {"node", "22", "node"},
	} {
		if got := PackageFor(Need{Tool: toolByKey(tc.key), Version: tc.version}); got != tc.want {
			t.Errorf("%s %s : %q, attendu %q", tc.key, tc.version, got, tc.want)
		}
	}
}
