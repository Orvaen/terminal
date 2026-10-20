// Package apt implémente les commandes apt au-dessus de winget et de Windows Update.
package apt

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Orvaen/terminal/internal/docker"
	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
	"github.com/Orvaen/terminal/internal/update"
)

// Version est injectée à la compilation (-ldflags "-X ...apt.Version=v1.0.0").
var Version = "dev"

var stdout io.Writer = os.Stdout

// Elevate relance « apt <args> » en administrateur. Le shell terminal.exe la remplace,
// car son exécutable n'est pas apt.exe.
var Elevate = func(args ...string) int { return platform.RunSelfElevated(args...) }

// « docker » ne passe pas par winget : c'est le Docker léger (Alpine + Docker Engine dans WSL2).
// Points d'injection, remplacés dans les tests.
var (
	dockerInstall   = docker.Install
	dockerUninstall = docker.Uninstall
	dockerUpgrade   = docker.Upgrade
	dockerInstalled = func() bool { return docker.Installed() }
)

// Mise à jour d'Orvaen Terminal lui-même, depuis les releases GitHub. Remplacées dans les tests.
var (
	selfUpdate = func(force bool) int { return update.Run(Version, force) }
	selfCheck  = func() (string, bool) { return update.Check(Version, 5*time.Second) }
)

// isSelf : noms qui désignent Orvaen Terminal dans « apt upgrade <nom> ».
func isSelf(name string) bool {
	switch strings.ToLower(name) {
	case "terminal", "orvaen-terminal", "orvaen":
		return true
	}
	return false
}

func onlySelf(pkgs []string) bool {
	for _, p := range pkgs {
		if !isSelf(p) {
			return false
		}
	}
	return len(pkgs) > 0
}

func isDocker(name string) bool {
	switch strings.ToLower(name) {
	case "docker", "docker-engine", "docker.io", "docker-ce", "docker-compose", "docker-compose-plugin":
		return true
	}
	return false
}

// Run exécute apt avec les arguments donnés et renvoie le code de sortie.
func Run(args []string) int {
	inv := Parse(args)

	switch inv.Command {
	case "", "help", "-h", "--help":
		printHelp(stdout)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "apt pour Windows %s\n", Version)
		return 0
	case "windows-update":
		return windowsUpdate(inv.Pause)
	case "wsl-setup": // usage interne : activation de WSL2 dans une fenêtre admin
		code := docker.SetupWSL()
		if inv.Pause {
			waitForEnter()
		}
		return code
	case "autoremove", "clean", "autoclean":
		ui.Info("Rien à faire sous Windows : winget ne garde ni cache ni dépendances orphelines.")
		return 0
	}

	// La mise à jour du terminal ne dépend pas de winget.
	if inv.Command == "upgrade" && onlySelf(inv.Packages) {
		return selfUpdate(false)
	}
	if inv.Command == "alias" {
		return cmdAlias(inv.Packages)
	}
	switch inv.Command {
	case "optimiser", "optimise", "optimize", "optimisation":
		return cmdOptimise(inv.Packages, inv.Yes)
	}
	if inv.Command == "deps" || inv.Command == "build-dep" {
		return cmdDeps(inv.Packages, inv.Command == "build-dep", inv.Yes)
	}

	if !wingetAvailable() {
		ui.Errorf("winget est introuvable. Installe « App Installer » depuis le Microsoft Store.")
		return 1
	}

	switch inv.Command {
	case "update":
		return cmdUpdate()
	case "upgrade":
		return cmdUpgrade(inv.Packages)
	case "full-upgrade", "dist-upgrade":
		return cmdFullUpgrade()
	case "install":
		return cmdInstall(inv.Packages, inv.Yes)
	case "remove", "purge", "uninstall":
		return cmdRemove(inv.Packages, inv.Yes)
	case "search":
		return cmdSearch(inv.Packages)
	case "show":
		return cmdShow(inv.Packages)
	case "list":
		return cmdList(inv)
	default:
		ui.Errorf("commande inconnue : %s", inv.Command)
		printHelp(os.Stderr)
		return 1
	}
}

func cmdUpdate() int {
	if code := updateSources(); code != 0 {
		return code
	}
	fmt.Fprintln(stdout)
	return listUpgrades()
}

func cmdUpgrade(pkgs []string) int {
	if len(pkgs) == 0 {
		ui.Step("Mise à jour de tous les logiciels")
		code := winget("upgrade", "--all", acceptSource, acceptPackage)
		if isNotApplicable(code) {
			ui.Info("Tout est déjà à jour.")
			code = 0
		}
		// winget peut avoir quelques jours de retard : on complète avec les sources officielles
		if nc, handled := upgradeNode(true); handled && code == 0 {
			code = nc
		}
		if dockerInstalled() {
			if dc := dockerUpgrade(); code == 0 {
				code = dc
			}
		}
		// Composer, Maven, Gradle, Poetry installés par apt
		if ec := upgradeExtTools(); code == 0 {
			code = ec
		}
		// Une compilation locale (build.ps1) n'est pas remplacée sans le demander : apt upgrade terminal.
		if !update.IsDev(Version) {
			if sc := selfUpdate(false); code == 0 {
				code = sc
			}
		}
		return code
	}
	return forEach(pkgs, "Mise à jour de", upgradeOne)
}

// upgradeOne met à jour un logiciel désigné par son nom.
func upgradeOne(p string) int {
	if isDocker(p) {
		return dockerUpgrade()
	}
	if isSelf(p) {
		return selfUpdate(false)
	}
	if t, ok := findExtTool(p); ok {
		return installExt(t, true)
	}
	if c, ok := findComponent(p); ok {
		return upgradeComponent(p, c, upgradeOne)
	}
	if isNode(p) {
		if code, handled := upgradeNode(false); handled {
			return code
		}
	}
	targets := [][]string{target(p)}
	if ids := installedTargets(p); ids != nil {
		targets = nil
		for _, id := range ids {
			targets = append(targets, exactTarget(id))
		}
	}
	result := 0
	for _, t := range targets {
		code := winget(append(append([]string{"upgrade"}, t...), acceptSource, acceptPackage)...)
		if isNotApplicable(code) {
			ui.Info(p + " est déjà à jour.")
			code = 0
		}
		if code != 0 && result == 0 {
			result = code
		}
	}
	return result
}

func cmdFullUpgrade() int {
	first := cmdUpgrade(nil)

	ui.Step("Mises à jour Windows")
	var wu int
	if platform.IsElevated() {
		wu = windowsUpdate(false)
	} else {
		ui.Info("Ouverture d'une fenêtre administrateur pour Windows Update…")
		wu = Elevate("windows-update", "--pause")
	}
	if first != 0 {
		return first
	}
	return wu
}

func cmdInstall(pkgs []string, yes bool) int {
	if len(pkgs) == 0 {
		ui.Errorf("précise au moins un paquet : apt install <nom>")
		return 1
	}
	return forEach(pkgs, "Installation de", func(p string) int {
		if isDocker(p) {
			return dockerInstall(Elevate)
		}
		if c, ok := findComponent(p); ok {
			ui.Info(fmt.Sprintf("%s est fourni avec %s : installation de %s.", p, c.Label, c.Label))
			p = c.Provider
		}
		if t, ok := findExtTool(p); ok {
			return installExt(t, false)
		}
		if buildEssentialNames[strings.ToLower(p)] {
			return installBuildEssential()
		}
		if spectreNames[strings.ToLower(p)] {
			return installSpectre()
		}
		if note, ok := windowsNotes[strings.ToLower(p)]; ok {
			if _, mine := userAlias(p); !mine {
				ui.Info("Rien à installer : " + note)
				return 0
			}
		}
		res, ok := resolvePackage(p, yes)
		if !ok {
			return 100 // comme apt : paquet introuvable
		}
		code := installTarget(p, res)
		// Déjà installé : winget tente une mise à jour et répond « aucune mise à jour applicable ».
		if isNotApplicable(code) {
			ui.Info(p + " est déjà installé et à jour.")
			return 0
		}
		// Un identifiant de la table a pu changer chez winget : on le retrouve autrement.
		if isNoPackageFound(code) && res.curated {
			ui.Warn(res.id + " est introuvable dans winget, recherche d'un équivalent…")
			if res, ok = searchResolution(p, yes); !ok {
				return 100
			}
			code = installTarget(p, res)
		}
		return code
	})
}

func installTarget(name string, res resolution) int {
	t := []string{name}
	if res.id != "" {
		t = []string{"--id", res.id, "--exact"}
	}
	return winget(append(append([]string{"install"}, t...), acceptSource, acceptPackage)...)
}

func cmdRemove(pkgs []string, yes bool) int {
	if len(pkgs) == 0 {
		ui.Errorf("précise au moins un paquet : apt remove <nom>")
		return 1
	}
	return forEach(pkgs, "Désinstallation de", func(p string) int {
		if isDocker(p) {
			return dockerUninstall(yes)
		}
		if t, ok := findExtTool(p); ok {
			return removeExt(t)
		}
		if c, ok := findComponent(p); ok {
			return removeComponent(p, c)
		}
		ids := installedTargets(p)
		switch {
		case ids == nil:
			return winget(append(append([]string{"uninstall"}, target(p)...), acceptSource)...)
		case len(ids) > 1:
			ui.Errorf("plusieurs versions de %s sont installées : %s", p, strings.Join(ids, ", "))
			ui.Info("Précise laquelle retirer : apt remove " + ids[0])
			return 1
		}
		code := winget(append(append([]string{"uninstall"}, exactTarget(ids[0])...), acceptSource)...)
		if isNoPackageFound(code) {
			ui.Info(p + " n'est pas installé (la liste : apt list).")
		}
		return code
	})
}

func cmdSearch(terms []string) int {
	if len(terms) == 0 {
		ui.Errorf("précise un mot-clé : apt search <mot>")
		return 1
	}
	return winget("search", strings.Join(terms, " "), acceptSource)
}

func cmdShow(pkgs []string) int {
	if len(pkgs) == 0 {
		ui.Errorf("précise un paquet : apt show <nom>")
		return 1
	}
	res, ok := resolvePackage(pkgs[0], false)
	if !ok {
		return 100
	}
	t := []string{pkgs[0]}
	if res.id != "" {
		t = []string{"--id", res.id, "--exact"}
	}
	return winget(append(append([]string{"show"}, t...), acceptSource)...)
}

func cmdList(inv Invocation) int {
	if inv.Upgradable {
		return listUpgrades()
	}
	args := []string{"list"}
	if len(inv.Packages) > 0 {
		args = append(args, strings.Join(inv.Packages, " "))
	}
	return winget(append(args, acceptSource)...)
}

// forEach applique fn à chaque paquet, continue en cas d'échec et renvoie le premier code d'erreur.
func forEach(pkgs []string, verb string, fn func(string) int) int {
	result := 0
	var failed []string
	for _, p := range pkgs {
		ui.Step(verb + " " + p)
		if code := fn(p); code != 0 {
			failed = append(failed, p)
			if result == 0 {
				result = code
			}
		}
	}
	if len(failed) > 0 && len(pkgs) > 1 {
		ui.Errorf("échec pour : %s", strings.Join(failed, ", "))
	}
	return result
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, `apt pour Windows %s — les commandes apt, branchées sur winget

Usage : apt <commande> [paquets...] [-y]

Commandes :
  update               Rafraîchit les sources et liste les mises à jour disponibles
  upgrade [paquet...]  Met à jour tous les logiciels (ou seulement ceux indiqués)
  upgrade terminal     Met à jour Orvaen Terminal, sans fermer ce qui tourne
  full-upgrade         upgrade + mises à jour Windows (alias : dist-upgrade)
  install <paquet...>  Installe un ou plusieurs logiciels (noms Linux acceptés : code, fd-find…)
  remove <paquet...>   Désinstalle (alias : purge, uninstall)
  search <mot>         Cherche un logiciel
  show <paquet>        Affiche les détails d'un logiciel
  list [--upgradable]  Logiciels installés (ou ceux qui ont une mise à jour)
  windows-update       Installe uniquement les mises à jour Windows (admin)
  deps [install]       Outils dont le projet a besoin, et leur installation (= build-dep)
  optimiser [etat|annuler]  Accélère le dev : Defender, Docker, compilation (demande avant)
  alias [nom id]       Tes noms : apt alias code Microsoft.VisualStudioCode (-r pour oublier)
  version              Affiche la version

Docker léger : « apt install docker » installe Docker Engine dans une mini machine
Alpine Linux (WSL2), sans Docker Desktop. Moteur : « docker engine status|stop ».

Noms : %d noms Linux (Debian, Ubuntu, Arch, Homebrew) sont connus. Pour un autre nom,
apt cherche dans winget le logiciel qui porte ce nom court ou qui fournit cette commande,
et te fait choisir si plusieurs correspondent (ton choix est retenu).
Un identifiant winget marche toujours : apt install Spotify.Spotify
`, Version, len(aliases))
}
