// Package update met à jour Orvaen Terminal à partir des releases GitHub, sans réinstaller.
//
// Sous Windows, on ne peut pas écraser un .exe en cours d'exécution, mais on peut le renommer :
// chaque exécutable est donc renommé en .old, puis la nouvelle version prend sa place. Les fenêtres
// déjà ouvertes continuent avec l'ancienne version jusqu'à leur fermeture, et ce qui tourne
// (Docker, conteneurs, serveurs lancés depuis le terminal) n'est pas interrompu. Les .old sont
// supprimés au lancement suivant.
package update

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Orvaen/terminal/internal/ui"
)

// APIBase : API GitHub du dépôt (remplacée dans les tests).
var APIBase = "https://api.github.com/repos/Orvaen/terminal"

// SumsFile : fichier des sommes SHA-256 publié avec chaque release.
const SumsFile = "SHA256SUMS.txt"

var client = &http.Client{Timeout: 5 * time.Minute}

// targets : fichiers du zip et noms sous lesquels ils sont installés.
var targets = []struct {
	src      string
	dests    []string
	optional bool
}{
	{"terminal.exe", []string{"terminal.exe"}, false},
	{"apt.exe", []string{"apt.exe", "apt-get.exe"}, false},
	{"docker.exe", []string{"docker.exe"}, false},
	{"req.exe", []string{"req.exe"}, false},
	{"terminal.ico", []string{"terminal.ico"}, true},
}

// Release : la partie de la réponse GitHub qui nous intéresse.
type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

// Asset : un fichier attaché à une release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func (r Release) asset(match func(string) bool) (Asset, bool) {
	for _, a := range r.Assets {
		if match(a.Name) {
			return a, true
		}
	}
	return Asset{}, false
}

// IsDev indique une version compilée localement (build.ps1), pas une release.
func IsDev(version string) bool {
	return version == "" || version == "dev" || strings.HasPrefix(version, "dev-")
}

// archSuffix : fin du nom du zip pour cette machine.
func archSuffix(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "-x64.zip", nil
	case "arm64":
		return "-arm64.zip", nil
	}
	return "", fmt.Errorf("architecture non prise en charge : %s", goarch)
}

// Latest renvoie la dernière release publiée.
func Latest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "orvaen-terminal")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("impossible de joindre GitHub : %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Release{}, errors.New("aucune release publiée pour le moment")
	default:
		return Release{}, fmt.Errorf("GitHub a répondu HTTP %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("réponse de GitHub illisible : %w", err)
	}
	if rel.Tag == "" {
		return Release{}, errors.New("réponse de GitHub sans numéro de version")
	}
	return rel, nil
}

// InstallDir : dossier d'installation (celui d'install.ps1), sinon celui de l'exécutable.
func InstallDir() (string, error) {
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		dir := filepath.Join(base, "Programs", "apt-windows")
		if _, err := os.Stat(filepath.Join(dir, "terminal.exe")); err == nil {
			return dir, nil
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// Run cherche et installe la dernière version. force : réinstalle même si la version est identique.
func Run(current string, force bool) int {
	ui.Step("Recherche d'une mise à jour d'Orvaen Terminal")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	rel, err := Latest(ctx)
	cancel()
	if err != nil {
		ui.Errorf("%v", err)
		return 1
	}
	saveCheck(rel.Tag)
	if rel.Tag == current && !force {
		ui.Info("Orvaen Terminal est déjà à jour (" + current + ").")
		return 0
	}

	dir, err := InstallDir()
	if err != nil {
		ui.Errorf("dossier d'installation introuvable : %v", err)
		return 1
	}
	ui.Step(fmt.Sprintf("Mise à jour %s → %s", current, rel.Tag))
	if err := Apply(rel, dir, runtime.GOARCH); err != nil {
		ui.Errorf("la mise à jour a échoué, rien n'a été modifié : %v", err)
		return 1
	}
	ui.Info("Orvaen Terminal est à jour (" + rel.Tag + ").")
	ui.Info("Les fenêtres déjà ouvertes gardent l'ancienne version jusqu'à leur fermeture ;")
	ui.Info("ce qui tourne (Docker, conteneurs, serveurs) continue sans interruption.")
	return 0
}

// Apply télécharge la release, vérifie sa somme SHA-256 et remplace les fichiers de dir.
func Apply(rel Release, dir, goarch string) error {
	suffix, err := archSuffix(goarch)
	if err != nil {
		return err
	}
	zipAsset, ok := rel.asset(func(n string) bool { return strings.HasSuffix(strings.ToLower(n), suffix) })
	if !ok {
		return fmt.Errorf("la release %s ne contient pas de zip *%s", rel.Tag, suffix)
	}
	sumsAsset, ok := rel.asset(func(n string) bool { return n == SumsFile })
	if !ok {
		return fmt.Errorf("la release %s ne contient pas %s : impossible de vérifier le téléchargement", rel.Tag, SumsFile)
	}

	tmp, err := os.MkdirTemp("", "orvaen-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	sumsPath := filepath.Join(tmp, SumsFile)
	if _, err := download(sumsAsset.URL, sumsPath); err != nil {
		return err
	}
	sums, err := readSums(sumsPath)
	if err != nil {
		return err
	}
	want, ok := sums[zipAsset.Name]
	if !ok {
		return fmt.Errorf("%s n'a pas de somme dans %s", zipAsset.Name, SumsFile)
	}

	zipPath := filepath.Join(tmp, "release.zip")
	got, err := download(zipAsset.URL, zipPath)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("somme de contrôle invalide pour %s (attendu %s, obtenu %s)", zipAsset.Name, want, got)
	}

	staged, err := stage(zipPath, dir)
	if err != nil {
		return err
	}
	return swap(staged)
}

// download enregistre url dans dest et renvoie sa somme SHA-256.
func download(url, dest string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "orvaen-terminal")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("téléchargement : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("téléchargement de %s : HTTP %d", path.Base(url), resp.StatusCode)
	}
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var w io.Writer = io.MultiWriter(out, h)
	var bar *ui.Bar
	if resp.ContentLength > 64<<10 { // pas de barre pour le petit fichier des sommes
		bar = ui.NewBar(path.Base(url), resp.ContentLength)
		w = io.MultiWriter(out, h, bar)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		if bar != nil {
			bar.Abort()
		}
		out.Close()
		return "", fmt.Errorf("téléchargement : %w", err)
	}
	if bar != nil {
		bar.Finish()
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// readSums lit un fichier au format de sha256sum : « <somme>  <fichier> ».
func readSums(p string) (map[string]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		sums[name] = strings.ToLower(fields[0])
	}
	return sums, sc.Err()
}

type staged struct{ tmp, dest string }

// stage extrait les fichiers du zip à côté de leur destination (nom.new), sans rien remplacer.
func stage(zipPath, dir string) ([]staged, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("zip illisible : %w", err)
	}
	defer zr.Close()
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			entries[strings.ToLower(path.Base(strings.ReplaceAll(f.Name, `\`, "/")))] = f
		}
	}

	var out []staged
	cleanup := func() {
		for _, s := range out {
			os.Remove(s.tmp)
		}
	}
	for _, t := range targets {
		f, ok := entries[t.src]
		if !ok {
			if t.optional {
				continue
			}
			cleanup()
			return nil, fmt.Errorf("%s manque dans le zip", t.src)
		}
		for _, d := range t.dests {
			s := staged{tmp: filepath.Join(dir, d+".new"), dest: filepath.Join(dir, d)}
			if err := extract(f, s.tmp); err != nil {
				cleanup()
				return nil, err
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func extract(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

type swapped struct{ dest, old string }

// swap met chaque fichier en place : l'ancien est renommé en .old (possible même s'il tourne),
// puis le nouveau prend son nom. En cas d'erreur, tout est remis comme avant.
func swap(files []staged) error {
	var done []swapped
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			os.Remove(done[i].dest)
			if done[i].old != "" {
				os.Rename(done[i].old, done[i].dest)
			}
		}
		for _, s := range files {
			os.Remove(s.tmp)
		}
	}
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	for _, s := range files {
		sw := swapped{dest: s.dest}
		if _, err := os.Stat(s.dest); err == nil {
			sw.old = s.dest + "." + stamp + ".old"
			if err := os.Rename(s.dest, sw.old); err != nil {
				rollback()
				return fmt.Errorf("impossible de remplacer %s : %w", filepath.Base(s.dest), err)
			}
		}
		if err := os.Rename(s.tmp, s.dest); err != nil {
			if sw.old != "" {
				os.Rename(sw.old, s.dest)
			}
			rollback()
			return fmt.Errorf("impossible d'installer %s : %w", filepath.Base(s.dest), err)
		}
		done = append(done, sw)
	}
	return nil
}

// Cleanup supprime les anciennes versions (.old) et les restes de mise à jour (.new) de dir.
// Un .old encore utilisé par une fenêtre ouverte reste en place jusqu'au prochain nettoyage.
func Cleanup(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() {
			continue
		}
		switch {
		case strings.HasSuffix(n, ".old"):
		case strings.HasSuffix(n, ".exe.new"), strings.HasSuffix(n, ".ico.new"):
			// Ne pas gêner une mise à jour en cours dans une autre fenêtre.
			if info, err := e.Info(); err != nil || time.Since(info.ModTime()) < 10*time.Minute {
				continue
			}
		default:
			continue
		}
		os.Remove(filepath.Join(dir, n))
	}
}
