package apt

// Sources officielles.
//
// Le catalogue winget est alimenté par la communauté et peut avoir quelques jours de retard
// sur les éditeurs. Pour les logiciels où ce retard compte (Node.js), apt compare aussi la
// version installée avec la source officielle et peut installer directement depuis celle-ci.
// On reste sur la même version majeure, comme apt sous Linux : changer de majeure se demande.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Orvaen/terminal/internal/ui"
)

// Points d'injection, remplacés dans les tests.
var (
	officialEnabled = runtime.GOOS == "windows"
	nodeDist        = "https://nodejs.org/dist"
	httpClient      = &http.Client{Timeout: 2 * time.Minute}
	nodeInstalled   = func() (string, bool) {
		out, err := exec.Command("node", "-v").Output()
		if err != nil {
			return "", false
		}
		v := strings.TrimSpace(string(out))
		return v, strings.HasPrefix(v, "v")
	}
	runMSI = func(path string) int {
		// msiexec affiche lui-même la demande d'élévation (UAC) si besoin
		cmd := exec.Command("msiexec", "/i", path, "/passive", "/norestart")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode()
			}
			ui.Errorf("impossible de lancer l'installeur : %v", err)
			return 1
		}
		return 0
	}
)

const (
	msiRebootRequired = 3010
	msiUserCancelled  = 1602
)

type version [3]int

func parseVersion(s string) (version, bool) {
	var v version
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func (a version) newer(b version) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func (a version) String() string { return fmt.Sprintf("v%d.%d.%d", a[0], a[1], a[2]) }

func isNode(name string) bool {
	switch strings.ToLower(name) {
	case "node", "nodejs", "node.js", "openjs.nodejs.lts":
		return true
	}
	return false
}

// nodePlan : ce que nodejs.org propose par rapport à la version installée.
type nodePlan struct {
	installed   version
	target      version // dernière version de la même majeure (zéro si déjà à jour)
	newMajorLTS version // LTS d'une majeure plus récente
	newCurrent  version // version « Current » d'une majeure plus récente
}

type nodeRelease struct {
	Version string          `json:"version"`
	LTS     json.RawMessage `json:"lts"`
	Files   []string        `json:"files"`
}

func (r nodeRelease) isLTS() bool {
	s := strings.TrimSpace(string(r.LTS))
	return s != "" && s != "false" && s != "null"
}

func nodeMSIKey() string {
	if runtime.GOARCH == "arm64" {
		return "win-arm64-msi"
	}
	if runtime.GOARCH == "386" {
		return "win-x86-msi"
	}
	return "win-x64-msi"
}

func nodeMSIArch() string {
	return strings.TrimSuffix(strings.TrimPrefix(nodeMSIKey(), "win-"), "-msi")
}

// checkNode renvoie ok=false si Node.js n'est pas installé (ou si la vérification est désactivée).
func checkNode() (plan nodePlan, ok bool, err error) {
	if !officialEnabled {
		return plan, false, nil
	}
	cur, found := nodeInstalled()
	if !found {
		return plan, false, nil
	}
	installed, valid := parseVersion(cur)
	if !valid {
		return plan, false, nil
	}
	plan.installed = installed

	resp, err := httpClient.Get(nodeDist + "/index.json")
	if err != nil {
		return plan, true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return plan, true, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var releases []nodeRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return plan, true, err
	}

	// index.json ne documente pas toujours l'installeur ARM64 : l'archive ARM64 suffit comme
	// signal, la présence du .msi est de toute façon vérifiée par SHASUMS256.txt.
	keys := []string{nodeMSIKey()}
	if runtime.GOARCH == "arm64" {
		keys = append(keys, "win-arm64-zip", "win-arm64-7z")
	}
	for _, r := range releases {
		v, valid := parseVersion(r.Version)
		if !valid || !containsAny(r.Files, keys) {
			continue
		}
		switch {
		case v[0] == installed[0]:
			if v.newer(installed) && v.newer(plan.target) {
				plan.target = v
			}
		case v[0] > installed[0] && r.isLTS():
			if v.newer(plan.newMajorLTS) {
				plan.newMajorLTS = v
			}
		case v[0] > installed[0]:
			if v.newer(plan.newCurrent) {
				plan.newCurrent = v
			}
		}
	}
	return plan, true, nil
}

func containsAny(list, wanted []string) bool {
	for _, x := range list {
		for _, w := range wanted {
			if x == w {
				return true
			}
		}
	}
	return false
}

// upgradeNode met Node.js à jour depuis nodejs.org, dans sa version majeure.
// Renvoie handled=false si Node.js n'est pas installé : l'appelant passe alors par winget.
func upgradeNode(quietIfCurrent bool) (code int, handled bool) {
	plan, ok, err := checkNode()
	if !ok {
		return 0, false
	}
	if err != nil {
		ui.Errorf("nodejs.org injoignable : %v", err)
		return 1, true
	}
	var zero version
	if plan.target == zero {
		if !quietIfCurrent {
			ui.Info(fmt.Sprintf("node %s est déjà à jour.", plan.installed))
		}
		return 0, true
	}
	ui.Step(fmt.Sprintf("Mise à jour de node %s → %s depuis nodejs.org", plan.installed, plan.target))
	msi, err := downloadNodeMSI(plan.target)
	if err != nil {
		ui.Errorf("%v", err)
		return 1, true
	}
	defer os.RemoveAll(filepath.Dir(msi))

	switch code := runMSI(msi); code {
	case 0:
		ui.Info(fmt.Sprintf("node %s est installé. Ouvre un nouveau terminal pour l'utiliser.", plan.target))
		return 0, true
	case msiRebootRequired:
		ui.Warn("Un redémarrage est nécessaire pour terminer l'installation de node.")
		return 0, true
	case msiUserCancelled:
		ui.Errorf("installation de node annulée.")
		return code, true
	default:
		ui.Errorf("l'installeur de node a échoué (code %d).", code)
		return code, true
	}
}

// downloadNodeMSI télécharge l'installeur officiel et vérifie sa somme SHA-256 (SHASUMS256.txt).
func downloadNodeMSI(v version) (string, error) {
	name := fmt.Sprintf("node-%s-%s.msi", v, nodeMSIArch())
	base := fmt.Sprintf("%s/%s/", nodeDist, v)

	sums, err := httpGetBody(base + "SHASUMS256.txt")
	if err != nil {
		return "", fmt.Errorf("sommes de contrôle de node : %w", err)
	}
	want := ""
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return "", fmt.Errorf("%s absent de SHASUMS256.txt", name)
	}

	resp, err := httpClient.Get(base + name)
	if err != nil {
		return "", fmt.Errorf("téléchargement de node : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("téléchargement de node : HTTP %d", resp.StatusCode)
	}
	dir, err := os.MkdirTemp("", "apt-node-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	bar := ui.NewBar(name, resp.ContentLength)
	_, err = io.Copy(io.MultiWriter(out, h, bar), resp.Body)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		bar.Abort()
		os.RemoveAll(dir)
		return "", fmt.Errorf("téléchargement de node : %w", err)
	}
	bar.Finish()
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		os.RemoveAll(dir)
		return "", fmt.Errorf("somme de contrôle invalide pour %s", name)
	}
	return path, nil
}

func httpGetBody(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}
