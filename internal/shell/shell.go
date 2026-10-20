// Package shell implémente terminal.exe : un shell façon Linux pour Windows,
// avec apt et sudo intégrés et PowerShell en renfort pour le reste.
package shell

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// AppName est le nom affiché dans le titre de la fenêtre et la bannière.
const AppName = "Orvaen Terminal"

// Shell garde l'état d'une session.
type Shell struct {
	in      *bufio.Reader
	out     io.Writer
	errOut  io.Writer
	color   bool
	width   func() int
	admin   bool
	last    int    // code de sortie de la dernière commande
	prevDir string // pour « cd - »
	exiting bool

	edit        *editor         // saisie touche par touche (nil : saisie par ligne)
	installed   func() []string // logiciels installés, pour la complétion de « apt remove »
	historyPath string

	lastInterrupt atomic.Int64
}

// New crée un shell branché sur la console.
func New() *Shell {
	s := &Shell{
		in:     bufio.NewReader(os.Stdin),
		out:    os.Stdout,
		errOut: os.Stderr,
		color:  ui.ColorEnabled(),
		width:  platform.TerminalWidth,
		admin:  platform.IsElevated(),
	}
	// Dans le shell, l'exécutable est terminal.exe : on relance donc « terminal -c "apt ..." ».
	apt.Elevate = func(args ...string) int {
		return platform.RunSelfElevated("-c", "apt "+joinArgs(args))
	}
	s.installed = func() []string { return apt.InstalledNames(4 * time.Second) }
	if restore, ok := platform.EnableRawInput(); ok {
		restore()
		s.edit = &editor{
			in:       s.in,
			out:      s.out,
			width:    s.width,
			complete: s.complete,
			onChange: prefetchForLine,
		}
		s.historyPath = historyFile()
		s.edit.history = loadHistory(s.historyPath)
	}
	return s
}

func (s *Shell) paint(code, text string) string {
	if !s.color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s *Shell) errorf(format string, a ...any) {
	fmt.Fprintf(s.errOut, format+"\n", a...)
}

// Interactive lance la boucle « prompt → commande » jusqu'à exit ou Ctrl+Z.
func (s *Shell) Interactive() int {
	if s.color {
		fmt.Fprint(s.out, "\x1b]0;"+AppName+"\x07")
	}
	fmt.Fprintf(s.out, "%s %s — tape %s pour voir les commandes.\n", AppName, apt.Version, s.paint("1", "help"))

	// Ctrl+C interrompt la commande en cours, pas le shell.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		for range sig {
			s.lastInterrupt.Store(time.Now().UnixNano())
		}
	}()

	for !s.exiting {
		fmt.Fprint(s.out, s.promptHeader())
		if s.edit != nil {
			if restore, ok := platform.EnableRawInput(); ok {
				line, err := s.edit.readLine(s.promptTail())
				restore()
				switch {
				case errors.Is(err, errInterrupted):
					s.last = 130
					continue
				case errors.Is(err, io.EOF):
					return s.last
				}
				s.remember(line)
				s.run(line)
				continue
			}
		}
		tail, _ := s.promptTail()
		fmt.Fprint(s.out, tail)
		line, err := s.in.ReadString('\n')
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			if time.Since(time.Unix(0, s.lastInterrupt.Load())) < 2*time.Second {
				fmt.Fprintln(s.out)
				s.last = 130
				continue
			}
			if strings.TrimSpace(line) == "" {
				fmt.Fprintln(s.out)
				return s.last
			}
		}
		s.run(strings.TrimRight(line, "\r\n"))
	}
	return s.last
}

// run exécute une ligne tapée ; une commande apt peut avoir changé les logiciels installés.
func (s *Shell) run(line string) {
	s.RunLine(line)
	if strings.Contains(strings.ToLower(line), "apt") {
		apt.InvalidateInstalled()
	}
}

// promptHeader : la ligne d'information au-dessus de la saisie (utilisateur, PC, dossier).
func (s *Shell) promptHeader() string {
	full := s.prompt()
	return full[:strings.LastIndex(full, "\n")+1]
}

// promptTail : le « $ » de la ligne de saisie et sa largeur visible.
func (s *Shell) promptTail() (string, int) {
	full := s.prompt()
	return full[strings.LastIndex(full, "\n")+1:], 2
}

func (s *Shell) prompt() string {
	cwd, _ := os.Getwd()
	user := firstNonEmpty(os.Getenv("USERNAME"), os.Getenv("USER"), "user")
	host, _ := os.Hostname()

	label, labelColor, sym := "WIN", "35", "$"
	if s.admin {
		label, labelColor, sym = "ADMIN", "1;31", "#"
	}
	symColor := "1;32"
	if s.last != 0 {
		symColor = "1;31"
	}
	return "\n" + s.paint("32", user+"@"+host) + " " + s.paint(labelColor, label) + " " +
		s.paint("33", displayPath(cwd, homeDir())) + "\n" + s.paint(symColor, sym) + " "
}

// RunLine exécute une ligne complète (avec ; && ||) et renvoie le code de la dernière commande.
func (s *Shell) RunLine(line string) int {
	if strings.TrimSpace(line) == "" {
		return s.last
	}
	segs, err := splitChain(line)
	if err != nil {
		s.errorf("terminal: %v", err)
		s.last = 2
		return s.last
	}
	for i, seg := range segs {
		if i > 0 {
			prev := segs[i-1].op
			if (prev == opAnd && s.last != 0) || (prev == opOr && s.last == 0) {
				continue
			}
		}
		s.last = s.runSegment(seg)
		if s.exiting {
			break
		}
	}
	return s.last
}

func (s *Shell) runSegment(seg segment) int {
	// Tubes, redirections, variables PowerShell : PowerShell sait faire.
	if seg.special {
		return s.powershell(seg.text)
	}
	args, err := tokenize(seg.text, homeDir())
	if err != nil {
		s.errorf("terminal: %v", err)
		return 2
	}
	if len(args) == 0 {
		return s.last
	}
	if b, ok := builtins[strings.ToLower(args[0])]; ok {
		return b.run(s, args[1:])
	}
	if path, err := exec.LookPath(args[0]); err == nil {
		return s.execProgram(path, args[1:])
	}
	// Ni commande intégrée ni programme : cmdlet, alias ou fonction PowerShell (dir, Get-Process...).
	return s.powershell(seg.text)
}

func (s *Shell) execProgram(path string, args []string) int {
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, s.out, s.errOut
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		s.errorf("terminal: %v", err)
		return 126
	}
	return 0
}

func (s *Shell) powershell(text string) int {
	ps := ""
	for _, name := range []string{"pwsh", "powershell"} {
		if p, err := exec.LookPath(name); err == nil {
			ps = p
			break
		}
	}
	if ps == "" {
		if runtime.GOOS == "windows" {
			s.errorf("terminal: PowerShell est introuvable")
		} else {
			first := strings.Fields(text)
			if len(first) > 0 {
				s.errorf("terminal: %s : commande introuvable", first[0])
			}
		}
		return 127
	}
	return s.execProgram(ps, []string{"-NoLogo", "-NoProfile", "-Command", text})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
