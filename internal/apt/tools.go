package apt

// Outils absents de winget (Composer, Maven, Gradle, Poetry) : apt les installe depuis leur source
// officielle, vérifie la somme de contrôle quand l'éditeur en publie une, et les range dans
// %LOCALAPPDATA%\OrvaenTerminal\outils\<nom>, avec leur dossier bin ajouté au PATH utilisateur.
//
// Et build-essential : les Build Tools de Visual Studio (via winget) avec la charge de travail C++
// déjà sélectionnée, ce que winget ne fait pas seul.

import (
	"archive/zip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// Points d'injection, remplacés dans les tests.
var (
	toolsRoot = func() string {
		base, err := os.UserCacheDir() // %LOCALAPPDATA%
		if err != nil {
			base = os.TempDir()
		}
		return filepath.Join(base, "OrvaenTerminal", "outils")
	}
	toolLookPath   = exec.LookPath
	addUserPath    = platform.AddUserPath
	removeUserPath = platform.RemoveUserPath
	composerBase   = "https://getcomposer.org"
	gradleVersions = "https://services.gradle.org/versions/current"
	mavenMetadata  = "https://repo.maven.apache.org/maven2/org/apache/maven/apache-maven/maven-metadata.xml"
	mavenMirrors   = []string{"https://dlcdn.apache.org/maven", "https://archive.apache.org/dist/maven"}
	mavenSums      = []string{"https://downloads.apache.org/maven", "https://archive.apache.org/dist/maven"}
	poetryVersions = "https://pypi.org/pypi/poetry/json"
	poetryScript   = "https://install.python-poetry.org"
	runPython      = func(python string, env []string, args ...string) int {
		cmd := exec.Command(python, args...)
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode()
			}
			return 1
		}
		return 0
	}
)

// extTool : un outil installé par apt lui-même.
type extTool struct {
	Name     string
	Label    string
	Aliases  []string
	Needs    []string // une de ces commandes doit exister (php, java, python)
	NeedsPkg string   // paquet apt qui la fournit
	Bin      string   // sous-dossier ajouté au PATH ("" : le dossier lui-même)
	Source   string   // pour la doc
	latest   func() (string, error)
	install  func(dir, version string) error // installe dans dir (vide), version donnée par latest
	remove   func(dir string)                // nettoyage en plus de la suppression du dossier
	inPlace  bool                            // s'installe directement à sa place (chemins absolus : venv Python)
}

var extTools = []extTool{
	{
		Name: "composer", Label: "Composer", Aliases: []string{"php-composer"}, Needs: []string{"php"}, NeedsPkg: "php",
		Source: "getcomposer.org, somme SHA-256 vérifiée",
		latest: composerLatest, install: composerInstall,
	},
	{
		Name: "maven", Label: "Maven", Aliases: []string{"mvn", "apache-maven"}, Needs: []string{"java"}, NeedsPkg: "java",
		Bin: "bin", Source: "Apache (dernière version stable), somme SHA-512 vérifiée",
		latest: mavenLatest, install: mavenInstall,
	},
	{
		Name: "gradle", Label: "Gradle", Needs: []string{"java"}, NeedsPkg: "java",
		Bin: "bin", Source: "services.gradle.org, somme SHA-256 vérifiée",
		latest: gradleLatest, install: gradleInstall,
	},
	{
		Name: "poetry", Label: "Poetry", Aliases: []string{"python3-poetry", "python-poetry"}, Needs: []string{"python", "py"}, NeedsPkg: "python",
		Bin: "bin", Source: "installeur officiel install.python-poetry.org",
		latest: poetryLatest, install: poetryInstall, remove: poetryRemove, inPlace: true,
	},
}

func findExtTool(name string) (extTool, bool) {
	n := strings.ToLower(name)
	for _, t := range extTools {
		if t.Name == n {
			return t, true
		}
		for _, a := range t.Aliases {
			if a == n {
				return t, true
			}
		}
	}
	return extTool{}, false
}

func (t extTool) dir() string { return filepath.Join(toolsRoot(), t.Name) }

func (t extTool) binDir() string {
	if t.Bin == "" {
		return t.dir()
	}
	return filepath.Join(t.dir(), t.Bin)
}

func (t extTool) installedVersion() (string, bool) {
	b, err := os.ReadFile(filepath.Join(t.dir(), ".version"))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// requirement trouve la commande dont l'outil a besoin, en évitant le faux python du Store.
func (t extTool) requirement() (string, bool) {
	if len(t.Needs) == 0 {
		return "", true
	}
	for _, c := range t.Needs {
		if p, err := toolLookPath(c); err == nil && !strings.Contains(strings.ToLower(p), `windowsapps`) {
			return p, true
		}
	}
	return "", false
}

// installExt installe ou met à jour un outil. onlyIfInstalled : pour apt upgrade (ne rien ajouter).
func installExt(t extTool, onlyIfInstalled bool) int {
	have, installed := t.installedVersion()
	if onlyIfInstalled && !installed {
		ui.Info(t.Label + " n'a pas été installé par apt : rien à mettre à jour.")
		return 0
	}
	if _, ok := t.requirement(); !ok {
		ui.Errorf("%s a besoin de %s : apt install %s, puis relance la commande.", t.Label, t.Needs[0], t.NeedsPkg)
		return 1
	}
	latest, err := t.latest()
	if err != nil {
		ui.Errorf("%s : impossible de trouver la dernière version : %v", t.Label, err)
		return 1
	}
	if installed && have == latest {
		ui.Info(fmt.Sprintf("%s %s est déjà à jour.", t.Label, have))
		return 0
	}
	ui.Info(fmt.Sprintf("Téléchargement de %s %s depuis %s…", t.Label, latest, strings.Split(t.Source, ",")[0]))

	if t.inPlace {
		// Poetry est un environnement virtuel Python : il ne supporte pas d'être déplacé.
		// Son installeur sait mettre à jour une installation existante.
		if err := os.MkdirAll(t.dir(), 0o755); err != nil {
			ui.Errorf("%v", err)
			return 1
		}
		if err := t.install(t.dir(), latest); err != nil {
			ui.Errorf("%s : %v", t.Label, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(t.dir(), ".version"), []byte(latest+"\n"), 0o644); err != nil {
			ui.Errorf("%v", err)
			return 1
		}
		return finishExt(t, have, latest, installed)
	}

	// Installation dans un dossier voisin, puis échange : une installation ratée ne casse pas l'ancienne.
	tmp := t.dir() + ".nouveau"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		ui.Errorf("%v", err)
		return 1
	}
	if err := t.install(tmp, latest); err != nil {
		os.RemoveAll(tmp)
		ui.Errorf("%s : %v", t.Label, err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(tmp, ".version"), []byte(latest+"\n"), 0o644); err != nil {
		os.RemoveAll(tmp)
		ui.Errorf("%v", err)
		return 1
	}
	old := t.dir() + ".ancien"
	os.RemoveAll(old)
	if installed {
		if err := os.Rename(t.dir(), old); err != nil {
			os.RemoveAll(tmp)
			ui.Errorf("%s est en cours d'utilisation ? Ferme-le et recommence (%v)", t.Label, err)
			return 1
		}
	} else {
		os.RemoveAll(t.dir())
	}
	if err := os.Rename(tmp, t.dir()); err != nil {
		os.Rename(old, t.dir())
		ui.Errorf("%v", err)
		return 1
	}
	os.RemoveAll(old)
	return finishExt(t, have, latest, installed)
}

// finishExt ajoute l'outil au PATH (utilisateur et terminal en cours) et le confirme.
func finishExt(t extTool, have, latest string, installed bool) int {
	if err := addUserPath(t.binDir()); err != nil {
		ui.Warn("impossible d'ajouter " + t.binDir() + " au PATH : " + err.Error())
	}
	prependProcessPath(t.binDir())
	if installed {
		ui.Info(fmt.Sprintf("%s mis à jour : %s → %s.", t.Label, have, latest))
	} else {
		ui.Info(fmt.Sprintf("%s %s est installé dans %s.", t.Label, latest, t.dir()))
	}
	return 0
}

func removeExt(t extTool) int {
	if _, installed := t.installedVersion(); !installed {
		ui.Info(t.Label + " n'a pas été installé par apt.")
		return 0
	}
	if t.remove != nil {
		t.remove(t.dir())
	}
	if err := os.RemoveAll(t.dir()); err != nil {
		ui.Errorf("%s est en cours d'utilisation ? Ferme-le et recommence (%v)", t.Label, err)
		return 1
	}
	_ = removeUserPath(t.binDir())
	ui.Info(t.Label + " est désinstallé.")
	return 0
}

// upgradeExtTools : pour « apt upgrade », met à jour les outils qu'apt a installés.
func upgradeExtTools() int {
	code := 0
	for _, t := range extTools {
		if _, installed := t.installedVersion(); installed {
			ui.Step("Mise à jour de " + t.Label)
			if c := installExt(t, true); c != 0 && code == 0 {
				code = c
			}
		}
	}
	return code
}

func prependProcessPath(dir string) {
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// ---------------------------------------------------------------------------
// Téléchargements vérifiés

// downloadVerified télécharge url dans dest et vérifie sa somme (hex) avec h.
func downloadVerified(url, dest, want string, h hash.Hash) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("téléchargement : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("téléchargement de %s : HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("téléchargement : %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	want = strings.ToLower(strings.TrimSpace(want))
	if got := hex.EncodeToString(h.Sum(nil)); want == "" || got != want {
		os.Remove(dest)
		return fmt.Errorf("somme de contrôle invalide pour %s (attendu %s, obtenu %s)", filepath.Base(dest), want, got)
	}
	return nil
}

// firstField : le premier mot d'un fichier de somme (« abc123  fichier.zip » → « abc123 »).
func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// unzipStrip extrait un zip dans dest en retirant le dossier de premier niveau (apache-maven-3.9.11/…).
func unzipStrip(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if i := strings.Index(name, "/"); i >= 0 {
			name = name[i+1:]
		} else {
			continue // fichier à la racine du zip : hors du dossier attendu
		}
		if name == "" {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return fmt.Errorf("chemin invalide dans l'archive : %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()|0o600)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func getJSON(url string, v any) error {
	body, err := httpGetBody(url)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}

// ---------------------------------------------------------------------------
// Composer

func composerLatest() (string, error) {
	var v struct {
		Stable []struct {
			Path    string `json:"path"`
			Version string `json:"version"`
		} `json:"stable"`
	}
	if err := getJSON(composerBase+"/versions", &v); err != nil {
		return "", err
	}
	if len(v.Stable) == 0 {
		return "", errors.New("aucune version stable annoncée")
	}
	return v.Stable[0].Version, nil
}

func composerInstall(dir, version string) error {
	sum, err := httpGetBody(fmt.Sprintf("%s/download/%s/composer.phar.sha256sum", composerBase, version))
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/download/%s/composer.phar", composerBase, version)
	if err := downloadVerified(url, filepath.Join(dir, "composer.phar"), firstField(sum), sha256.New()); err != nil {
		return err
	}
	// composer.bat : « composer » marche dans le terminal, PowerShell et cmd.
	bat := "@echo off\r\nphp \"%~dp0composer.phar\" %*\r\n"
	return os.WriteFile(filepath.Join(dir, "composer.bat"), []byte(bat), 0o755)
}

// ---------------------------------------------------------------------------
// Gradle

func gradleLatest() (string, error) {
	v, err := gradleCurrent()
	return v.Version, err
}

type gradleRelease struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	Checksum    string `json:"checksum"`
}

func gradleCurrent() (gradleRelease, error) {
	var r gradleRelease
	if err := getJSON(gradleVersions, &r); err != nil {
		return r, err
	}
	if r.Version == "" || r.DownloadURL == "" || r.Checksum == "" {
		return r, errors.New("réponse de services.gradle.org incomplète")
	}
	return r, nil
}

func gradleInstall(dir, version string) error {
	r, err := gradleCurrent()
	if err != nil {
		return err
	}
	if r.Version != version {
		return fmt.Errorf("la version a changé pendant l'installation (%s), recommence", r.Version)
	}
	zipPath := filepath.Join(dir, "gradle.zip")
	if err := downloadVerified(r.DownloadURL, zipPath, r.Checksum, sha256.New()); err != nil {
		return err
	}
	defer os.Remove(zipPath)
	return unzipStrip(zipPath, dir)
}

// ---------------------------------------------------------------------------
// Maven

var stableVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func mavenLatest() (string, error) {
	body, err := httpGetBody(mavenMetadata)
	if err != nil {
		return "", err
	}
	var meta struct {
		Versions []string `xml:"versioning>versions>version"`
	}
	if err := xml.Unmarshal([]byte(body), &meta); err != nil {
		return "", err
	}
	// Seulement les versions stables : la « dernière » annoncée peut être une 4.0.0-rc.
	var stable []string
	for _, v := range meta.Versions {
		if stableVersion.MatchString(v) {
			stable = append(stable, v)
		}
	}
	if len(stable) == 0 {
		return "", errors.New("aucune version stable trouvée")
	}
	sort.Slice(stable, func(i, j int) bool { return lessVersion(stable[i], stable[j]) })
	return stable[len(stable)-1], nil
}

func lessVersion(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}

func mavenInstall(dir, version string) error {
	major := strings.SplitN(version, ".", 2)[0]
	rel := fmt.Sprintf("/maven-%s/%s/binaries/apache-maven-%s-bin.zip", major, version, version)
	var sum string
	var err error
	for _, base := range mavenSums {
		if sum, err = httpGetBody(base + rel + ".sha512"); err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("somme SHA-512 introuvable : %w", err)
	}
	zipPath := filepath.Join(dir, "maven.zip")
	for _, base := range mavenMirrors {
		if err = downloadVerified(base+rel, zipPath, firstField(sum), sha512.New()); err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	defer os.Remove(zipPath)
	return unzipStrip(zipPath, dir)
}

// ---------------------------------------------------------------------------
// Poetry

func poetryLatest() (string, error) {
	var v struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := getJSON(poetryVersions, &v); err != nil {
		return "", err
	}
	if v.Info.Version == "" {
		return "", errors.New("version introuvable sur PyPI")
	}
	return v.Info.Version, nil
}

// poetryInstall lance l'installeur officiel avec POETRY_HOME : tout reste dans le dossier de l'outil,
// et poetry.exe est dans son sous-dossier bin. L'éditeur ne publie pas de somme pour ce script,
// qui est téléchargé en HTTPS depuis son domaine officiel.
func poetryInstall(dir, version string) error {
	python, ok := extToolPython()
	if !ok {
		return errors.New("Python est introuvable")
	}
	script, err := httpGetBody(poetryScript)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "install-poetry.py")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		return err
	}
	defer os.Remove(path)
	if code := runPython(python, []string{"POETRY_HOME=" + dir}, path, "--version", version); code != 0 {
		return fmt.Errorf("l'installeur de Poetry a échoué (code %d)", code)
	}
	return nil
}

func poetryRemove(dir string) {
	python, ok := extToolPython()
	if !ok {
		return
	}
	script, err := httpGetBody(poetryScript)
	if err != nil {
		return
	}
	path := filepath.Join(os.TempDir(), "install-poetry.py")
	if os.WriteFile(path, []byte(script), 0o644) == nil {
		runPython(python, []string{"POETRY_HOME=" + dir}, path, "--uninstall")
		os.Remove(path)
	}
}

func extToolPython() (string, bool) {
	return extTool{Needs: []string{"python", "py"}}.requirement()
}

// ---------------------------------------------------------------------------
// build-essential : MSVC

// buildEssentialNames : les noms qui installent le compilateur MSVC.
var buildEssentialNames = map[string]bool{
	"build-essential": true, "msvc": true, "buildtools": true, "vs-buildtools": true,
	"visual-studio-build-tools": true, "visualcpp-build-tools": true,
}

const msvcID = "Microsoft.VisualStudio.BuildTools"

// installBuildEssential installe les Build Tools de Visual Studio avec la charge de travail C++
// (compilateur cl, SDK Windows, CMake), ce qu'un simple « winget install » ne coche pas.
func installBuildEssential() int {
	ui.Info("build-essential → " + msvcID + " avec la charge de travail « Développement Desktop en C++ »")
	ui.Info("Le téléchargement fait plusieurs Go ; Windows demandera une confirmation (UAC).")
	code := winget("install", "--id", msvcID, "--exact", acceptSource, acceptPackage,
		"--override", "--wait --passive --norestart --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended")
	if code == 0 {
		ui.Info("cl.exe s'utilise depuis « Developer PowerShell for VS » ; CMake trouve MSVC tout seul.")
		ui.Info("Pour gcc dans le terminal : apt install gcc")
	}
	return code
}
