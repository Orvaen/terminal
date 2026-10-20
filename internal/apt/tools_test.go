package apt

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	return buf.Bytes()
}

// fakeSources imite getcomposer.org, services.gradle.org, Maven Central/Apache et PyPI.
func fakeSources(t *testing.T, badGradleSum bool) (paths *[]string, pathAdds *[]string) {
	t.Helper()
	phar := []byte("<?php // composer")
	gradleZip := zipWith(t, map[string]string{"gradle-9.8.2/bin/gradle.bat": "@echo gradle", "gradle-9.8.2/lib/x.jar": "jar"})
	mavenZip := zipWith(t, map[string]string{"apache-maven-3.9.11/bin/mvn.cmd": "@echo mvn", "apache-maven-3.9.11/conf/settings.xml": "<s/>"})
	s256 := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	s512 := func(b []byte) string { h := sha512.Sum512(b); return hex.EncodeToString(h[:]) }
	gradleSum := s256(gradleZip)
	if badGradleSum {
		gradleSum = strings.Repeat("0", 64)
	}

	var log []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log = append(log, r.URL.Path)
		switch r.URL.Path {
		case "/composer/versions":
			fmt.Fprint(w, `{"stable":[{"path":"/download/2.10.3/composer.phar","version":"2.10.3"},{"path":"/download/2.2.30/composer.phar","version":"2.2.30"}]}`)
		case "/composer/download/2.10.3/composer.phar":
			w.Write(phar)
		case "/composer/download/2.10.3/composer.phar.sha256sum":
			fmt.Fprintf(w, "%s  composer.phar\n", s256(phar))
		case "/gradle/current":
			fmt.Fprintf(w, `{"version":"9.8.2","downloadUrl":"%s/gradle/gradle-9.8.2-bin.zip","checksum":"%s"}`, srv.URL, gradleSum)
		case "/gradle/gradle-9.8.2-bin.zip":
			w.Write(gradleZip)
		case "/maven/meta.xml":
			fmt.Fprint(w, `<metadata><versioning><latest>4.0.0-rc-7</latest><versions><version>3.9.9</version><version>3.9.11</version><version>3.10.0-alpha-1</version><version>4.0.0-rc-7</version></versions></versioning></metadata>`)
		case "/mirror/maven-3/3.9.11/binaries/apache-maven-3.9.11-bin.zip":
			w.Write(mavenZip)
		case "/sums/maven-3/3.9.11/binaries/apache-maven-3.9.11-bin.zip.sha512":
			fmt.Fprintf(w, "%s  apache-maven-3.9.11-bin.zip\n", s512(mavenZip))
		case "/pypi/poetry":
			fmt.Fprint(w, `{"info":{"version":"2.3.1"}}`)
		case "/poetry-installer":
			fmt.Fprint(w, "# installeur de Poetry")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	old := []any{composerBase, gradleVersions, mavenMetadata, mavenMirrors, mavenSums, poetryVersions, poetryScript, toolLookPath, addUserPath, removeUserPath, runPython}
	t.Cleanup(func() {
		composerBase, gradleVersions, mavenMetadata = old[0].(string), old[1].(string), old[2].(string)
		mavenMirrors, mavenSums = old[3].([]string), old[4].([]string)
		poetryVersions, poetryScript = old[5].(string), old[6].(string)
		toolLookPath = old[7].(func(string) (string, error))
		addUserPath, removeUserPath = old[8].(func(string) error), old[9].(func(string) error)
		runPython = old[10].(func(string, []string, ...string) int)
	})
	composerBase = srv.URL + "/composer"
	gradleVersions = srv.URL + "/gradle/current"
	mavenMetadata = srv.URL + "/maven/meta.xml"
	mavenMirrors = []string{srv.URL + "/absent", srv.URL + "/mirror"}
	mavenSums = []string{srv.URL + "/sums"}
	poetryVersions = srv.URL + "/pypi/poetry"
	poetryScript = srv.URL + "/poetry-installer"
	toolLookPath = func(c string) (string, error) {
		if c == "php" || c == "java" || c == "py" {
			return `C:\bin\` + c + ".exe", nil
		}
		if c == "python" {
			return `C:\Users\r\AppData\Local\Microsoft\WindowsApps\python.exe`, nil // faux python du Store
		}
		return "", errors.New("absent")
	}
	var adds []string
	addUserPath = func(d string) error { adds = append(adds, d); return nil }
	removeUserPath = func(d string) error { adds = append(adds, "-"+d); return nil }
	return &log, &adds
}

func TestInstallComposerGradleMaven(t *testing.T) {
	calls := fakeWinget(t, nil)
	_, adds := fakeSources(t, false)
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })

	if code := Run([]string{"install", "composer", "gradle", "mvn"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("winget ne doit pas être appelé : %v", *calls)
	}
	root := toolsRoot()
	for file, want := range map[string]string{
		"composer/composer.phar":  "<?php // composer",
		"composer/.version":       "2.10.3\n",
		"gradle/bin/gradle.bat":   "@echo gradle",
		"gradle/.version":         "9.8.2\n",
		"maven/bin/mvn.cmd":       "@echo mvn",
		"maven/conf/settings.xml": "<s/>",
		"maven/.version":          "3.9.11\n",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil || string(data) != want {
			t.Errorf("%s : %q %v", file, data, err)
		}
	}
	bat, _ := os.ReadFile(filepath.Join(root, "composer", "composer.bat"))
	if !strings.Contains(string(bat), `php "%~dp0composer.phar" %*`) {
		t.Errorf("composer.bat : %q", bat)
	}
	wantAdds := []string{filepath.Join(root, "composer"), filepath.Join(root, "gradle", "bin"), filepath.Join(root, "maven", "bin")}
	if strings.Join(*adds, "|") != strings.Join(wantAdds, "|") {
		t.Errorf("PATH utilisateur : %v", *adds)
	}
	if !strings.HasPrefix(os.Getenv("PATH"), filepath.Join(root, "maven", "bin")) {
		t.Error("le PATH du processus doit contenir l'outil tout de suite")
	}

	// Déjà à jour : rien n'est retéléchargé ; puis désinstallation.
	if code := Run([]string{"upgrade", "gradle"}); code != 0 {
		t.Fatalf("upgrade : %d", code)
	}
	if code := Run([]string{"remove", "gradle"}); code != 0 {
		t.Fatalf("remove : %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "gradle")); !os.IsNotExist(err) {
		t.Fatal("le dossier de Gradle doit être supprimé")
	}
	if last := (*adds)[len(*adds)-1]; last != "-"+filepath.Join(root, "gradle", "bin") {
		t.Fatalf("Gradle doit être retiré du PATH : %v", *adds)
	}
}

func TestBadChecksumKeepsPreviousInstall(t *testing.T) {
	fakeWinget(t, nil)
	fakeSources(t, true)
	if code := Run([]string{"install", "gradle"}); code == 0 {
		t.Fatal("une somme invalide doit faire échouer l'installation")
	}
	entries, _ := os.ReadDir(toolsRoot())
	if len(entries) != 0 {
		t.Fatalf("aucun dossier ne doit rester : %v", entries)
	}
}

func TestToolNeedsItsRuntime(t *testing.T) {
	fakeWinget(t, nil)
	fakeSources(t, false)
	toolLookPath = func(string) (string, error) { return "", errors.New("absent") }
	if code := Run([]string{"install", "maven"}); code != 1 {
		t.Fatalf("sans Java : code 1 attendu, obtenu %d", code)
	}
}

func TestInstallPoetryUsesOfficialInstaller(t *testing.T) {
	fakeWinget(t, nil)
	fakeSources(t, false)
	var got []string
	runPython = func(python string, env []string, args ...string) int {
		got = append(got, python+" "+strings.Join(env, " ")+" "+filepath.Base(args[0])+" "+strings.Join(args[1:], " "))
		return 0
	}
	if code := Run([]string{"install", "poetry"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	want := `C:\bin\py.exe POETRY_HOME=` + toolsRoot() + string(filepath.Separator) + "poetry install-poetry.py --version 2.3.1"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("installeur lancé : %q\nattendu : %q", got, want)
	}
}

func TestMavenLatestIgnoresPrereleases(t *testing.T) {
	fakeSources(t, false)
	if v, err := mavenLatest(); err != nil || v != "3.9.11" {
		t.Fatalf("obtenu %q %v", v, err)
	}
}

func TestBuildEssentialInstallsMSVCWithCppWorkload(t *testing.T) {
	calls := fakeWinget(t, nil)
	Run([]string{"install", "build-essential"})
	if len(*calls) != 1 || !strings.Contains(strings.Join((*calls)[0], " "), "--add Microsoft.VisualStudio.Workload.VCTools") {
		t.Fatalf("appels %v", *calls)
	}
	*calls = nil
	Run([]string{"install", "gcc"})
	if len(*calls) != 1 || (*calls)[0][2] != "BrechtSanders.WinLibs.POSIX.UCRT" {
		t.Fatalf("gcc : %v", *calls)
	}
}
