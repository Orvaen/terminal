package scripts

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Orvaen/terminal/internal/platform"
)

// Runner lance les scripts. Chaque commande tourne dans son propre processus « terminal -c »,
// avec son dossier et ses variables : les commandes du terminal (ls, rm, apt, docker, && …)
// y marchent, et plusieurs commandes peuvent tourner en même temps sans se gêner.
type Runner struct {
	In       io.Reader
	Out, Err io.Writer
	Color    bool

	// Command prépare l'exécution d'une ligne (remplacée dans les tests).
	Command func(line string) *exec.Cmd
}

// NewRunner crée un Runner branché sur la console, qui relance l'exécutable courant avec -c.
func NewRunner(color bool) *Runner {
	return &Runner{
		In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Color: color,
		Command: func(line string) *exec.Cmd {
			exe, err := os.Executable()
			if err != nil {
				exe = "terminal"
			}
			return exec.Command(exe, "-c", line)
		},
	}
}

func (r *Runner) paint(code, s string) string {
	if !r.Color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (r *Runner) errorf(format string, a ...any) {
	fmt.Fprintln(r.Err, r.paint("1;31", "E:")+" "+fmt.Sprintf(format, a...))
}

// Run lance le script name. extra (les arguments après --) est ajouté à sa commande.
func (r *Runner) Run(f *File, name string, extra []string) int {
	s, ok := f.Get(name)
	if !ok {
		r.unknown(f, name)
		return 1
	}
	dir, env, err := r.context(f, s)
	if err != nil {
		r.errorf("script %q : %v", name, err)
		return 1
	}
	if len(extra) > 0 {
		if len(s.Steps) != 1 {
			r.errorf("script %q : les arguments après -- ne marchent qu'avec un script d'une seule commande", name)
			return 1
		}
		s.Steps = []string{s.Steps[0] + " " + quoteArgs(extra)}
	}
	if len(s.Parallel) > 0 {
		return r.parallel(tasksFor(s.Parallel), dir, env)
	}
	for _, step := range s.Steps {
		fmt.Fprintln(r.Out, r.paint("2", "> "+name+" : "+step))
		if code := r.exec(step, dir, env); code != 0 {
			if len(s.Steps) > 1 {
				r.errorf("script %q arrêté : « %s » a échoué (code %d)", name, step, code)
			}
			return code
		}
	}
	return 0
}

// RunSequence lance plusieurs scripts l'un après l'autre et s'arrête au premier échec.
func (r *Runner) RunSequence(f *File, names []string) int {
	for _, n := range names {
		if _, ok := f.Get(n); !ok {
			r.unknown(f, n)
			return 1
		}
	}
	for _, n := range names {
		if code := r.Run(f, n, nil); code != 0 {
			return code
		}
	}
	return 0
}

// RunParallel lance plusieurs scripts en même temps, chacun avec sa sortie préfixée.
func (r *Runner) RunParallel(f *File, names []string) int {
	lines := make([]string, len(names))
	for i, n := range names {
		if _, ok := f.Get(n); !ok {
			r.unknown(f, n)
			return 1
		}
		lines[i] = "run " + n
	}
	return r.parallel(tasksFor(lines), f.Dir, r.baseEnv(f.Dir))
}

func (r *Runner) unknown(f *File, name string) {
	r.errorf("script inconnu : %s", name)
	if names := f.Names(); len(names) > 0 {
		fmt.Fprintln(r.Err, "Scripts de "+filepath.Base(f.Path)+" : "+strings.Join(names, ", "))
	}
}

// context : dossier et variables d'environnement d'un script.
func (r *Runner) context(f *File, s Script) (string, []string, error) {
	dir := f.Dir
	if s.Cwd != "" {
		dir = s.Cwd
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(f.Dir, filepath.FromSlash(s.Cwd))
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return "", nil, fmt.Errorf(`le dossier "cwd" %s n'existe pas`, s.Cwd)
		}
	}
	env := r.baseEnv(dir)
	for k, v := range s.Env {
		env = setEnv(env, k, v)
	}
	return dir, env, nil
}

// baseEnv : l'environnement courant, avec node_modules/.bin du dossier dans le PATH (comme npm).
func (r *Runner) baseEnv(dir string) []string {
	env := os.Environ()
	bin := filepath.Join(dir, "node_modules", ".bin")
	if info, err := os.Stat(bin); err == nil && info.IsDir() {
		path := getEnv(env, "PATH")
		if path != "" {
			path = bin + string(os.PathListSeparator) + path
		} else {
			path = bin
		}
		env = setEnv(env, "PATH", path)
	}
	return env
}

// exec lance une commande attachée à la console et renvoie son code de sortie.
func (r *Runner) exec(line, dir string, env []string) int {
	cmd := r.Command(line)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r.In, r.Out, r.Err
	return exitCode(cmd.Run(), r, line)
}

type task struct{ label, line string }

// tasksFor : une étiquette par commande, le nom du script pour « run x », sinon le premier mot.
func tasksFor(lines []string) []task {
	tasks := make([]task, len(lines))
	seen := map[string]int{}
	for i, l := range lines {
		fields := strings.Fields(l)
		label := fields[0]
		if strings.EqualFold(fields[0], "run") && len(fields) > 1 && !strings.HasPrefix(fields[1], "-") {
			label = fields[1]
		}
		seen[label]++
		if seen[label] > 1 {
			label = fmt.Sprintf("%s#%d", label, seen[label])
		}
		tasks[i] = task{label: label, line: l}
	}
	return tasks
}

var labelColors = []string{"36", "35", "33", "32", "34", "31", "96", "95"}

type result struct {
	i    int
	code int
}

// parallel lance les commandes en même temps. Dès que l'une échoue, les autres sont arrêtées
// (comme concurrently --kill-others-on-fail). Ctrl+C arrête tout.
func (r *Runner) parallel(tasks []task, dir string, env []string) int {
	width := 0
	for _, t := range tasks {
		width = max(width, len(t.label))
	}
	if r.Color && getEnv(env, "FORCE_COLOR") == "" {
		env = append(env, "FORCE_COLOR=1") // les sorties ne sont plus une console : garder les couleurs
	}

	var mu sync.Mutex
	cmds := make([]*exec.Cmd, len(tasks))
	writers := make([]*prefixWriter, len(tasks))
	done := make(chan result, len(tasks))
	running := 0

	for i, t := range tasks {
		prefix := r.paint(labelColors[i%len(labelColors)], fmt.Sprintf("[%-*s]", width, t.label)) + " "
		w := &prefixWriter{mu: &mu, out: r.Out, prefix: prefix}
		writers[i] = w
		cmd := r.Command(t.line)
		cmd.Dir, cmd.Env = dir, env
		cmd.Stdout, cmd.Stderr = w, w
		// Si un petit-enfant garde la sortie ouverte après l'arrêt de la commande, ne pas l'attendre indéfiniment.
		cmd.WaitDelay = 3 * time.Second
		w.line(r.paint("2", "> "+t.line))
		if err := cmd.Start(); err != nil {
			w.line(r.paint("31", "impossible de lancer : "+err.Error()))
			done <- result{i, 127}
			continue
		}
		cmds[i] = cmd
		running++
		go func(i int, cmd *exec.Cmd) {
			done <- result{i, exitCode(cmd.Wait(), nil, "")}
		}(i, cmd)
	}

	failed, firstCode := -1, 0
	finished := make([]bool, len(tasks))
	for range tasks {
		res := <-done
		finished[res.i] = true
		writers[res.i].flush()
		if failed >= 0 && res.i != failed {
			writers[res.i].line(r.paint("2", "arrêté"))
			continue
		}
		if res.code == 0 {
			writers[res.i].line(r.paint("32", "terminé"))
			continue
		}
		writers[res.i].line(r.paint("31", fmt.Sprintf("a échoué (code %d)", res.code)))
		if failed < 0 {
			failed, firstCode = res.i, res.code
			for j, c := range cmds {
				if c != nil && !finished[j] {
					_ = platform.KillTree(c.Process)
				}
			}
		}
	}
	if failed >= 0 {
		r.errorf("%s a échoué (code %d) : les autres commandes ont été arrêtées", tasks[failed].label, firstCode)
		return firstCode
	}
	return 0
}

// prefixWriter ajoute une étiquette au début de chaque ligne ; les lignes de plusieurs
// commandes ne se mélangent pas.
type prefixWriter struct {
	mu     *sync.Mutex
	out    io.Writer
	prefix string
	buf    []byte
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexByte(string(w.buf), '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.buf[:i]), "\r")
		fmt.Fprintln(w.out, w.prefix+line)
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// flush écrit la dernière ligne si elle ne finit pas par un retour à la ligne.
func (w *prefixWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		fmt.Fprintln(w.out, w.prefix+strings.TrimRight(string(w.buf), "\r"))
		w.buf = nil
	}
}

func (w *prefixWriter) line(s string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Fprintln(w.out, w.prefix+s)
}

func exitCode(err error, r *Runner, line string) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
		return 1 // arrêté par un signal
	}
	if r != nil {
		r.errorf("impossible de lancer « %s » : %v", line, err)
	}
	return 127
}

func getEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// setEnv remplace (sans tenir compte de la casse, comme Windows) ou ajoute une variable.
func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			key = k // garde l'orthographe existante (Path sous Windows)
			continue
		}
		out = append(out, kv)
	}
	return append(out, key+"="+value)
}

// quoteArgs recolle les arguments après -- en gardant ceux qui contiennent des espaces.
func quoteArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}
