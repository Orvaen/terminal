package apt

// Outils fournis par un autre logiciel, et logiciels installés dans une autre version que celle de la table.
//
// pip n'est pas un logiciel à part sous Windows : il vient avec Python. « apt install pip » installe
// donc Python, mais « apt remove pip » ne doit surtout pas désinstaller Python ; et « apt upgrade pip »
// passe par pip lui-même (python -m pip install --upgrade pip). De même pour npm (Node.js), cargo (Rust)…
//
// La table vise une version précise (Python.Python.3.14) ; « apt remove python » et « apt upgrade python »
// visent celle qui est installée (Python.Python.3.12, par exemple).

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Orvaen/terminal/internal/ui"
)

type component struct {
	Provider string   // nom apt du logiciel qui le fournit
	Label    string   // son nom lisible
	Upgrade  []string // commande qui met l'outil à jour lui-même (nil : mettre à jour le fournisseur)
}

var pipUpgrade = []string{"python", "-m", "pip", "install", "--upgrade", "pip"}

var components = map[string]component{
	"pip":          {"python", "Python", pipUpgrade},
	"pip3":         {"python", "Python", pipUpgrade},
	"python3-pip":  {"python", "Python", pipUpgrade},
	"npm":          {"node", "Node.js", []string{"npm", "install", "--global", "npm@latest"}},
	"npx":          {"node", "Node.js", nil},
	"corepack":     {"node", "Node.js", nil},
	"cargo":        {"rust", "Rust (rustup)", []string{"rustup", "update"}},
	"rustc":        {"rust", "Rust (rustup)", []string{"rustup", "update"}},
	"g++":          {"gcc", "GCC (WinLibs)", nil},
	"gdb":          {"gcc", "GCC (WinLibs)", nil},
	"clang-format": {"llvm", "LLVM", nil},
	"lld":          {"llvm", "LLVM", nil},
	"javac":        {"java", "le JDK", nil},
	"dot":          {"graphviz", "Graphviz", nil},
}

// runTool lance une commande d'outil (pip, npm…) dans la console. Remplacée dans les tests.
var runTool = func(args []string) int {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		ui.Errorf("impossible de lancer %s : %v", args[0], err)
		return 127
	}
	return 0
}

func findComponent(name string) (component, bool) {
	c, ok := components[strings.ToLower(name)]
	return c, ok
}

func removeComponent(name string, c component) int {
	ui.Errorf("%s fait partie de %s : il ne se désinstalle pas seul.", name, c.Label)
	ui.Info(fmt.Sprintf("Pour retirer %s en entier (avec %s) : apt remove %s", c.Label, name, c.Provider))
	return 1
}

func upgradeComponent(name string, c component, upgradeProvider func(string) int) int {
	if c.Upgrade == nil {
		ui.Info(fmt.Sprintf("%s fait partie de %s : mise à jour de %s.", name, c.Label, c.Label))
		return upgradeProvider(c.Provider)
	}
	ui.Info(fmt.Sprintf("%s se met à jour lui-même : %s", name, strings.Join(c.Upgrade, " ")))
	return runTool(c.Upgrade)
}

// ---------------------------------------------------------------------------
// Version installée d'un logiciel versionné

var reNumSegment = regexp.MustCompile(`^\d+$`)

// familyPattern : les identifiants d'un même logiciel dans d'autres versions
// (Python.Python.3.14 → Python.Python.<n>.<n>, OpenJS.NodeJS.LTS → OpenJS.NodeJS[.LTS|.<n>]).
// nil si l'identifiant n'a pas de version.
func familyPattern(id string) *regexp.Regexp {
	if strings.EqualFold(id, "OpenJS.NodeJS.LTS") || strings.EqualFold(id, "OpenJS.NodeJS") {
		return regexp.MustCompile(`(?i)^OpenJS\.NodeJS(\.LTS|\.\d+)?$`)
	}
	parts := strings.Split(id, ".")
	versioned := false
	for i, p := range parts {
		if reNumSegment.MatchString(p) {
			parts[i] = `\d+`
			versioned = true
		} else {
			parts[i] = regexp.QuoteMeta(p)
		}
	}
	if !versioned {
		return nil
	}
	return regexp.MustCompile(`(?i)^` + strings.Join(parts, `\.`) + `$`)
}

// installedTargets : pour remove et upgrade, les identifiants réellement installés que désigne name.
// Sans correspondance (ou liste indisponible), renvoie l'identifiant de la table, comme avant.
func installedTargets(name string) []string {
	id, exact := Resolve(name)
	if !exact {
		return nil
	}
	installed, _ := exportInstalled()
	if len(installed) == 0 {
		return []string{id}
	}
	for _, i := range installed {
		if strings.EqualFold(i, id) {
			return []string{i}
		}
	}
	re := familyPattern(id)
	if re == nil {
		return []string{id}
	}
	var found []string
	for _, i := range installed {
		if re.MatchString(i) {
			found = append(found, i)
		}
	}
	if len(found) == 0 {
		return []string{id}
	}
	if !strings.EqualFold(found[0], id) {
		ui.Info(fmt.Sprintf("%s → %s (version installée)", name, strings.Join(found, ", ")))
	}
	return found
}

// exactTarget : la partie « quel paquet » d'une commande winget pour un identifiant exact.
func exactTarget(id string) []string { return []string{"--id", id, "--exact"} }
