package apt

// Trouver le bon logiciel quand le nom Linux ne ressemble pas à l'identifiant winget
// (code → Microsoft.VisualStudioCode, fd-find → sharkdp.fd, openjdk-21-jdk → EclipseAdoptium.Temurin.21.JDK).
//
// Ordre :
//  1. tes noms à toi (apt alias, et les choix que tu as faits), dans paquets.json ;
//  2. la table des noms connus (aliases.go) ;
//  3. winget, par moniker (le nom court déclaré par l'éditeur : vscode) ;
//  4. winget, par commande (le programme installé : code) — sous Linux, le paquet porte souvent ce nom ;
//  5. une recherche winget : un seul résultat évident est pris, sinon tu choisis et c'est retenu.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Orvaen/terminal/internal/ui"
)

// userAliasPath : fichier des noms personnels (remplacé dans les tests).
var userAliasPath = func() string {
	dir, err := os.UserConfigDir() // %APPDATA% sous Windows
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "OrvaenTerminal", "paquets.json")
}

func loadUserAliases() map[string]string {
	m := map[string]string{}
	if data, err := os.ReadFile(userAliasPath()); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

func saveUserAliases(m map[string]string) error {
	p := userAliasPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o644)
}

func userAlias(name string) (string, bool) {
	id, ok := loadUserAliases()[strings.ToLower(name)]
	return id, ok && id != ""
}

// windowsNotes : paquets Linux dont l'équivalent est déjà dans Windows ou s'installe autrement.
var windowsNotes = map[string]string{
	"openssh-client": "OpenSSH est déjà intégré à Windows : ssh, scp et sftp marchent tout de suite.",
	"openssh":        "OpenSSH est déjà intégré à Windows : ssh, scp et sftp marchent tout de suite.",
	"ssh":            "OpenSSH est déjà intégré à Windows : ssh, scp et sftp marchent tout de suite.",
	"tree":           "tree est déjà intégré à Windows.",
	"sudo":           "sudo est déjà intégré au terminal.",
	"tar":            "tar est déjà intégré à Windows.",
	"unzip":          "Windows ouvre les .zip tout seul ; en ligne de commande : tar -xf fichier.zip.",
	"zip":            "en ligne de commande : tar -a -cf archive.zip dossier (intégré à Windows), ou apt install 7zip.",
}

// interactive : peut-on poser une question (console, sans -y) ? Remplacée dans les tests.
var interactive = func() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// askChoice affiche les candidats et renvoie le numéro choisi (0 : aucun). Remplacée dans les tests.
var askChoice = func(name string, pkgs []Package) int {
	fmt.Fprintf(stdout, "Plusieurs logiciels correspondent à « %s » :\n", name)
	printCandidates(pkgs)
	fmt.Fprintf(stdout, "Lequel installer ? [1-%d, Entrée pour annuler] ", len(pkgs))
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(pkgs) {
		return 0
	}
	return n
}

func printCandidates(pkgs []Package) {
	for i, p := range pkgs {
		line := fmt.Sprintf("  %d. %-32s %s", i+1, p.ID, p.Name)
		if p.Source == "msstore" {
			line += "  (Microsoft Store)"
		}
		fmt.Fprintln(stdout, line)
	}
}

// resolution : ce que « apt install <nom> » va installer.
type resolution struct {
	id      string // identifiant winget exact ; "" : nom transmis tel quel à winget
	curated bool   // vient de la table : si winget ne le trouve plus, on cherche autrement
}

const maxChoices = 9

// resolvePackage trouve l'identifiant winget de name. ok=false : rien d'installable (message déjà affiché).
func resolvePackage(name string, yes bool) (resolution, bool) {
	if id, ok := userAlias(name); ok {
		ui.Info(fmt.Sprintf("%s → %s (ton nom, apt alias)", name, id))
		return resolution{id: id}, true
	}
	if id, ok := aliases[strings.ToLower(name)]; ok {
		if !strings.EqualFold(id, name) {
			ui.Info(fmt.Sprintf("%s → %s", name, id))
		}
		return resolution{id: id, curated: true}, true
	}
	// Un identifiant winget (Editeur.Logiciel) : winget sait faire.
	if strings.Contains(name, ".") && !strings.ContainsAny(name, " \t") {
		return resolution{}, true
	}
	return searchResolution(name, yes)
}

// searchResolution interroge winget : moniker, commande, puis recherche libre.
func searchResolution(name string, yes bool) (resolution, bool) {
	for _, by := range []struct{ flag, label string }{
		{"--moniker", "nom court winget"},
		{"--command", "fournit la commande « " + name + " »"},
	} {
		pkgs := preferWinget(searchPackages(by.flag, name, "--exact"))
		if len(pkgs) == 1 {
			ui.Info(fmt.Sprintf("%s → %s (%s)", name, pkgs[0].ID, by.label))
			return resolution{id: pkgs[0].ID}, true
		}
		if len(pkgs) > 1 {
			return choose(name, pkgs, yes)
		}
	}

	pkgs := preferWinget(searchPackages(name, "--count", "20"))
	if len(pkgs) == 0 {
		ui.Errorf("aucun logiciel ne correspond à « %s ». Cherche avec : apt search <mot>", name)
		return resolution{}, false
	}
	if best := strongMatches(name, pkgs); len(best) == 1 {
		ui.Info(fmt.Sprintf("%s → %s (%s)", name, best[0].ID, best[0].Name))
		return resolution{id: best[0].ID}, true
	}
	if len(pkgs) == 1 {
		ui.Info(fmt.Sprintf("%s → %s (%s)", name, pkgs[0].ID, pkgs[0].Name))
		return resolution{id: pkgs[0].ID}, true
	}
	return choose(name, pkgs, yes)
}

// preferWinget : les résultats du dépôt winget passent avant ceux du Microsoft Store,
// qui ne sont gardés que s'il n'y a rien d'autre.
func preferWinget(pkgs []Package) []Package {
	var main []Package
	for _, p := range pkgs {
		if p.Source != "msstore" {
			main = append(main, p)
		}
	}
	if len(main) > 0 {
		return main
	}
	return pkgs
}

// strongMatches : nom identique, ou identifiant qui se termine par le nom (Neovim.Neovim pour neovim).
func strongMatches(name string, pkgs []Package) []Package {
	var out []Package
	for _, p := range pkgs {
		last := p.ID[strings.LastIndex(p.ID, ".")+1:]
		if strings.EqualFold(p.Name, name) || strings.EqualFold(last, name) {
			out = append(out, p)
		}
	}
	return out
}

// choose fait choisir parmi plusieurs logiciels et retient le choix pour la prochaine fois.
// Sans console ou avec -y, ne devine pas : affiche les choix et s'arrête.
func choose(name string, pkgs []Package, yes bool) (resolution, bool) {
	if len(pkgs) > maxChoices {
		pkgs = pkgs[:maxChoices]
	}
	if yes || !interactive() {
		ui.Errorf("plusieurs logiciels correspondent à « %s » : précise lequel avec son identifiant.", name)
		printCandidates(pkgs)
		fmt.Fprintf(stdout, "Exemple : apt install %s   (ou retiens-le : apt alias %s %s)\n", pkgs[0].ID, name, pkgs[0].ID)
		return resolution{}, false
	}
	n := askChoice(name, pkgs)
	if n == 0 {
		ui.Info("Annulé.")
		return resolution{}, false
	}
	p := pkgs[n-1]
	m := loadUserAliases()
	m[strings.ToLower(name)] = p.ID
	if err := saveUserAliases(m); err == nil {
		ui.Info(fmt.Sprintf("Retenu : « apt install %s » installera %s. Pour changer : apt alias %s <identifiant>", name, p.ID, name))
	}
	return resolution{id: p.ID}, true
}

// cmdAlias gère tes noms : apt alias [nom identifiant | -r nom].
func cmdAlias(args []string) int {
	m := loadUserAliases()
	switch {
	case len(args) == 0:
		if len(m) == 0 {
			fmt.Fprintln(stdout, "Aucun nom personnel. Pour en créer un : apt alias <nom> <identifiant winget>")
			fmt.Fprintf(stdout, "(%d noms Linux sont déjà connus : apt help)\n", len(aliases))
			return 0
		}
		names := make([]string, 0, len(m))
		for n := range m {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintln(stdout, "Tes noms ("+userAliasPath()+") :")
		for _, n := range names {
			fmt.Fprintf(stdout, "  %-20s → %s\n", n, m[n])
		}
		return 0
	case (args[0] == "-r" || args[0] == "--remove") && len(args) == 2:
		key := strings.ToLower(args[1])
		if _, ok := m[key]; !ok {
			ui.Errorf("« %s » n'est pas un de tes noms.", args[1])
			return 1
		}
		delete(m, key)
		if err := saveUserAliases(m); err != nil {
			ui.Errorf("%v", err)
			return 1
		}
		ui.Info("« " + args[1] + " » oublié.")
		return 0
	case len(args) == 2 && !strings.HasPrefix(args[0], "-"):
		m[strings.ToLower(args[0])] = args[1]
		if err := saveUserAliases(m); err != nil {
			ui.Errorf("%v", err)
			return 1
		}
		ui.Info(fmt.Sprintf("« apt install %s » installera désormais %s.", args[0], args[1]))
		return 0
	}
	ui.Errorf("usage : apt alias [<nom> <identifiant winget>] | apt alias -r <nom>")
	return 1
}
