package shell

// diag : de quoi comprendre un souci sur un PC sans y avoir accès (fenêtre, langue, touches reçues).

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/platform"
)

// langFromWindows : LANG existait déjà au démarrage (défini dans Windows ou par le programme parent).
var langFromWindows bool

func runDiag(s *Shell, args []string) int {
	if len(args) > 0 && (args[0] == "touches" || args[0] == "clavier") {
		return diagKeys(s)
	}
	if len(args) > 0 {
		s.errorf("usage : diag | diag touches")
		return 2
	}
	exe, _ := os.Executable()
	row := func(k, v string) { fmt.Fprintf(s.out, "  %-22s %s\n", k, v) }
	fmt.Fprintln(s.out, "Orvaen Terminal — diagnostic")
	row("version", apt.Version+" ("+runtime.GOOS+"/"+runtime.GOARCH+")")
	row("programme", exe)
	row("fenêtre", hostName())
	r, raw := platform.EnableRawInput()
	if raw {
		r()
	}
	row("saisie touche à touche", map[bool]string{true: "oui", false: "non (saisie par ligne)"}[raw])
	ui, format := platform.LocaleDetails()
	row("langue d'affichage", ui)
	row("format régional", format)
	origin := "le terminal (langue d'affichage de Windows)"
	if langFromWindows {
		origin = "Windows ou le programme qui a ouvert le terminal (gardée telle quelle)"
	}
	row("LANG", os.Getenv("LANG")+"  ← "+origin)
	for _, k := range []string{"LC_ALL", "TERM", "TERM_PROGRAM", "WT_SESSION", "SHELL", "HOME"} {
		if v, ok := os.LookupEnv(k); ok {
			row(k, v)
		}
	}
	fmt.Fprintln(s.out, "\nPour voir ce que la fenêtre envoie quand tu tapes ou colles : diag touches")
	return 0
}

// hostName devine l'application qui affiche le terminal.
func hostName() string {
	switch {
	case os.Getenv("TERM_PROGRAM") == "vscode":
		return "terminal de VS Code"
	case os.Getenv("WT_SESSION") != "":
		return "Windows Terminal"
	case os.Getenv("ConEmuPID") != "":
		return "ConEmu / Cmder"
	case os.Getenv("TERM_PROGRAM") == "mintty" || os.Getenv("MSYSTEM") != "":
		return "mintty (Git Bash)"
	case runtime.GOOS == "windows":
		return "console Windows (conhost) ou autre"
	}
	return os.Getenv("TERM_PROGRAM")
}

// diagKeys affiche les caractères reçus, tels quels, jusqu'à Ctrl+C.
func diagKeys(s *Shell) int {
	restore, ok := platform.EnableRawInput()
	if !ok {
		s.errorf("diag touches : l'entrée n'est pas une console")
		return 1
	}
	defer restore()
	fmt.Fprint(s.out, "\x1b[?2004h")
	defer fmt.Fprint(s.out, "\x1b[?2004l")
	fmt.Fprintln(s.out, "Tape ou colle quelque chose : chaque caractère reçu s'affiche. Ctrl+C pour arrêter.")
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return 0
		}
		chunk := buf[:n]
		var parts []string
		for len(chunk) > 0 {
			r, size := utf8.DecodeRune(chunk)
			chunk = chunk[size:]
			parts = append(parts, describeRune(r))
			if r == 0x03 {
				fmt.Fprintf(s.out, "%s  %s\r\n", time.Now().Format("15:04:05.000"), strings.Join(parts, " "))
				return 0
			}
		}
		fmt.Fprintf(s.out, "%s  %s\r\n", time.Now().Format("15:04:05.000"), strings.Join(parts, " "))
	}
}

func describeRune(r rune) string {
	names := map[rune]string{'\r': "\\r", '\n': "\\n", '\t': "\\t", 0x1b: "ESC", 0x03: "Ctrl+C", 0x7f: "DEL", ' ': "␠"}
	if n, ok := names[r]; ok {
		return n
	}
	if r < 0x20 {
		return fmt.Sprintf("^%c", r+'@')
	}
	return string(r)
}
