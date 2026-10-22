package docker

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConvertArgs(t *testing.T) {
	in := []string{
		"run", "-v", `C:\Users\Rayan\projet:/app`,
		"--mount", `type=bind,source=D:\data,target=/data`,
		"-e", "MODE=prod", "nginx:alpine", "./local", "-p", "8080:80",
	}
	want := []string{
		"run", "-v", "/mnt/c/Users/Rayan/projet:/app",
		"--mount", "type=bind,source=/mnt/d/data,target=/data",
		"-e", "MODE=prod", "nginx:alpine", "./local", "-p", "8080:80",
	}
	if got := convertArgs(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu %q", got)
	}
}

func TestHasDistro(t *testing.T) {
	list := "Ubuntu\r\norvaen-docker\r\n"
	if !hasDistro(list, Distro) {
		t.Fatal("la machine aurait dû être trouvée")
	}
	if hasDistro("Ubuntu\r\norvaen-docker-old\r\n", Distro) {
		t.Fatal("ne doit pas confondre avec un nom proche")
	}
}

const sampleYAML = `---
-
  title: "Standard"
  flavor: alpine-standard
  file: alpine-standard-3.20.3-x86_64.iso
  sha256: aaaa
-
  title: "Mini root filesystem"
  desc: "Minimal root filesystem.
    For use in containers
    and minimal chroots."
  profile: minirootfs
  image: alpine-minirootfs-3.20.3-x86_64.tar.gz
  flavor: alpine-minirootfs
  file: alpine-minirootfs-3.20.3-x86_64.tar.gz
  iso: alpine-minirootfs-3.20.3-x86_64.tar.gz
  date: 2024-09-06
  time: 06:36:31
  sha256: "ABCDEF0123"
-
  title: "Virtual"
  flavor: alpine-virt
  file: alpine-virt-3.20.3-x86_64.iso
  sha256: bbbb
`

func TestParseLatestReleases(t *testing.T) {
	rel, err := parseLatestReleases(strings.NewReader(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	want := release{File: "alpine-minirootfs-3.20.3-x86_64.tar.gz", SHA256: "abcdef0123"}
	if rel != want {
		t.Fatalf("obtenu %+v", rel)
	}
	if _, err := parseLatestReleases(strings.NewReader("---\n- flavor: alpine-standard\n  file: x\n  sha256: y\n")); err == nil {
		t.Fatal("une liste sans minirootfs doit échouer")
	}
}

func TestSetupScriptEmbedsEngine(t *testing.T) {
	if !strings.Contains(setupScript, "dockerd >/var/log/docker.log") || !strings.Contains(setupScript, "\nORVAEN_EOF\n") {
		t.Fatal("le script d'installation doit contenir le script du moteur et fermer le heredoc")
	}
}

func TestWSLConfMetadata(t *testing.T) {
	if !strings.Contains(wslConf, "metadata") || !strings.Contains(wslConf, wslConfMarker) {
		t.Fatal("wsl.conf doit activer metadata et contenir le marqueur de version")
	}
	if !strings.Contains(setupScript, writeWSLConfScript) {
		t.Fatal("l'installation doit écrire /etc/wsl.conf")
	}
	if !strings.HasSuffix(writeWSLConfScript, "\nORVAEN_EOF\n") {
		t.Fatal("le heredoc de wsl.conf doit être fermé")
	}
}

func TestSetupInstallsBuildKit(t *testing.T) {
	if !strings.Contains(setupScript, "docker-cli-buildx") {
		t.Fatal("l'installation doit ajouter BuildKit (docker-cli-buildx)")
	}
	if !strings.Contains(prepareScript, buildxPlugin) || !strings.Contains(prepareScript, writeWSLConfScript) {
		t.Fatal("la préparation doit ajouter BuildKit et mettre wsl.conf à jour")
	}
}

func TestBuildHint(t *testing.T) {
	dir := t.TempDir()
	if buildHint([]string{"build", "."}, dir) != "" {
		t.Fatal("pas de conseil sans gros dossier")
	}
	if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buildHint([]string{"build", "."}, dir), "node_modules") {
		t.Fatal("conseil attendu pour node_modules sans .dockerignore")
	}
	if !strings.Contains(buildHint([]string{"buildx", "build", "."}, dir), "node_modules") {
		t.Fatal("conseil attendu aussi pour docker buildx build")
	}
	if buildHint([]string{"run", "nginx"}, dir) != "" {
		t.Fatal("pas de conseil hors build")
	}
	if err := os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("node_modules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if buildHint([]string{"build", "."}, dir) != "" {
		t.Fatal("pas de conseil avec un .dockerignore")
	}
}
