package apt

// Installation sans droits administrateur.
//
// Certains installeurs winget n'existent qu'en version « pour tout l'ordinateur » : l'installeur MSI
// de Node.js, par exemple, demande l'élévation (UAC). Sur un poste sans droits administrateur
// (entreprise, école), la demande est refusée et l'installation est abandonnée.
//
// Comme sous Linux, « apt install » installe donc pour toi seul, et « sudo apt install » pour tout
// l'ordinateur :
//
//   - Node.js et Go : la version portable officielle (zip), vérifiée par sa somme SHA-256, rangée
//     dans %LOCALAPPDATA%\OrvaenTerminal\outils et ajoutée à ton PATH utilisateur ;
//   - les autres : winget avec --scope user ; s'il n'existe aucun installeur pour l'utilisateur,
//     winget installe pour tout l'ordinateur (et Windows demande la confirmation).

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Orvaen/terminal/internal/ui"
)

// APPINSTALLER_CLI_ERROR_NO_APPLICABLE_INSTALLER : aucun installeur ne correspond (ici : pas de --scope user).
const wingetNoApplicableInstaller = 0x8A150010

// Points d'injection, remplacés dans les tests.
var (
	portableEnabled = runtime.GOOS == "windows"
	goDownloads     = "https://go.dev/dl/?mode=json"
	goDist          = "https://go.dev/dl/"
)

var portableTools = []extTool{
	{
		Name: "node", Label: "Node.js", Cmd: "node",
		Aliases: []string{"nodejs", "node.js", "nodejs-lts", "openjs.nodejs.lts"},
		Source:  "nodejs.org (zip portable, dernière LTS), somme SHA-256 vérifiée",
		latest:  nodePortableLatest, install: nodePortableInstall, extra: npmGlobalDir,
	},
	{
		Name: "go", Label: "Go", Cmd: "go", Bin: "bin",
		Aliases: []string{"golang", "golang-go", "golang.go"},
		Source:  "go.dev (zip portable), somme SHA-256 vérifiée",
		latest:  goPortableLatest, install: goPortableInstall, extra: goBinDir,
	},
}

func findPortable(name string) (extTool, bool) {
	n := strings.ToLower(name)
	for _, t := range portableTools {
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

// portableInstalled : l'outil a été installé en portable par apt.
func portableInstalled(name string) (extTool, bool) {
	if !portableEnabled {
		return extTool{}, false
	}
	t, ok := findPortable(name)
	if !ok {
		return extTool{}, false
	}
	_, installed := t.installedVersion()
	return t, installed
}

// usePortable : installer la version portable plutôt que passer par winget.
// Oui si elle est déjà là, ou si on n'est pas administrateur et que la commande n'existe pas encore
// (une installation existante, faite par winget, reste gérée par winget).
func usePortable(name string) (extTool, bool) {
	if !portableEnabled {
		return extTool{}, false
	}
	t, ok := findPortable(name)
	if !ok {
		return extTool{}, false
	}
	if _, installed := t.installedVersion(); installed {
		return t, true
	}
	if isElevated() {
		return extTool{}, false
	}
	if p, err := toolLookPath(t.Cmd); err == nil && p != "" {
		return extTool{}, false
	}
	return t, true
}

// installUserScope : winget pour l'utilisateur seul quand on n'est pas administrateur,
// puis pour tout l'ordinateur si le logiciel n'existe pas autrement.
func installUserScope(name string, res resolution) int {
	t := []string{name}
	if res.id != "" {
		t = []string{"--id", res.id, "--exact"}
	}
	args := append([]string{"install"}, t...)
	if !portableEnabled || isElevated() {
		return winget(append(args, acceptSource, acceptPackage)...)
	}
	code := winget(append(args, "--scope", "user", acceptSource, acceptPackage)...)
	if uint32(code) != wingetNoApplicableInstaller {
		return code
	}
	ui.Warn(name + " n'existe pas en installation pour l'utilisateur seul : installation pour tout l'ordinateur.")
	ui.Info("Windows va demander les droits administrateur. Sans eux, l'installation sera abandonnée.")
	code = winget(append(args, acceptSource, acceptPackage)...)
	if code != 0 && !isNotApplicable(code) {
		ui.Errorf("%s demande les droits administrateur pour s'installer (code %d). Demande-les, ou lance : sudo apt install %s", name, code, name)
	}
	return code
}

// ---------------------------------------------------------------------------
// Node.js portable

func nodeZipArch() string { return nodeMSIArch() } // x64, arm64, x86

func npmGlobalDir() []string {
	// Le npmrc du zip place les paquets globaux (npm install -g) dans %APPDATA%\npm.
	if d := os.Getenv("APPDATA"); d != "" {
		return []string{filepath.Join(d, "npm")}
	}
	return nil
}

func nodePortableLatest() (string, error) {
	var releases []nodeRelease
	if err := getJSON(nodeDist+"/index.json", &releases); err != nil {
		return "", fmt.Errorf("nodejs.org : %w", err)
	}
	key := "win-" + nodeZipArch() + "-zip"
	for _, r := range releases { // du plus récent au plus ancien
		if r.isLTS() && containsAny(r.Files, []string{key}) {
			if _, ok := parseVersion(r.Version); ok {
				return r.Version, nil
			}
		}
	}
	return "", fmt.Errorf("aucune LTS pour Windows %s", nodeZipArch())
}

func nodePortableInstall(dir, v string) error {
	name := fmt.Sprintf("node-%s-win-%s.zip", v, nodeZipArch())
	base := fmt.Sprintf("%s/%s/", nodeDist, v)
	sums, err := httpGetBody(base + "SHASUMS256.txt")
	if err != nil {
		return fmt.Errorf("sommes de contrôle de node : %w", err)
	}
	want := ""
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && f[1] == name {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("%s absent de SHASUMS256.txt", name)
	}
	return downloadUnzip(base+name, dir, want)
}

// ---------------------------------------------------------------------------
// Go portable

type goRelease struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Files   []struct {
		Filename string `json:"filename"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Kind     string `json:"kind"`
		SHA256   string `json:"sha256"`
	} `json:"files"`
}

func goBinDir() []string {
	// go install range ses programmes dans %USERPROFILE%\go\bin.
	if h, err := os.UserHomeDir(); err == nil {
		return []string{filepath.Join(h, "go", "bin")}
	}
	return nil
}

func goPortableFile() (version, file, sum string, err error) {
	var rels []goRelease
	if err := getJSON(goDownloads, &rels); err != nil {
		return "", "", "", fmt.Errorf("go.dev : %w", err)
	}
	for _, r := range rels {
		if !r.Stable {
			continue
		}
		for _, f := range r.Files {
			if f.OS == "windows" && f.Arch == runtime.GOARCH && f.Kind == "archive" && strings.HasSuffix(f.Filename, ".zip") {
				return r.Version, f.Filename, f.SHA256, nil
			}
		}
	}
	return "", "", "", fmt.Errorf("aucune version de Go pour Windows %s", runtime.GOARCH)
}

func goPortableLatest() (string, error) {
	v, _, _, err := goPortableFile()
	return v, err
}

func goPortableInstall(dir, v string) error {
	got, file, sum, err := goPortableFile()
	if err != nil {
		return err
	}
	if got != v {
		return fmt.Errorf("la dernière version de Go a changé pendant l'installation (%s → %s), recommence", v, got)
	}
	return downloadUnzip(goDist+file, dir, sum)
}

// downloadUnzip télécharge un zip, vérifie sa somme SHA-256 et l'extrait dans dir (sans le dossier racine).
func downloadUnzip(url, dir, sum string) error {
	tmp, err := os.MkdirTemp("", "apt-portable-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	zipPath := filepath.Join(tmp, filepath.Base(url))
	if err := downloadVerified(url, zipPath, sum, sha256.New()); err != nil {
		return err
	}
	return unzipStrip(zipPath, dir)
}
