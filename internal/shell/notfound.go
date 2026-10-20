package shell

// Commande introuvable : comme sous Linux, un message court qui dit comment l'installer,
// au lieu de l'erreur de PowerShell.
//
// Ce qui n'est ni une commande intégrée ni un programme est confié à PowerShell (Get-Process, dir…).
// Pour savoir si PowerShell connaît la commande sans le lancer à chaque fois, le terminal garde la
// liste de ses commandes (cmdlets, fonctions, alias) dans un fichier, rafraîchi chaque semaine.
//
// Avant de déclarer une commande introuvable, le terminal relit le PATH de Windows : un logiciel
// installé depuis l'ouverture de la fenêtre (apt install hurl) marche alors tout de suite.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/platform"
)

// Points d'injection, remplacés dans les tests.
var (
	goos             = runtime.GOOS
	refreshPath      = platform.RefreshPath
	findInstalledExe = platform.FindInstalledExe
	psCacheFile      = func() string {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		return filepath.Join(base, "OrvaenTerminal", "commandes-powershell.txt")
	}
	// listPSCommands demande à PowerShell la liste de ses commandes.
	listPSCommands = func() ([]string, error) {
		ps := ""
		for _, name := range []string{"pwsh", "powershell"} {
			if p, err := exec.LookPath(name); err == nil {
				ps = p
				break
			}
		}
		if ps == "" {
			return nil, os.ErrNotExist
		}
		out, err := exec.Command(ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
			"Get-Command -CommandType Alias,Function,Cmdlet | ForEach-Object Name").Output()
		if err != nil {
			return nil, err
		}
		return strings.Fields(string(out)), nil
	}
)

const psCacheTTL = 7 * 24 * time.Hour

var psCommands struct {
	sync.Mutex
	names  map[string]bool // nil : liste pas encore connue
	loaded bool
}

// loadPSCommands lit la liste en cache, et la rafraîchit en arrière-plan si elle est ancienne.
func loadPSCommands() {
	psCommands.Lock()
	if psCommands.loaded {
		psCommands.Unlock()
		return
	}
	psCommands.loaded = true
	psCommands.Unlock()

	path := psCacheFile()
	fresh := false
	if info, err := os.Stat(path); err == nil {
		if f, err := os.Open(path); err == nil {
			names := map[string]bool{}
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if n := strings.TrimSpace(sc.Text()); n != "" {
					names[strings.ToLower(n)] = true
				}
			}
			f.Close()
			if len(names) > 0 {
				psCommands.Lock()
				psCommands.names = names
				psCommands.Unlock()
				fresh = time.Since(info.ModTime()) < psCacheTTL
			}
		}
	}
	if !fresh {
		go refreshPSCommands()
	}
}

func refreshPSCommands() {
	list, err := listPSCommands()
	if err != nil || len(list) == 0 {
		return
	}
	names := map[string]bool{}
	for _, n := range list {
		names[strings.ToLower(n)] = true
	}
	psCommands.Lock()
	psCommands.names = names
	psCommands.Unlock()
	path := psCacheFile()
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = os.WriteFile(path, []byte(strings.Join(list, "\n")+"\n"), 0o644)
	}
}

// psKnows : PowerShell connaît-il cette commande ? known=false si la liste n'est pas encore là.
func psKnows(name string) (knows, known bool) {
	psCommands.Lock()
	defer psCommands.Unlock()
	if psCommands.names == nil {
		return false, false
	}
	return psCommands.names[strings.ToLower(name)], true
}

// isBareName : un nom de commande simple (pas un chemin, pas un script).
func isBareName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\:.$&|<>"'`+"`")
}

// lookAgain relit le PATH de Windows et cherche de nouveau la commande.
func lookAgain(name string) (string, bool) {
	if goos != "windows" || !refreshPath() {
		return "", false
	}
	resetCommandNames()
	p, err := exec.LookPath(name)
	return p, err == nil
}

// notFound décide quoi faire d'une commande inconnue : true si le terminal a répondu lui-même
// (code), false s'il faut la confier à PowerShell.
func (s *Shell) notFound(name string) (int, bool) {
	if !isBareName(name) {
		return 0, false
	}
	if knows, known := psKnows(name); !known || knows {
		return 0, false
	}
	// Installé, mais l'installeur n'a pas ajouté son dossier au PATH ?
	if goos == "windows" {
		if dir := findInstalledExe(name); dir != "" {
			os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			resetCommandNames()
			if _, err := exec.LookPath(name); err == nil {
				fmt.Fprintf(s.errOut, "(%s trouvé dans %s, ajouté au PATH de ce terminal)\n", name, dir)
				return -1, false
			}
		}
	}
	s.errorf("%s : commande introuvable", name)
	if hint := installHint(name); hint != "" {
		s.errorf("%s", hint)
	}
	return 127, true
}

// installHint : comment obtenir une commande, d'après les noms qu'apt connaît.
func installHint(name string) string {
	low := strings.ToLower(name)
	if coreutilsCommands[low] && coreutilsDir() == "" {
		return "Elle fait partie des commandes Unix de Microsoft : apt install coreutils"
	}
	if id, exact := apt.Resolve(low); exact {
		return fmt.Sprintf("Pour l'installer : apt install %s   (%s)", low, id)
	}
	return fmt.Sprintf("Si c'est un logiciel, cherche-le : apt install %s (apt cherche dans winget par nom et par commande)", low)
}
