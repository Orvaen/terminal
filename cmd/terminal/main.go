// Command terminal ouvre un shell façon Linux pour Windows (comme Git Bash),
// avec apt et sudo intégrés.
//
//	terminal              session interactive
//	terminal -c "ligne"   exécute une ligne puis quitte
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/shell"
)

func main() {
	var (
		line    string
		hasLine bool
		pause   bool
	)
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
		case "--version", "-v":
			fmt.Printf("%s %s\n", shell.AppName, apt.Version)
			return
		case "--help", "-h":
			fmt.Println("Usage : terminal [-c \"commande\"]")
			return
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
	os.Exit(code)
}
