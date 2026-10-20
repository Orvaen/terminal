package apt

// Noms proposés par la complétion (Tab) : logiciels installés, noms connus, sous-commandes.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Subcommands : les sous-commandes d'apt, dans l'ordre d'usage.
var Subcommands = []string{
	"update", "upgrade", "full-upgrade", "dist-upgrade", "install", "remove", "purge",
	"search", "show", "list", "windows-update", "autoremove", "clean", "help", "version",
}

// PackageSubcommand indique si la sous-commande attend des logiciels installés (true)
// ou des logiciels à installer (false), et si elle attend des noms de paquets du tout (ok).
func PackageSubcommand(sub string) (installed bool, ok bool) {
	switch strings.ToLower(sub) {
	case "remove", "purge", "uninstall", "upgrade", "show":
		return true, true
	case "install":
		return false, true
	}
	return false, false
}

// InstallableNames : noms courts connus (alias) et docker.
func InstallableNames() []string {
	return append([]string{"docker"}, AliasNames()...)
}

const installedTTL = 5 * time.Minute

var (
	instMu      sync.Mutex
	instNames   []string
	instAt      time.Time
	instLoading chan struct{}

	// exportInstalled liste les identifiants winget des logiciels installés (injectable).
	exportInstalled = wingetExport
)

// PrefetchInstalled lance en arrière-plan la lecture des logiciels installés si elle est périmée.
func PrefetchInstalled() { startLoad() }

// InvalidateInstalled oublie la liste (après une installation ou une désinstallation).
func InvalidateInstalled() {
	instMu.Lock()
	instAt = time.Time{}
	instMu.Unlock()
}

// InstalledNames renvoie les noms des logiciels installés, en attendant au plus `wait`
// si la liste est en cours de lecture. La liste peut être vide si winget est lent.
func InstalledNames(wait time.Duration) []string {
	done := startLoad()
	if done != nil {
		select {
		case <-done:
		case <-time.After(wait):
		}
	}
	instMu.Lock()
	defer instMu.Unlock()
	return append([]string(nil), instNames...)
}

func startLoad() chan struct{} {
	instMu.Lock()
	defer instMu.Unlock()
	if instLoading != nil {
		return instLoading
	}
	if !instAt.IsZero() && time.Since(instAt) < installedTTL {
		return nil
	}
	ch := make(chan struct{})
	instLoading = ch
	go func() {
		ids, _ := exportInstalled()
		names := namesFromIDs(ids, dockerInstalled())
		instMu.Lock()
		instNames, instAt, instLoading = names, time.Now(), nil
		instMu.Unlock()
		close(ch)
	}()
	return ch
}

// namesFromIDs : identifiants winget, plus le nom court quand un alias pointe dessus.
func namesFromIDs(ids []string, docker bool) []string {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			names = append(names, n)
		}
	}
	installed := map[string]bool{}
	for _, id := range ids {
		installed[strings.ToLower(id)] = true
	}
	for _, alias := range AliasNames() {
		if id, _ := Resolve(alias); installed[strings.ToLower(id)] {
			add(alias)
		}
	}
	if docker {
		add("docker")
	}
	sort.Slice(ids, func(i, j int) bool { return strings.ToLower(ids[i]) < strings.ToLower(ids[j]) })
	for _, id := range ids {
		add(id)
	}
	return names
}

func wingetExport() ([]string, error) {
	if runtime.GOOS != "windows" || !wingetAvailable() {
		return nil, nil
	}
	dir, err := os.MkdirTemp("", "apt-export-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "installed.json")
	// winget signale sur sa sortie les logiciels qu'il ne sait pas mettre à jour : on l'ignore.
	_ = exec.Command("winget", "export", "-o", path, acceptSource).Run()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseExport(data)
}

func parseExport(data []byte) ([]string, error) {
	var doc struct {
		Sources []struct {
			Packages []struct {
				PackageIdentifier string
			}
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var ids []string
	for _, src := range doc.Sources {
		for _, p := range src.Packages {
			if p.PackageIdentifier != "" {
				ids = append(ids, p.PackageIdentifier)
			}
		}
	}
	return ids, nil
}
