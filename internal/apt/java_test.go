package apt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Orvaen/terminal/internal/platform"
)

// fakeJava imite api.adoptium.net, le registre (JAVA_HOME, PATH utilisateur) et un Java système.
type fakeJavaEnv struct {
	release  map[int]string // version majeure → release_name annoncée
	badSum   bool
	env      map[string]string
	userPath []string
	calls    *[][]string
}

func fakeJava(t *testing.T, elevated bool) *fakeJavaEnv {
	t.Helper()
	f := &fakeJavaEnv{
		release:  map[int]string{21: "jdk-21.0.12+7", 25: "jdk-25.0.1+8"},
		env:      map[string]string{},
		userPath: []string{`%USERPROFILE%\AppData\Local\Microsoft\WindowsApps`, `C:\outils\git\bin`},
	}
	f.calls = fakeWinget(t, nil)

	zips := map[string][]byte{} // zipWith n'est pas déterministe : un seul zip par version
	zipFor := func(rel string) []byte {
		if z, ok := zips[rel]; ok {
			return z
		}
		zips[rel] = zipWith(t, map[string]string{
			rel + "/bin/" + javaExe(): "java " + rel,
			rel + "/release":          "JAVA_VERSION=" + rel,
		})
		return zips[rel]
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/info/available_releases":
			fmt.Fprint(w, `{"available_lts_releases":[8,11,17,21,25],"available_releases":[8,11,17,21,25,26,27],"most_recent_lts":25,"most_recent_feature_release":27}`)
		case strings.HasPrefix(r.URL.Path, "/v3/assets/latest/"):
			var major int
			fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/v3/assets/latest/"), "%d", &major)
			rel, ok := f.release[major]
			if !ok || r.URL.Query().Get("os") != "windows" || r.URL.Query().Get("image_type") != "jdk" {
				fmt.Fprint(w, `[]`)
				return
			}
			h := sha256.Sum256(zipFor(rel))
			sum := hex.EncodeToString(h[:])
			if f.badSum {
				sum = strings.Repeat("0", 64)
			}
			fmt.Fprintf(w, `[{"binary":{"installer":{"link":"http://%[1]s/dl/%[2]s.msi","checksum":"x"},"package":{"link":"http://%[1]s/dl/%[2]s.zip","checksum":"%[3]s"}},"release_name":"%[2]s"}]`, r.Host, rel, sum)
		case strings.HasPrefix(r.URL.Path, "/dl/") && strings.HasSuffix(r.URL.Path, ".zip"):
			w.Write(zipFor(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/dl/"), ".zip")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	oldAPI, oldSet, oldRead, oldWrite, oldSys := adoptiumAPI, setUserEnv, readUserPath, writeUserPath, systemJavaDir
	oldE, oldEl, oldPath, oldHome := portableEnabled, isElevated, os.Getenv("PATH"), os.Getenv("JAVA_HOME")
	t.Cleanup(func() {
		adoptiumAPI, setUserEnv, readUserPath, writeUserPath, systemJavaDir = oldAPI, oldSet, oldRead, oldWrite, oldSys
		portableEnabled, isElevated = oldE, oldEl
		os.Setenv("PATH", oldPath)
		os.Setenv("JAVA_HOME", oldHome)
	})
	os.Unsetenv("JAVA_HOME")
	os.Setenv("PATH", `C:\Program Files\Common Files\Oracle\Java\javapath`)
	adoptiumAPI = srv.URL + "/v3"
	portableEnabled = true
	isElevated = func() bool { return elevated }
	setUserEnv = func(name, value string, remove bool) error {
		if remove {
			delete(f.env, name)
		} else {
			f.env[name] = value
		}
		return nil
	}
	readUserPath = func() []platform.PathEntry {
		var out []platform.PathEntry
		for _, p := range f.userPath {
			out = append(out, platform.PathEntry{Raw: p, Dir: p})
		}
		return out
	}
	writeUserPath = func(raw []string) error { f.userPath = append([]string(nil), raw...); return nil }
	systemJavaDir = func() string { return `C:\Program Files\Common Files\Oracle\Java\javapath` }
	return f
}

func firstPath() string { return filepath.SplitList(os.Getenv("PATH"))[0] }

func TestJavaInstallFirstBecomesActive(t *testing.T) {
	f := fakeJava(t, false)
	if code := Run([]string{"java", "install", "21"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*f.calls) != 0 {
		t.Fatalf("winget n'aurait pas dû être appelé : %v", *f.calls)
	}
	if !fileExists(jdkJava(21)) {
		t.Fatal("java absent du JDK 21")
	}
	if v, _ := jdkVersion(21); v != "jdk-21.0.12+7" {
		t.Fatalf("version %q", v)
	}
	if f.env["JAVA_HOME"] != jdkDir(21) || os.Getenv("JAVA_HOME") != jdkDir(21) {
		t.Fatalf("JAVA_HOME %q / %q", f.env["JAVA_HOME"], os.Getenv("JAVA_HOME"))
	}
	if f.userPath[0] != jdkBin(21) || len(f.userPath) != 3 {
		t.Fatalf("PATH utilisateur %v", f.userPath)
	}
	if firstPath() != jdkBin(21) || ActiveJDKBin() != jdkBin(21) {
		t.Fatalf("PATH du terminal %q", os.Getenv("PATH"))
	}

	// Le deuxième JDK ne remplace pas l'actif.
	if code := Run([]string{"java", "install", "lts"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if activeJDK() != 21 || !reflect.DeepEqual(installedJDKs(), []int{25, 21}) {
		t.Fatalf("actif %d, installés %v", activeJDK(), installedJDKs())
	}
}

func TestJavaUseSwitchesHomeAndPath(t *testing.T) {
	f := fakeJava(t, false)
	Run([]string{"java", "install", "21"})
	Run([]string{"java", "install", "25"})
	if code := Run([]string{"java", "use", "jdk-25"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	want := []string{jdkBin(25), `%USERPROFILE%\AppData\Local\Microsoft\WindowsApps`, `C:\outils\git\bin`}
	if !reflect.DeepEqual(f.userPath, want) {
		t.Fatalf("PATH utilisateur %v, attendu %v", f.userPath, want)
	}
	if f.env["JAVA_HOME"] != jdkDir(25) || activeJDK() != 25 {
		t.Fatalf("JAVA_HOME %q", f.env["JAVA_HOME"])
	}
	if strings.Contains(os.Getenv("PATH"), jdkBin(21)) || firstPath() != jdkBin(25) {
		t.Fatalf("PATH du terminal %q", os.Getenv("PATH"))
	}
	if code := Run([]string{"java", "use", "17"}); code == 0 {
		t.Fatal("un JDK non installé ne peut pas devenir actif")
	}
}

func TestJavaUpgradeKeepsJavaHome(t *testing.T) {
	f := fakeJava(t, false)
	Run([]string{"java", "install", "21"})
	f.release[21] = "jdk-21.0.13+9"
	if code := Run([]string{"upgrade", "java"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*f.calls) != 0 {
		t.Fatalf("winget appelé : %v", *f.calls)
	}
	if v, _ := jdkVersion(21); v != "jdk-21.0.13+9" {
		t.Fatalf("version %q", v)
	}
	if b, _ := os.ReadFile(jdkJava(21)); string(b) != "java jdk-21.0.13+9" {
		t.Fatalf("java %q", b)
	}
	if f.env["JAVA_HOME"] != jdkDir(21) {
		t.Fatalf("JAVA_HOME %q", f.env["JAVA_HOME"])
	}
	for _, d := range []string{jdkDir(21) + ".nouveau", jdkDir(21) + ".ancien"} {
		if fileExists(d) {
			t.Fatalf("%s n'a pas été nettoyé", d)
		}
	}
}

func TestJavaBadChecksumKeepsOldJDK(t *testing.T) {
	f := fakeJava(t, false)
	Run([]string{"java", "install", "21"})
	f.release[21], f.badSum = "jdk-21.0.13+9", true
	if code := Run([]string{"java", "upgrade"}); code == 0 {
		t.Fatal("une somme invalide doit échouer")
	}
	if v, _ := jdkVersion(21); v != "jdk-21.0.12+7" || !fileExists(jdkJava(21)) {
		t.Fatalf("l'ancien JDK a été abîmé (%q)", v)
	}
}

func TestJavaRemoveActive(t *testing.T) {
	f := fakeJava(t, false)
	Run([]string{"java", "install", "21"})
	if code := Run([]string{"remove", "openjdk-21-jdk"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*f.calls) != 0 {
		t.Fatalf("winget appelé : %v", *f.calls)
	}
	if _, ok := f.env["JAVA_HOME"]; ok || os.Getenv("JAVA_HOME") != "" {
		t.Fatal("JAVA_HOME aurait dû être retiré")
	}
	if fileExists(jdkDir(21)) || len(f.userPath) != 2 || strings.Contains(os.Getenv("PATH"), jdkBin(21)) {
		t.Fatalf("restes : PATH utilisateur %v, terminal %q", f.userPath, os.Getenv("PATH"))
	}
}

func TestAptInstallJavaWithoutAdminIsPortable(t *testing.T) {
	f := fakeJava(t, false)
	if code := Run([]string{"install", "java"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(*f.calls) != 0 || activeJDK() != 25 {
		t.Fatalf("winget %v, actif %d", *f.calls, activeJDK())
	}
}

func TestAptInstallJavaAsAdminKeepsWinget(t *testing.T) {
	f := fakeJava(t, true)
	Run([]string{"install", "openjdk-21-jdk"})
	want := [][]string{{"install", "--id", "EclipseAdoptium.Temurin.21.JDK", "--exact", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*f.calls, want) || len(installedJDKs()) != 0 {
		t.Fatalf("appels %v", *f.calls)
	}
}

func TestJavaPackageNames(t *testing.T) {
	cases := map[string]int{"java": 0, "jdk": 0, "openjdk": 0, "default-jdk": 0, "jdk21": 21, "jdk-17": 17,
		"openjdk-21-jdk": 21, "openjdk-17-jre": 17, "temurin-25": 25, "java8": 8}
	for name, want := range cases {
		if v, ok := javaPackage(name); !ok || v != want {
			t.Errorf("%s → %d, %v (attendu %d)", name, v, ok, want)
		}
	}
	for _, name := range []string{"javascript", "gradle", "maven", "jdk-mission-control"} {
		if _, ok := javaPackage(name); ok {
			t.Errorf("%s ne désigne pas un JDK", name)
		}
	}
}

func TestJavaList(t *testing.T) {
	fakeJava(t, false)
	Run([]string{"java", "install", "21"})
	var out strings.Builder
	old := stdout
	stdout = &out
	defer func() { stdout = old }()
	if code := Run([]string{"java"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	s := out.String()
	for _, want := range []string{" 27 ", "*  21  LTS  ", "jdk-21.0.12+7", "← conseillé"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q absent de :\n%s", want, s)
		}
	}
	if strings.Contains(s, " 26 ") {
		t.Errorf("26 (ni LTS ni la plus récente) ne devrait pas être listé :\n%s", s)
	}
}

func TestRemoveSystemJavaExplains(t *testing.T) {
	fakeJava(t, false)
	old := toolLookPath
	t.Cleanup(func() { toolLookPath = old })
	toolLookPath = func(string) (string, error) {
		return `C:\Program Files\Common Files\Oracle\Java\javapath\java.exe`, nil
	}
	if _, ok := javaNotRemovable("java"); !ok {
		t.Fatal("le Java système devrait être expliqué")
	}
	if _, ok := javaNotRemovable("gradle"); ok {
		t.Fatal("gradle n'est pas un JDK")
	}
}
