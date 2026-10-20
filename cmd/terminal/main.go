// Command terminal ouvre un shell façon Linux pour Windows (comme Git Bash),
// avec apt et sudo intégrés.
//
//	terminal              session interactive
//	terminal -c "ligne"   exécute une ligne puis quitte
//	terminal --setup-vscode  ajoute le profil au terminal intégré de VS Code
//	terminal --update     met à jour Orvaen Terminal (comme « apt upgrade terminal »)
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/setup"
	"github.com/Orvaen/terminal/internal/shell"
	"github.com/Orvaen/terminal/internal/update"
)

func main() {
	// Console en UTF-8 : les accents de winget, git, node, Coreutils… s'affichent correctement.
	// Les pages de code d'origine sont rétablies en quittant (utile avec terminal -c depuis PowerShell).
	restore := platform.EnableUTF8Console()
	code := run()
	restore()
	os.Exit(code)
}

func run() int {
	var (
		line    string
		hasLine bool
		pause   bool
	)
	// Supprime les anciennes versions laissées par la dernière mise à jour.
	if dir, err := update.InstallDir(); err == nil {
		update.Cleanup(dir)
	}

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pause":
			pause = true
		case "-c":
			var rest []string
			for _, a := range args[i+1:] {
				if a == "--pause" {
					pause = true
					continue
				}
				rest = append(rest, a)
			}
			line, hasLine = strings.Join(rest, " "), true
			i = len(args)
		case "--setup-vscode":
			exe, err := os.Executable()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return setup.VSCode(exe, os.Stdout)
		case "--update":
			return update.Run(apt.Version, false)
		case "--version", "-v":
			fmt.Printf("%s %s\n", shell.AppName, apt.Version)
			return 0
		case "--help", "-h":
			fmt.Println("Usage : terminal [-c \"commande\"] [--update] [--version]")
			return 0
		}
	}

	sh := shell.New()
	var code int
	if hasLine {
		code = sh.RunLine(line)
	} else {
		code = sh.Interactive()
	}

	if pause {
		fmt.Print("\nAppuie sur Entrée pour fermer cette fenêtre.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return code
}
