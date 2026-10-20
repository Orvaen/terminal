package apt

// apt deps : les outils dont le projet a besoin, et leur installation (apt deps install,
// ou apt build-dep comme sous Debian).

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Orvaen/terminal/internal/deps"
	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// Remplacées dans les tests.
var (
	refreshPath = refreshPathFromRegistry
	runStep     = func(dir string, args []string) int {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			ui.Errorf("impossible de lancer %s : %v", args[0], err)
			return 127
		}
		return 0
	}
	confirm = func(question string) bool {
		fmt.Fprint(stdout, question+" [O/n] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "o", "oui", "y", "yes":
			return true
		}
		return false
	}
)

// msvcInstalled : les Build Tools ou Visual Studio avec le compilateur C++, d'après vswhere.
var msvcInstalled = func() bool {
	vswhere := filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft Visual Studio", "Installer", "vswhere.exe")
	if os.Getenv("ProgramFiles(x86)") == "" {
		return false
	}
	out, err := exec.Command(vswhere, "-latest", "-products", "*",
		"-requires", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "-property", "installationPath").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// cmdDeps : apt deps [install] [dossier]. install force le mode installation (apt build-dep).
func cmdDeps(args []string, install, yes bool) int {
	dir := "."
	withCI := false
	for _, a := range args {
		switch strings.ToLower(a) {
		case "install", "installer":
			install = true
		case "--ci":
			withCI = true
		default:
			dir = a
		}
	}
	deps.DockerInstalled = dockerInstalled
	deps.MSVCInstalled = msvcInstalled

	proj, err := deps.Detect(dir)
	if err != nil {
		ui.Errorf("%v", err)
		return 1
	}
	if len(proj.Needs) == 0 && len(proj.Steps) == 0 {
		ui.Info("Aucun outil reconnu dans " + proj.Root + " (package.json, go.mod, pyproject.toml, Makefile…).")
		return 0
	}

	statuses := deps.Check(proj.Needs)
	fmt.Fprintln(stdout, "Projet : "+proj.Root)
	fmt.Fprintln(stdout)
	printStatuses(statuses)
	printAdvice(statuses)

	// Ce que la CI seule demande n'est installé qu'avec --ci.
	wanted := func(s deps.Status) bool {
		return (s.State == deps.Missing || s.State == deps.Outdated) && (withCI || !s.CIOnly())
	}
	todo, ciOnly := 0, 0
	for _, s := range statuses {
		if wanted(s) {
			todo++
		} else if s.State == deps.Missing && s.CIOnly() {
			ciOnly++
		}
	}
	steps := pendingSteps(proj.Steps)

	if !install {
		fmt.Fprintln(stdout)
		if len(steps) > 0 {
			fmt.Fprintln(stdout, "Dépendances du projet :")
			for _, s := range steps {
				fmt.Fprintf(stdout, "  %s : %s\n", s.Dir, s.Label)
			}
			fmt.Fprintln(stdout)
		}
		if ciOnly > 0 {
			fmt.Fprintf(stdout, "○ : demandé seulement par la CI (souvent un job secondaire), installé avec : apt deps install --ci\n\n")
		}
		switch {
		case todo > 0:
			fmt.Fprintf(stdout, "Pour installer ce qui manque (%d outil(s)) : apt deps install\n", todo)
		case len(steps) > 0:
			fmt.Fprintln(stdout, "Tous les outils sont là. Pour installer les dépendances du projet : apt deps install")
		default:
			fmt.Fprintln(stdout, "Tout est prêt.")
		}
		return 0
	}

	// Installation des outils
	if todo > 0 && !wingetAvailable() {
		ui.Errorf("winget est introuvable. Installe « App Installer » depuis le Microsoft Store.")
		return 1
	}
	code := 0
	for _, s := range statuses {
		if !wanted(s) {
			continue
		}
		pkg := deps.PackageFor(s.Need)
		switch s.State {
		case deps.Missing:
			if c := cmdInstall([]string{pkg}, true); c != 0 && code == 0 {
				code = c
			}
		case deps.Outdated:
			// Mise à jour ; si la version installée est une autre « branche » (Python 3.11 au lieu de 3.14),
			// on installe le paquet qui convient.
			c := cmdUpgrade([]string{pkg})
			refreshPath()
			if again := deps.Check([]deps.Need{s.Need}); again[0].State == deps.Outdated {
				c = cmdInstall([]string{pkg}, true)
			}
			if c != 0 && code == 0 {
				code = c
			}
		}
	}
	if todo > 0 {
		refreshPath()
		fmt.Fprintln(stdout)
		ui.Step("Vérification")
		statuses = deps.Check(proj.Needs)
		printStatuses(statuses)
	}

	// Dépendances du projet (npm ci, go mod download…)
	if len(steps) > 0 {
		fmt.Fprintln(stdout)
		var lines []string
		for _, s := range steps {
			lines = append(lines, s.Label+" ("+s.Dir+")")
		}
		if yes || !interactive() || confirm("Installer aussi les dépendances du projet : "+strings.Join(lines, ", ")+" ?") {
			for _, s := range steps {
				ui.Step(s.Label + " dans " + s.Dir)
				for _, args := range s.Args {
					if c := runStep(filepath.Join(proj.Root, filepath.FromSlash(s.Dir)), args); c != 0 {
						ui.Errorf("« %s » a échoué (code %d)", strings.Join(args, " "), c)
						if code == 0 {
							code = c
						}
						break
					}
				}
			}
		}
	}

	missing := 0
	for _, s := range statuses {
		if s.State != deps.OK && !(s.CIOnly() && !withCI) {
			missing++
		}
	}
	if missing == 0 && code == 0 {
		ui.Info("Le projet est prêt. Ouvre un nouveau terminal si une commande n'est pas encore trouvée ailleurs (VS Code, PowerShell).")
	}
	return code
}

// printAdvice : ce qu'apt ne fait pas à ta place, et comment le faire.
func printAdvice(statuses []deps.Status) {
	for _, s := range statuses {
		if s.State != deps.Mismatch {
			continue
		}
		major := deps.Major(s.Pinned)
		fmt.Fprintln(stdout)
		if s.Key == "node" {
			fmt.Fprintf(stdout, "Node.js : le projet est construit avec Node %s (%s), tu as la %s.\n", major, s.PinnedFrom, s.Have)
			fmt.Fprintln(stdout, "apt ne remplace pas ton Node (tes autres projets en dépendent). Pour avoir les deux côte à côte :")
			fmt.Fprintf(stdout, "  apt install nvm, puis dans un nouveau terminal : nvm install %s && nvm use %s\n", s.Pinned, s.Pinned)
			continue
		}
		fmt.Fprintf(stdout, "%s : le projet utilise la version %s (%s), tu as la %s.\n", s.Label, major, s.PinnedFrom, s.Have)
	}
}

func pendingSteps(steps []deps.Step) []deps.Step {
	var out []deps.Step
	for _, s := range steps {
		if !s.Done {
			out = append(out, s)
		}
	}
	return out
}

func printStatuses(statuses []deps.Status) {
	for _, s := range statuses {
		var mark, state string
		switch s.State {
		case deps.OK:
			mark, state = ui.Color("32", "✓"), s.Have
			if state == "" {
				state = "installé"
			}
			if s.Version != "" {
				state += " (≥ " + s.Version + " voulu)"
			}
		case deps.Outdated:
			mark = ui.Color("33", "!")
			state = fmt.Sprintf("%s, trop ancien (≥ %s voulu par %s)", s.Have, s.Version, s.VersionFrom)
		case deps.Missing:
			mark, state = ui.Color("31", "✗"), "manquant → apt install "+deps.PackageFor(s.Need)
			if s.CIOnly() {
				mark = ui.Color("2", "○")
			}
			if s.Version != "" {
				state += " (≥ " + s.Version + " voulu)"
			}
		case deps.NoPackage:
			mark, state = ui.Color("33", "?"), "manquant : "+s.Note
		case deps.Mismatch:
			mark = ui.Color("33", "!")
			state = fmt.Sprintf("%s, mais le projet utilise la version %s (%s)", s.Have, deps.Major(s.Pinned), s.PinnedFrom)
		}
		fmt.Fprintf(stdout, "  %s %-18s %s\n", mark, s.Label, state)
		fmt.Fprintf(stdout, "    %-18s %s\n", "", ui.Color("2", s.SourcesSummary()))
	}
}

// refreshPathFromRegistry ajoute au PATH du processus les dossiers enregistrés par les installeurs
// (Node, Go, Python…), pour utiliser les outils tout de suite, sans rouvrir le terminal.
func refreshPathFromRegistry() {
	cur := os.Getenv("PATH")
	seen := map[string]bool{}
	for _, p := range filepath.SplitList(cur) {
		seen[strings.ToLower(filepath.Clean(p))] = true
	}
	parts := []string{cur}
	for _, d := range platform.RegistryPathDirs() {
		if k := strings.ToLower(filepath.Clean(d)); !seen[k] {
			seen[k] = true
			parts = append(parts, d)
		}
	}
	os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
}
