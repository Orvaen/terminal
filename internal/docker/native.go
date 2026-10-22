package docker

// Client docker natif Windows pour les builds.
//
// Dans la machine Linux, le client docker lit le dossier du build à travers /mnt/c, le partage de fichiers
// de WSL : chaque petit fichier y coûte cher, et un build de projet reste lent même avec BuildKit.
// Comme Docker Desktop, on lance donc les builds avec le client docker officiel pour Windows : il lit
// le dossier directement sur le disque et l'envoie au moteur en un seul flux, par une connexion TCP
// locale protégée par TLS (voir tls.go). Les autres commandes passent toujours par WSL.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Orvaen/terminal/internal/ui"
)

func cacheBase() string {
	base, err := os.UserCacheDir() // %LOCALAPPDATA% sous Windows
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "OrvaenTerminal")
}

// clientDir : le client docker Windows, ses plugins et ses certificats.
func clientDir() string     { return filepath.Join(cacheBase(), "docker-client") }
func clientExe() string     { return filepath.Join(clientDir(), "docker.exe") }
func clientConfig() string  { return filepath.Join(clientDir(), "config") }
func clientPlugins() string { return filepath.Join(clientConfig(), "cli-plugins") }

func clientInstalled() bool {
	return fileExists(clientExe()) &&
		fileExists(filepath.Join(clientPlugins(), "docker-buildx.exe")) &&
		fileExists(filepath.Join(clientPlugins(), "docker-compose.exe"))
}

// ---------------------------------------------------------------------------
// Quelles commandes passent par le client natif

// nativePlan : comment lancer une commande. native est lancé par le client Windows ; then, s'il n'est pas
// nil, est lancé ensuite par WSL (pour « compose up --build » : build natif, puis up sans --build).
type nativePlan struct {
	native []string
	then   []string
}

// planFor renvoie nil si la commande ne fait pas de build : elle passe alors entièrement par WSL.
func planFor(args []string) *nativePlan {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "build":
		return &nativePlan{native: args}
	case "buildx", "image", "builder":
		if len(args) > 1 && (args[1] == "build" || args[0] == "buildx" && args[1] == "b") {
			return &nativePlan{native: args}
		}
	case "compose":
		globals, sub, rest := splitCompose(args[1:])
		switch sub {
		case "build":
			return &nativePlan{native: args}
		case "up", "run", "create":
			if i := indexBefore(rest, "--build", "--"); i >= 0 {
				build := append(append([]string{"compose"}, globals...), "build")
				then := append(append(append([]string{"compose"}, globals...), sub), append(append([]string{}, rest[:i]...), rest[i+1:]...)...)
				return &nativePlan{native: build, then: then}
			}
		}
	}
	return nil
}

// composeValueFlags : options globales de docker compose suivies d'une valeur.
var composeValueFlags = map[string]bool{
	"-f": true, "--file": true, "-p": true, "--project-name": true, "--profile": true,
	"--env-file": true, "--project-directory": true, "--ansi": true, "--progress": true, "--parallel": true,
}

// splitCompose sépare « docker compose <options globales> <sous-commande> <reste> ».
func splitCompose(args []string) (globals []string, sub string, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return args[:i], a, args[i+1:]
		}
		if composeValueFlags[a] && i+1 < len(args) {
			i++
		}
	}
	return args, "", nil
}

// indexBefore : position de want dans args, en s'arrêtant à stop (après « -- », ce sont des arguments).
func indexBefore(args []string, want, stop string) int {
	for i, a := range args {
		if a == stop {
			return -1
		}
		if a == want {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Préparation et lancement

// nativeReady : le client Windows est installé et le moteur répond sur son port TCP (démarré si besoin).
// En cas de problème, renvoie false : la commande passe alors par WSL, comme avant.
func nativeReady() bool {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return false
	}
	if !clientInstalled() {
		ui.Info("Builds rapides : installation du client docker pour Windows (une seule fois)…")
		if err := installClient(); err != nil {
			ui.Warn("client docker Windows non installé (" + err.Error() + ") : le build passe par WSL, plus lent.")
			return false
		}
	}
	if err := ensureCerts(); err != nil {
		ui.Warn("certificats du moteur : " + err.Error())
		return false
	}
	if engineReachable() {
		return true
	}
	if engineRunning() {
		// Moteur démarré par une ancienne version, sans le port TCP : on ne coupe pas ses conteneurs.
		ui.Info("Pour des builds plus rapides, redémarre le moteur une fois : docker engine stop, puis relance ta commande.")
		return false
	}
	if startEngine() != 0 {
		return false
	}
	deadline := time.Now().Add(8 * time.Second) // le relais localhost de WSL peut mettre un instant
	for time.Now().Before(deadline) {
		if engineReachable() {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	ui.Warn("le moteur ne répond pas sur localhost:" + EnginePort + " (relais localhost de WSL désactivé ?) : le build passe par WSL.")
	return false
}

func engineReachable() bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+EnginePort, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// nativeEnv : environnement du client Windows, connecté au moteur de la machine par TLS.
func nativeEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "DOCKER_CONFIG":
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"DOCKER_HOST=tcp://127.0.0.1:"+EnginePort,
		"DOCKER_TLS_VERIFY=1",
		"DOCKER_CERT_PATH="+certsDir(),
		"DOCKER_CONFIG="+clientConfig(),
	)
}

func runNative(args []string) int {
	syncAuth()
	cmd := exec.Command(clientExe(), args...)
	cmd.Env = nativeEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return exitCode(cmd.Run())
}

// syncAuth recopie les identifiants de « docker login » (faits dans la machine) pour le client Windows,
// sans rien d'autre : un build peut ainsi tirer une image privée.
func syncAuth() {
	var data []byte
	for _, root := range []string{`\\wsl.localhost\`, `\\wsl$\`} {
		if b, err := os.ReadFile(root + Distro + `\root\.docker\config.json`); err == nil {
			data = b
			break
		}
	}
	if data == nil {
		return
	}
	out, ok := authsOnly(data)
	if !ok {
		return
	}
	path := filepath.Join(clientConfig(), "config.json")
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, out) {
		return
	}
	_ = os.MkdirAll(clientConfig(), 0o700)
	_ = os.WriteFile(path, out, 0o600)
}

// authsOnly garde seulement « auths » d'un config.json docker.
func authsOnly(data []byte) ([]byte, bool) {
	var cfg struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return nil, false
	}
	if cfg.Auths == nil {
		cfg.Auths = map[string]json.RawMessage{}
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	return out, err == nil
}

// ---------------------------------------------------------------------------
// Installation du client officiel (docker.exe, buildx, compose)

var (
	dockerStaticIndex = "https://download.docker.com/win/static/stable/x86_64/"
	githubAPI         = "https://api.github.com/repos/"
	githubDownload    = "https://github.com/"
	httpClient        = &http.Client{Timeout: 10 * time.Minute}
)

// installClient télécharge (ou met à jour) le client docker Windows et ses plugins buildx et compose.
func installClient() error {
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("client docker Windows disponible seulement en x86_64")
	}
	if err := os.MkdirAll(clientPlugins(), 0o755); err != nil {
		return err
	}

	ver, err := latestStaticDocker()
	if err != nil {
		return err
	}
	zipData, err := download("docker-"+ver+".zip", dockerStaticIndex+"docker-"+ver+".zip", "")
	if err != nil {
		return err
	}
	exe, err := extractFromZip(zipData, "docker/docker.exe")
	if err != nil {
		return err
	}
	if err := writeAtomic(clientExe(), exe); err != nil {
		return err
	}

	plugins := []struct{ repo, asset, dest string }{
		{"docker/buildx", "buildx-%s.windows-amd64.exe", "docker-buildx.exe"},
		{"docker/compose", "docker-compose-windows-x86_64.exe", "docker-compose.exe"},
	}
	for _, p := range plugins {
		tag, err := latestTag(p.repo)
		if err != nil {
			return err
		}
		asset := p.asset
		if strings.Contains(asset, "%s") {
			asset = fmt.Sprintf(asset, tag)
		}
		base := githubDownload + p.repo + "/releases/download/" + tag + "/"
		sums, err := download("", base+"checksums.txt", "")
		if err != nil {
			return err
		}
		sum, ok := checksumFor(sums, asset)
		if !ok {
			return fmt.Errorf("somme de contrôle introuvable pour %s", asset)
		}
		data, err := download(asset, base+asset, sum)
		if err != nil {
			return err
		}
		if err := writeAtomic(filepath.Join(clientPlugins(), p.dest), data); err != nil {
			return err
		}
	}
	return nil
}

var staticZip = regexp.MustCompile(`docker-(\d+)\.(\d+)\.(\d+)\.zip`)

func latestStaticDocker() (string, error) {
	page, err := download("", dockerStaticIndex, "")
	if err != nil {
		return "", err
	}
	return pickLatestStatic(string(page))
}

// pickLatestStatic : la version la plus récente parmi les docker-X.Y.Z.zip d'une page d'index.
func pickLatestStatic(page string) (string, error) {
	best, bestVer := "", [3]int{-1}
	for _, m := range staticZip.FindAllStringSubmatch(page, -1) {
		var v [3]int
		for i := range v {
			v[i], _ = strconv.Atoi(m[i+1])
		}
		if v[0] > bestVer[0] || v[0] == bestVer[0] && (v[1] > bestVer[1] || v[1] == bestVer[1] && v[2] > bestVer[2]) {
			best, bestVer = m[1]+"."+m[2]+"."+m[3], v
		}
	}
	if best == "" {
		return "", fmt.Errorf("aucune version de docker trouvée sur %s", dockerStaticIndex)
	}
	return best, nil
}

func latestTag(repo string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPI+repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("dernière version de %s : %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dernière version de %s : HTTP %d", repo, resp.StatusCode)
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil || rel.Tag == "" || strings.ContainsAny(rel.Tag, `/\`) {
		return "", fmt.Errorf("dernière version de %s illisible", repo)
	}
	return rel.Tag, nil
}

// checksumFor lit une ligne « <sha256>  [*]<fichier> » dans un checksums.txt.
func checksumFor(sums []byte, name string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return strings.ToLower(f[0]), true
		}
	}
	return "", false
}

// download récupère une URL en mémoire (barre de progression si label), et vérifie le SHA-256 si fourni.
func download(label, url, sha string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s : HTTP %d", url, resp.StatusCode)
	}
	var r io.Reader = resp.Body
	var bar *ui.Bar
	if label != "" {
		bar = ui.NewBar(label, resp.ContentLength)
		r = bar.Reader(r)
	}
	data, err := io.ReadAll(r)
	if bar != nil {
		if err != nil {
			bar.Abort()
		} else {
			bar.Finish()
		}
	}
	if err != nil {
		return nil, err
	}
	if sha != "" {
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != sha {
			return nil, fmt.Errorf("somme de contrôle invalide pour %s", label)
		}
	}
	return data, nil
}

func extractFromZip(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s absent de l'archive", name)
}

// writeAtomic écrit à côté puis renomme : un build en cours garde l'ancien fichier jusqu'au bout.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
