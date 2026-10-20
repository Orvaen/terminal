// Package apt implémente les commandes apt au-dessus de winget et de Windows Update.
package apt

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// Version est injectée à la compilation (-ldflags "-X ...apt.Version=v1.0.0").
var Version = "dev"

var stdout io.Writer = os.Stdout

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
	case "autoremove", "clean", "autoclean":
		ui.Info("Rien à faire sous Windows : winget ne garde ni cache ni dépendances orphelines.")
		return 0
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
		return cmdInstall(inv.Packages)
	case "remove", "purge", "uninstall":
		return cmdRemove(inv.Packages)
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
	ui.Step("Mise à jour des sources")
	if code := winget("source", "update"); code != 0 {
		return code
	}
	ui.Step("Logiciels pouvant être mis à jour")
	return winget("upgrade", acceptSource)
}

func cmdUpgrade(pkgs []string) int {
	if len(pkgs) == 0 {
		ui.Step("Mise à jour de tous les logiciels")
		code := winget("upgrade", "--all", acceptSource, acceptPackage)
		if isNotApplicable(code) {
			ui.Info("Tout est déjà à jour.")
			return 0
		}
		return code
	}
	return forEach(pkgs, "Mise à jour de", func(p string) int {
		code := winget(append(append([]string{"upgrade"}, target(p)...), acceptSource, acceptPackage)...)
		if isNotApplicable(code) {
			ui.Info(p + " est déjà à jour.")
			return 0
		}
		return code
	})
}

func cmdFullUpgrade() int {
	first := cmdUpgrade(nil)

	ui.Step("Mises à jour Windows")
	var wu int
	if platform.IsElevated() {
		wu = windowsUpdate(false)
	} else {
		ui.Info("Ouverture d'une fenêtre administrateur pour Windows Update…")
		wu = platform.RunSelfElevated("windows-update", "--pause")
	}
	if first != 0 {
		return first
	}
	return wu
}

func cmdInstall(pkgs []string) int {
	if len(pkgs) == 0 {
		ui.Errorf("précise au moins un paquet : apt install <nom>")
		return 1
	}
	return forEach(pkgs, "Installation de", func(p string) int {
		return winget(append(append([]string{"install"}, target(p)...), acceptSource, acceptPackage)...)
	})
}

func cmdRemove(pkgs []string) int {
	if len(pkgs) == 0 {
		ui.Errorf("précise au moins un paquet : apt remove <nom>")
		return 1
	}
	return forEach(pkgs, "Désinstallation de", func(p string) int {
		return winget(append(append([]string{"uninstall"}, target(p)...), acceptSource)...)
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
	return winget(append(append([]string{"show"}, target(pkgs[0])...), acceptSource)...)
}

func cmdList(inv Invocation) int {
	if inv.Upgradable {
		return winget("upgrade", acceptSource)
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
  full-upgrade         upgrade + mises à jour Windows (alias : dist-upgrade)
  install <paquet...>  Installe un ou plusieurs logiciels
  remove <paquet...>   Désinstalle (alias : purge, uninstall)
  search <mot>         Cherche un logiciel
  show <paquet>        Affiche les détails d'un logiciel
  list [--upgradable]  Logiciels installés (ou ceux qui ont une mise à jour)
  windows-update       Installe uniquement les mises à jour Windows (admin)
  version              Affiche la version

Noms courts reconnus : %s
Les autres noms sont transmis à winget (ex : apt install Spotify.Spotify).
`, Version, strings.Join(AliasNames(), ", "))
}
