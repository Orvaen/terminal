package shell

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/docker"
	"github.com/Orvaen/terminal/internal/req"
	"github.com/Orvaen/terminal/internal/scripts"
)

// helpAliases : les autres noms des commandes intégrées.
var helpAliases = map[string]string{"apt-get": "apt", "docker-compose": "docker", "ll": "ls"}

// detailedHelp : les commandes qui ont leur propre aide détaillée.
var detailedHelp = map[string]func(s *Shell) int{
	"prompt": func(s *Shell) int {
		var names []string
		for _, t := range Themes {
			names = append(names, t.Name)
		}
		fmt.Fprintf(s.out, "prompt — l'apparence du prompt\n\n"+
			"  prompt                 aperçu de tous les styles, dans ton dossier actuel\n"+
			"  prompt <style>         choisit un style : %s\n"+
			"  prompt perso \"…\"       ton propre format (voir ci-dessous)\n\n", strings.Join(names, ", "))
		fmt.Fprint(s.out, promptHelp)
		return 0
	},
	"path":      func(s *Shell) int { fmt.Fprint(s.out, pathHelp); return 0 },
	"variables": func(s *Shell) int { fmt.Fprint(s.out, variablesHelp); return 0 },
	"export":    func(s *Shell) int { fmt.Fprint(s.out, variablesHelp); return 0 },
	"set":       func(s *Shell) int { fmt.Fprint(s.out, variablesHelp); return 0 },
	"run":       func(s *Shell) int { fmt.Fprint(s.out, scripts.Usage); return 0 },
	"apt":       func(s *Shell) int { return apt.Run([]string{"help"}) },
	"req":       func(s *Shell) int { return req.Run([]string{"aide"}, s.out) },
	"docker": func(s *Shell) int {
		fmt.Fprintln(s.out, "docker — le Docker léger (Docker Engine dans WSL2, sans Docker Desktop)")
		fmt.Fprintln(s.out, "\nToutes les commandes de Docker marchent : docker run, docker compose up -d, docker ps…")
		fmt.Fprint(s.out, "Installation : apt install docker\n\n")
		return docker.Run([]string{"engine", "help"})
	},
	"sudo": func(s *Shell) int {
		fmt.Fprint(s.out, `sudo <commande> — lance une commande en administrateur

  sudo apt full-upgrade     mises à jour Windows (administrateur requis)
  sudo path nettoyer        nettoie aussi le PATH système

apt install/upgrade et docker n'en ont pas besoin : winget demande lui-même la confirmation
de Windows (UAC) quand un installeur le demande. Avec le sudo de Windows 11 activé
(sudo config --enable normal), la commande s'exécute dans la même fenêtre.
`)
		return 0
	},
}

// helpFor affiche l'aide d'une commande intégrée, ou dit où trouver celle d'un programme.
func helpFor(s *Shell, name string) int {
	key := strings.ToLower(name)
	if real, ok := helpAliases[key]; ok {
		fmt.Fprintf(s.out, "%s est un autre nom de %s.\n\n", key, real)
		key = real
	}
	if f, ok := detailedHelp[key]; ok {
		return f(s)
	}
	if b, ok := builtins[key]; ok && b.usage != "" {
		fmt.Fprintf(s.out, "%s\n    %s\n", b.usage, b.help)
		if _, _, ok := coreutilsTool(key); ok {
			fmt.Fprintf(s.out, "\nPour les options qu'elle ne connaît pas, la commande passe la main à Coreutils : %s --help\n", key)
		}
		return 0
	}
	if _, err := exec.LookPath(name); err == nil {
		fmt.Fprintf(s.out, "%s n'est pas une commande intégrée du terminal. Son aide : %s --help\n", name, name)
		return 0
	}
	s.errorf("help: %s : commande inconnue (help pour la liste)", name)
	return 1
}

const variablesHelp = `Variables, comme dans bash

  NOM=valeur              variable du terminal (pas transmise aux programmes)
  export NOM=valeur       variable transmise aux programmes lancés ensuite
  export NOM | export -n NOM   transmettre une variable existante | ne plus la transmettre
  NOM=valeur commande     seulement pour cette commande (NODE_ENV=production node app.js)
  unset NOM               supprimer
  set | env | printenv    tout lister | l'environnement | une variable (printenv PATH)

  $NOM  ${NOM}            la valeur ; entre "…" elle reste un seul argument, entre '…' rien n'est remplacé
  ${NOM:-défaut}          défaut si vide          ${NOM:=défaut}   et l'enregistre
  ${NOM:+autre}           autre si non vide       ${NOM:?message}  erreur si vide
  ${#NOM}                 longueur                ${NOM:2:3}       extrait (début, longueur)
  ${NOM#*/} ${NOM##*/}    retire un début         ${NOM%.*} ${NOM%%.*}   retire une fin
  ${NOM/a/b} ${NOM//a/b}  remplace la première, toutes      ${NOM^^} ${NOM,,}  MAJ, min
  $(commande) ` + "`commande`" + `  la sortie d'une commande : echo "branche : $(git branch --show-current)"
  $((2 * (3 + 4)))        un calcul entier
  $env:NOM                la syntaxe de PowerShell marche aussi

Paramètres spéciaux : $? (code de la dernière commande) $$ (n° du terminal) $_ (dernier argument)
  $0 $# $@ $* $- $!

Définies au démarrage, comme sous Linux : HOME USER LOGNAME PWD OLDPWD SHLVL TERM COLORTERM LANG
  SHELL PATH ; HOSTNAME HOSTTYPE OSTYPE MACHTYPE UID EUID PPID IFS PS1 PS2 PS4
  HISTFILE HISTSIZE HISTFILESIZE HISTCONTROL ; calculées à chaque lecture : RANDOM SRANDOM SECONDS
  EPOCHSECONDS EPOCHREALTIME LINENO COLUMNS LINES BASHPID.

PS1 change le prompt pour la session, avec les séquences de bash : PS1='\u@\h:\w\$ '
(unset PS1 revient au style choisi avec la commande prompt, qui lui est gardé).
`
