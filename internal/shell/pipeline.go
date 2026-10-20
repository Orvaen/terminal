package shell

// Tubes et redirections gérés par le terminal lui-même : cat f | grep x | sort > tri.txt
//
// Avant, toute ligne avec | ou > partait dans PowerShell, où cat, ls, sort… sont des alias
// PowerShell (Get-Content, Get-ChildItem, Sort-Object) et pas les commandes Unix. Le terminal relie
// maintenant lui-même les programmes par de vrais tubes, comme bash. PowerShell reste le repli
// pour ce que le terminal ne gère pas : variables ($env:PATH), sous-commandes (`…`), cmdlets
// (Get-Process | Select -First 3) ou commandes de cmd (dir).

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// stage : une commande d'un tube, avec ses redirections.
type stage struct {
	text      string // la commande sans ses redirections
	in        string // < fichier
	out       string // > ou >> fichier
	outAppend bool
	err       string // 2> ou 2>> fichier
	errAppend bool
	errToOut  bool // 2>&1
	bothFile  bool // &> fichier : sortie et erreurs dans le même fichier
}

var errNeedsPowerShell = errors.New("PowerShell")

// parsePipeline découpe une ligne en commandes reliées par |, avec leurs redirections.
// Renvoie errNeedsPowerShell pour ce que le terminal ne sait pas faire lui-même.
func parsePipeline(line string) ([]stage, error) {
	var (
		stages []stage
		cur    stage
		b      strings.Builder
		quote  rune
	)
	runes := []rune(line)
	n := len(runes)

	// word lit la cible d'une redirection (guillemets acceptés).
	word := func(i int) (string, int, error) {
		for i < n && (runes[i] == ' ' || runes[i] == '\t') {
			i++
		}
		start, q := i, rune(0)
		for i < n {
			r := runes[i]
			if q != 0 {
				if r == q {
					q = 0
				}
				i++
				continue
			}
			if r == '\'' || r == '"' {
				q = r
				i++
				continue
			}
			if r == ' ' || r == '\t' || r == '|' || r == '<' || r == '>' || r == '&' {
				break
			}
			if r == '$' || r == '`' {
				return "", i, errNeedsPowerShell
			}
			i++
		}
		if q != 0 {
			return "", i, errUnclosedQuote
		}
		toks, err := tokenize(string(runes[start:i]), homeDir())
		if err != nil {
			return "", i, err
		}
		if len(toks) != 1 || toks[0] == "" {
			return "", i, errors.New("erreur de syntaxe : fichier manquant après une redirection")
		}
		return convertPath(toks[0], runtime.GOOS), i, nil
	}

	// fdBefore retire un « 1 » ou « 2 » collé devant > (2>fichier) et renvoie le descripteur.
	fdBefore := func() int {
		s := b.String()
		if len(s) == 0 {
			return 1
		}
		last := s[len(s)-1]
		if (last == '1' || last == '2') && (len(s) == 1 || s[len(s)-2] == ' ' || s[len(s)-2] == '\t') {
			b.Reset()
			b.WriteString(s[:len(s)-1])
			return int(last - '0')
		}
		return 1
	}

	endStage := func() error {
		cur.text = strings.TrimSpace(b.String())
		b.Reset()
		if cur.text == "" {
			return errors.New("erreur de syntaxe près de « | »")
		}
		stages = append(stages, cur)
		cur = stage{}
		return nil
	}

	for i := 0; i < n; i++ {
		r := runes[i]
		if quote != 0 {
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			b.WriteRune(r)
		case '$', '`':
			return nil, errNeedsPowerShell
		case '|':
			if err := endStage(); err != nil {
				return nil, err
			}
		case '<':
			target, next, err := word(i + 1)
			if err != nil {
				return nil, err
			}
			cur.in, i = target, next-1
			b.WriteRune(' ')
		case '&':
			if i+1 < n && runes[i+1] == '>' { // &> fichier
				target, next, err := word(i + 2)
				if err != nil {
					return nil, err
				}
				cur.out, cur.bothFile, cur.outAppend, i = target, true, false, next-1
				b.WriteRune(' ')
				continue
			}
			return nil, errNeedsPowerShell // & seul : processus en arrière-plan, non géré
		case '>':
			fd := fdBefore()
			appendMode := i+1 < n && runes[i+1] == '>'
			j := i + 1
			if appendMode {
				j++
			}
			if j+1 < n && runes[j] == '&' { // 2>&1 ou 1>&2
				if fd == 2 && runes[j+1] == '1' {
					cur.errToOut, i = true, j+1
					b.WriteRune(' ')
					continue
				}
				return nil, errNeedsPowerShell
			}
			target, next, err := word(j)
			if err != nil {
				return nil, err
			}
			if fd == 2 {
				cur.err, cur.errAppend = target, appendMode
			} else {
				cur.out, cur.outAppend = target, appendMode
			}
			i = next - 1
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, errUnclosedQuote
	}
	if err := endStage(); err != nil {
		return nil, err
	}
	return stages, nil
}

// resolved : ce qui exécute une commande.
type resolved struct {
	builtin *builtin
	path    string   // programme
	pre     []string // arguments ajoutés avant (binaire multi-appel de Coreutils)
	args    []string // arguments (sans le nom de la commande)
}

// resolve trouve la commande à lancer.
// ok=false : ni commande intégrée ni programme (cmdlet, alias ou commande cmd → PowerShell).
func (s *Shell) resolve(args []string) (resolved, bool) {
	name := strings.ToLower(args[0])
	if b, ok := builtins[name]; ok {
		if needsCoreutils(name, args[1:]) {
			if path, pre, ok := coreutilsTool(name); ok {
				rest := args[1:]
				if name != "echo" && name != "env" {
					rest = convertArgs(rest)
				}
				return resolved{path: path, pre: pre, args: rest}, true
			}
		}
		return resolved{builtin: &b, args: args[1:]}, true
	}
	if path, err := exec.LookPath(args[0]); err == nil {
		return resolved{path: path, args: args[1:]}, true
	}
	return resolved{}, false
}

// convertArgs traduit les chemins façon Git Bash (/c/Users/…) pour les programmes Windows.
func convertArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			out[i] = a
		} else {
			out[i] = convertPath(a, runtime.GOOS)
		}
	}
	return out
}

// guard refuse ce que les commandes intégrées refusent aussi quand Coreutils prend le relais :
// supprimer la racine d'un disque ou le dossier personnel.
func (r resolved) guard(name string) error {
	if r.builtin != nil || !strings.EqualFold(name, "rm") {
		return nil
	}
	noMore := false
	for _, a := range r.args {
		if !noMore && a == "--" {
			noMore = true
			continue
		}
		if !noMore && strings.HasPrefix(a, "-") {
			continue
		}
		if isProtected(a) {
			return fmt.Errorf("rm: refus de supprimer '%s' (racine du disque ou dossier personnel)", a)
		}
	}
	return nil
}

// runSpecial exécute une ligne avec tubes ou redirections, ou la confie à PowerShell.
func (s *Shell) runSpecial(text string) int {
	stages, err := parsePipeline(text)
	if errors.Is(err, errNeedsPowerShell) {
		return s.powershell(text)
	}
	if err != nil {
		s.errorf("terminal: %v", err)
		return 2
	}
	cmds := make([]resolved, len(stages))
	for i, st := range stages {
		args, err := tokenize(st.text, homeDir())
		if err != nil {
			s.errorf("terminal: %v", err)
			return 2
		}
		if len(args) == 0 {
			s.errorf("terminal: erreur de syntaxe près de « | »")
			return 2
		}
		r, ok := s.resolve(args)
		if !ok {
			return s.powershell(text) // cmdlet ou commande cmd : PowerShell sait faire
		}
		if err := r.guard(args[0]); err != nil {
			s.errorf("%v", err)
			return 1
		}
		if r.builtin != nil && strings.EqualFold(args[0], "ls") && (i < len(stages)-1 || st.out != "") {
			r.args = append([]string{"-1"}, r.args...) // un nom par ligne, comme ls dans un tube
		}
		cmds[i] = r
	}
	return s.runPipeline(stages, cmds)
}

// runPipeline relie les commandes par de vrais tubes et attend la fin ; renvoie le code de la dernière.
func (s *Shell) runPipeline(stages []stage, cmds []resolved) int {
	var files []*os.File
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	open := func(path string, write, appendMode bool) (*os.File, error) {
		flag := os.O_RDONLY
		if write {
			flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			if appendMode {
				flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
			}
		}
		f, err := os.OpenFile(path, flag, 0o644)
		if err == nil {
			files = append(files, f)
		}
		return f, err
	}

	// Plusieurs commandes écrivent en même temps dans la sortie du terminal : si ce n'est pas
	// un fichier (tests, sortie capturée), on protège les écritures.
	out, errOut := s.out, s.errOut
	if _, isFile := out.(*os.File); !isFile {
		mu := &sync.Mutex{}
		out, errOut = &lockedWriter{mu: mu, w: s.out}, &lockedWriter{mu: mu, w: s.errOut}
	}

	n := len(stages)
	type ends struct {
		out, err io.Writer
		stdin    io.Reader
		closers  []io.Closer // à fermer quand la commande est finie
	}
	plan := make([]ends, n)
	var prevR *os.File
	for i, st := range stages {
		e := ends{out: out, err: errOut}
		if prevR != nil {
			e.stdin = prevR
			e.closers = append(e.closers, prevR)
			prevR = nil
		}
		if st.in != "" {
			f, err := open(st.in, false, false)
			if err != nil {
				s.errorf("terminal: %s : %v", st.in, unwrap(err))
				return 1
			}
			e.stdin = f
		}
		if i < n-1 {
			r, w, err := os.Pipe()
			if err != nil {
				s.errorf("terminal: %v", err)
				return 1
			}
			e.out = w
			e.closers = append(e.closers, w)
			prevR = r
		}
		if st.out != "" {
			f, err := open(st.out, true, st.outAppend)
			if err != nil {
				s.errorf("terminal: %s : %v", st.out, unwrap(err))
				return 1
			}
			e.out = f
		}
		switch {
		case st.bothFile || st.errToOut:
			e.err = e.out
		case st.err != "":
			f, err := open(st.err, true, st.errAppend)
			if err != nil {
				s.errorf("terminal: %s : %v", st.err, unwrap(err))
				return 1
			}
			e.err = f
		}
		plan[i] = e
	}

	codes := make([]int, n)
	var wg sync.WaitGroup
	for i, c := range cmds {
		e := plan[i]
		closeAll := func() {
			for _, cl := range e.closers {
				cl.Close()
			}
		}
		if c.builtin != nil {
			wg.Add(1)
			go func(i int, c resolved, e ends) {
				defer wg.Done()
				defer closeAll()
				in := s.in
				if e.stdin != nil {
					in = bufio.NewReader(e.stdin)
				}
				sub := &Shell{in: in, out: e.out, errOut: e.err, width: s.width, admin: s.admin, prevDir: s.prevDir,
					color: s.color && e.out == out}
				codes[i] = c.builtin.run(sub, c.args)
			}(i, c, e)
			continue
		}
		cmd := exec.Command(c.path, append(append([]string{}, c.pre...), c.args...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, e.out, e.err
		if e.stdin != nil {
			cmd.Stdin = e.stdin
		}
		if err := cmd.Start(); err != nil {
			s.errorf("terminal: %v", err)
			codes[i] = 126
			closeAll()
			continue
		}
		// Le terminal ferme sa copie des tubes : la fin de fichier passe d'une commande à l'autre.
		closeAll()
		wg.Add(1)
		go func(i int, cmd *exec.Cmd) {
			defer wg.Done()
			codes[i] = exitCode(cmd.Wait())
		}(i, cmd)
	}
	wg.Wait()
	return codes[n-1]
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if c := ee.ExitCode(); c >= 0 {
			return c
		}
	}
	return 1
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
