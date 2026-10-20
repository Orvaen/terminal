package shell

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/docker"
	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/req"
	"github.com/Orvaen/terminal/internal/scripts"
	"github.com/Orvaen/terminal/internal/ui"
)

type builtin struct {
	run   func(s *Shell, args []string) int
	usage string
	help  string
}

// Rempli dans init() : certaines commandes (help, which) lisent elles-mêmes la table.
var builtins map[string]builtin

// Ordre d'affichage dans « help ».
var builtinOrder = []string{
	"apt", "sudo", "docker", "req", "run",
	"ls", "cd", "pwd", "cat", "touch", "mkdir", "rm", "rmdir", "cp", "mv",
	"echo", "clear", "which", "export", "unset", "env", "path", "open", "prompt", "help", "exit",
}

func init() {
	builtins = map[string]builtin{
		"apt":    {runApt, "apt <commande> [paquets]", "Gestion des logiciels (winget) et mises à jour Windows"},
		"docker": {runDocker, "docker <commande>", "Docker léger, sans Docker Desktop (apt install docker)"},
		"req":    {runReq, "req <fichier.http> | req nouveau", "Teste des requêtes HTTP (GET, POST...) décrites dans un fichier"},
		"run":    {runScripts, "run [script] | run -p <a> <b>", "Lance les scripts du projet (terminal.json), en parallèle avec -p"},
		"sudo":   {runSudo, "sudo <commande>", "Lance une commande en administrateur"},
		"ls":     {runLs, "ls [-a] [-l] [-h] [chemin...]", "Liste un dossier"},
		"cd":     {runCd, "cd [dossier | - | ~]", "Change de dossier (accepte /c/Users/...)"},
		"pwd":    {runPwd, "pwd", "Affiche le dossier courant"},
		"cat":    {runCat, "cat [fichier...]", "Affiche le contenu de fichiers"},
		"touch":  {runTouch, "touch <fichier...>", "Crée un fichier vide ou met à jour sa date"},
		"mkdir":  {runMkdir, "mkdir [-p] <dossier...>", "Crée des dossiers"},
		"rm":     {runRm, "rm [-r] [-f] <chemin...>", "Supprime des fichiers (et dossiers avec -r)"},
		"rmdir":  {runRmdir, "rmdir <dossier...>", "Supprime des dossiers vides"},
		"cp":     {runCp, "cp [-r] <source...> <destination>", "Copie des fichiers ou dossiers"},
		"mv":     {runMv, "mv <source...> <destination>", "Déplace ou renomme"},
		"echo":   {runEcho, "echo [-n] [texte...]", "Affiche du texte"},
		"clear":  {runClear, "clear", "Efface l'écran"},
		"which":  {runWhich, "which <commande...>", "Indique où se trouve une commande"},
		"export": {runExport, "export NOM=valeur", "Définit une variable d'environnement"},
		"unset":  {runUnset, "unset NOM", "Supprime une variable d'environnement"},
		"env":    {runEnv, "env", "Liste les variables d'environnement"},
		"open":   {runOpen, "open [chemin]", "Ouvre un dossier ou fichier dans l'Explorateur"},
		"path":   {runPath, "path [qui|monter|masquer|nettoyer]", "Voit et range le PATH (quel make, quel python gagne…)"},
		"prompt": {runPrompt, "prompt [style]", "Change l'apparence du prompt (aperçu de chaque style)"},
		"help":   {runHelp, "help", "Affiche cette aide"},
		"exit":   {runExit, "exit [code]", "Quitte le terminal"},
	}
	builtins["apt-get"] = builtins["apt"]
	builtins["docker-compose"] = builtin{func(s *Shell, a []string) int { return runDocker(s, append([]string{"compose"}, a...)) }, "", ""}
	builtins["ll"] = builtin{func(s *Shell, a []string) int { return runLs(s, append([]string{"-l"}, a...)) }, "", ""}
}

// parseFlags sépare les options courtes (-rf) des arguments. allowed liste les lettres acceptées.
func parseFlags(cmd string, args []string, allowed string) (map[rune]bool, []string, error) {
	flags := map[rune]bool{}
	var rest []string
	noMore := false
	for _, a := range args {
		if !noMore && a == "--" {
			noMore = true
			continue
		}
		if !noMore && len(a) > 1 && a[0] == '-' {
			for _, r := range a[1:] {
				if !strings.ContainsRune(allowed, r) {
					return nil, nil, fmt.Errorf("%s: option invalide -- '%c'", cmd, r)
				}
				flags[r] = true
			}
			continue
		}
		rest = append(rest, convertPath(a, runtime.GOOS))
	}
	return flags, rest, nil
}

func runApt(s *Shell, args []string) int { return apt.Run(args) }

func runDocker(s *Shell, args []string) int { return docker.Run(args) }

func runReq(s *Shell, args []string) int { return req.Run(args, s.out) }

func runScripts(s *Shell, args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		s.errorf("run: %v", err)
		return 1
	}
	r := scripts.NewRunner(s.color)
	r.Out, r.Err = s.out, s.errOut
	return scripts.Main(r, cwd, args)
}

func runSudo(s *Shell, args []string) int {
	if len(args) == 0 {
		s.errorf("usage : sudo <commande>")
		return 1
	}
	line := joinArgs(args)
	if s.admin || !needsAdmin(args) {
		return s.RunLine(line)
	}
	exe, err := os.Executable()
	if err != nil {
		s.errorf("sudo: %v", err)
		return 1
	}
	if platform.NativeSudoInline() {
		return s.execProgram("sudo.exe", []string{exe, "-c", line})
	}
	ui.Info("Ouverture d'une fenêtre administrateur…")
	return platform.RunSelfElevated("-c", line, "--pause")
}

// needsAdmin : apt n'a besoin des droits admin que pour Windows Update,
// winget demande lui-même l'élévation pour les installeurs qui en ont besoin.
func needsAdmin(args []string) bool {
	switch strings.ToLower(args[0]) {
	case "apt", "apt-get":
		for _, a := range args[1:] {
			switch strings.ToLower(a) {
			case "full-upgrade", "dist-upgrade", "windows-update":
				return true
			}
		}
		return false
	case "docker", "docker-compose":
		return false // le moteur tourne dans WSL : pas besoin des droits admin Windows
	case "req", "run":
		return false
	}
	return true
}

type lsItem struct {
	name string
	info fs.FileInfo
}

func runLs(s *Shell, args []string) int {
	flags, paths, err := parseFlags("ls", args, "alh1")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	code := 0
	var files []lsItem
	var dirs []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			s.errorf("ls: impossible d'accéder à '%s' : %v", p, unwrap(err))
			code = 2
			continue
		}
		if info.IsDir() {
			dirs = append(dirs, p)
		} else {
			files = append(files, lsItem{p, info})
		}
	}
	if len(files) > 0 {
		s.printLs(files, flags)
	}
	for i, d := range dirs {
		if len(paths) > 1 {
			if i > 0 || len(files) > 0 {
				fmt.Fprintln(s.out)
			}
			fmt.Fprintf(s.out, "%s:\n", d)
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			s.errorf("ls: impossible d'ouvrir le dossier '%s' : %v", d, unwrap(err))
			code = 2
			continue
		}
		var items []lsItem
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if !flags['a'] && (strings.HasPrefix(e.Name(), ".") || platform.IsHidden(info)) {
				continue
			}
			items = append(items, lsItem{e.Name(), info})
		}
		s.printLs(items, flags)
	}
	return code
}

func (s *Shell) printLs(items []lsItem, flags map[rune]bool) {
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].name) < strings.ToLower(items[j].name) })

	if flags['l'] {
		for _, it := range items {
			kind, perm := "-", "rw"
			if it.info.IsDir() {
				kind = "d"
			}
			if it.info.Mode().Perm()&0o200 == 0 {
				perm = "r-"
			}
			size := strconv.FormatInt(it.info.Size(), 10)
			if flags['h'] {
				size = humanSize(it.info.Size())
			}
			if it.info.IsDir() {
				size = "-"
			}
			fmt.Fprintf(s.out, "%s%s %10s %s %s\n", kind, perm, size,
				it.info.ModTime().Format("2006-01-02 15:04"), s.lsName(it))
		}
		return
	}
	if flags['1'] {
		for _, it := range items {
			fmt.Fprintln(s.out, s.lsName(it))
		}
		return
	}

	// Affichage en colonnes, comme ls : remplissage colonne par colonne.
	if len(items) == 0 {
		return
	}
	widths := make([]int, len(items))
	maxw := 0
	for i, it := range items {
		widths[i] = utf8.RuneCountInString(it.name)
		if it.info.IsDir() {
			widths[i]++
		}
		if widths[i] > maxw {
			maxw = widths[i]
		}
	}
	colw := maxw + 2
	cols := s.width() / colw
	if cols < 1 {
		cols = 1
	}
	rows := (len(items) + cols - 1) / cols
	for r := 0; r < rows; r++ {
		var line strings.Builder
		for c := 0; c < cols; c++ {
			i := c*rows + r
			if i >= len(items) {
				break
			}
			line.WriteString(s.lsName(items[i]))
			if next := (c+1)*rows + r; next < len(items) {
				line.WriteString(strings.Repeat(" ", colw-widths[i]))
			}
		}
		fmt.Fprintln(s.out, line.String())
	}
}

func (s *Shell) lsName(it lsItem) string {
	switch {
	case it.info.IsDir():
		return s.paint("1;34", it.name) + "/"
	case it.info.Mode()&fs.ModeSymlink != 0:
		return s.paint("1;36", it.name)
	}
	switch strings.ToLower(filepath.Ext(it.name)) {
	case ".exe", ".bat", ".cmd", ".ps1", ".msi":
		return s.paint("1;32", it.name)
	}
	return it.name
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTPE"[exp])
}

func runCd(s *Shell, args []string) int {
	var target string
	switch {
	case len(args) == 0:
		target = homeDir()
	case args[0] == "-":
		if s.prevDir == "" {
			s.errorf("cd: aucun dossier précédent")
			return 1
		}
		target = s.prevDir
		fmt.Fprintln(s.out, displayPath(target, homeDir()))
	default:
		target = convertPath(args[0], runtime.GOOS)
	}
	cur, _ := os.Getwd()
	if err := os.Chdir(target); err != nil {
		s.errorf("cd: %s : %v", target, unwrap(err))
		return 1
	}
	s.prevDir = cur
	return 0
}

func runPwd(s *Shell, args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		s.errorf("pwd: %v", err)
		return 1
	}
	fmt.Fprintln(s.out, cwd)
	return 0
}

func runCat(s *Shell, args []string) int {
	_, files, err := parseFlags("cat", args, "")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	if len(files) == 0 {
		files = []string{"-"} // sans fichier : l'entrée (un tube, un fichier avec <, ou le clavier)
	}
	code := 0
	for _, f := range files {
		if f == "-" {
			if s.in != nil {
				_, _ = io.Copy(s.out, s.in)
			}
			continue
		}
		fh, err := os.Open(f)
		if err != nil {
			s.errorf("cat: %s : %v", f, unwrap(err))
			code = 1
			continue
		}
		if info, err := fh.Stat(); err == nil && info.IsDir() {
			s.errorf("cat: %s : c'est un dossier", f)
			fh.Close()
			code = 1
			continue
		}
		_, _ = io.Copy(s.out, fh)
		fh.Close()
	}
	return code
}

func runTouch(s *Shell, args []string) int {
	_, files, err := parseFlags("touch", args, "")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	if len(files) == 0 {
		s.errorf("usage : touch <fichier...>")
		return 1
	}
	code := 0
	now := time.Now()
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			if err := os.Chtimes(f, now, now); err != nil {
				s.errorf("touch: %s : %v", f, unwrap(err))
				code = 1
			}
			continue
		}
		fh, err := os.OpenFile(f, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			s.errorf("touch: %s : %v", f, unwrap(err))
			code = 1
			continue
		}
		fh.Close()
	}
	return code
}

func runMkdir(s *Shell, args []string) int {
	flags, dirs, err := parseFlags("mkdir", args, "p")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	if len(dirs) == 0 {
		s.errorf("usage : mkdir [-p] <dossier...>")
		return 1
	}
	code := 0
	for _, d := range dirs {
		var err error
		if flags['p'] {
			err = os.MkdirAll(d, 0o755)
		} else {
			err = os.Mkdir(d, 0o755)
		}
		if err != nil {
			s.errorf("mkdir: impossible de créer '%s' : %v", d, unwrap(err))
			code = 1
		}
	}
	return code
}

// isProtected refuse de supprimer la racine d'un disque ou le dossier perso.
func isProtected(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	abs = filepath.Clean(abs)
	if abs == filepath.VolumeName(abs)+string(filepath.Separator) || abs == string(filepath.Separator) {
		return true
	}
	home := homeDir()
	return home != "" && strings.EqualFold(abs, filepath.Clean(home))
}

func runRm(s *Shell, args []string) int {
	flags, paths, err := parseFlags("rm", args, "rRfv")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	recursive, force := flags['r'] || flags['R'], flags['f']
	if len(paths) == 0 {
		if force {
			return 0
		}
		s.errorf("usage : rm [-r] [-f] <chemin...>")
		return 1
	}
	code := 0
	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			if !force {
				s.errorf("rm: impossible de supprimer '%s' : %v", p, unwrap(err))
				code = 1
			}
			continue
		}
		if isProtected(p) {
			s.errorf("rm: refus de supprimer '%s' (racine du disque ou dossier personnel)", p)
			code = 1
			continue
		}
		if info.IsDir() {
			if !recursive {
				s.errorf("rm: impossible de supprimer '%s' : c'est un dossier (utilise -r)", p)
				code = 1
				continue
			}
			err = os.RemoveAll(p)
		} else {
			err = os.Remove(p)
		}
		if err != nil {
			s.errorf("rm: impossible de supprimer '%s' : %v", p, unwrap(err))
			code = 1
		} else if flags['v'] {
			fmt.Fprintf(s.out, "'%s' supprimé\n", p)
		}
	}
	return code
}

func runRmdir(s *Shell, args []string) int {
	_, dirs, err := parseFlags("rmdir", args, "")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	if len(dirs) == 0 {
		s.errorf("usage : rmdir <dossier...>")
		return 1
	}
	code := 0
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err == nil && !info.IsDir() {
			s.errorf("rmdir: '%s' n'est pas un dossier", d)
			code = 1
			continue
		}
		if err := os.Remove(d); err != nil {
			s.errorf("rmdir: impossible de supprimer '%s' : %v", d, unwrap(err))
			code = 1
		}
	}
	return code
}

// destinationFor calcule la cible d'une copie ou d'un déplacement (dans le dossier dest s'il existe).
func destinationFor(src, dest string, multiple bool) (string, error) {
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return filepath.Join(dest, filepath.Base(src)), nil
	}
	if multiple {
		return "", fmt.Errorf("la cible '%s' n'est pas un dossier", dest)
	}
	return dest, nil
}

func runCp(s *Shell, args []string) int {
	flags, paths, err := parseFlags("cp", args, "rRv")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	if len(paths) < 2 {
		s.errorf("usage : cp [-r] <source...> <destination>")
		return 1
	}
	recursive := flags['r'] || flags['R']
	srcs, dest := paths[:len(paths)-1], paths[len(paths)-1]
	code := 0
	for _, src := range srcs {
		info, err := os.Stat(src)
		if err != nil {
			s.errorf("cp: '%s' : %v", src, unwrap(err))
			code = 1
			continue
		}
		target, err := destinationFor(src, dest, len(srcs) > 1)
		if err != nil {
			s.errorf("cp: %v", err)
			return 1
		}
		if info.IsDir() {
			if !recursive {
				s.errorf("cp: -r non spécifié ; '%s' (dossier) ignoré", src)
				code = 1
				continue
			}
			if isInside(target, src) {
				s.errorf("cp: impossible de copier '%s' dans lui-même", src)
				code = 1
				continue
			}
			err = copyDir(src, target)
		} else {
			err = copyFile(src, target, info.Mode())
		}
		if err != nil {
			s.errorf("cp: %v", unwrap(err))
			code = 1
		} else if flags['v'] {
			fmt.Fprintf(s.out, "'%s' -> '%s'\n", src, target)
		}
	}
	return code
}

func isInside(path, dir string) bool {
	p, err1 := filepath.Abs(path)
	d, err2 := filepath.Abs(dir)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(d, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyFile(src, dest string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target, info.Mode())
	})
}

func runMv(s *Shell, args []string) int {
	flags, paths, err := parseFlags("mv", args, "v")
	if err != nil {
		s.errorf("%v", err)
		return 2
	}
	if len(paths) < 2 {
		s.errorf("usage : mv <source...> <destination>")
		return 1
	}
	srcs, dest := paths[:len(paths)-1], paths[len(paths)-1]
	code := 0
	for _, src := range srcs {
		target, err := destinationFor(src, dest, len(srcs) > 1)
		if err != nil {
			s.errorf("mv: %v", err)
			return 1
		}
		if err := os.Rename(src, target); err != nil {
			s.errorf("mv: impossible de déplacer '%s' : %v", src, unwrap(err))
			code = 1
		} else if flags['v'] {
			fmt.Fprintf(s.out, "'%s' -> '%s'\n", src, target)
		}
	}
	return code
}

func runEcho(s *Shell, args []string) int {
	newline := true
	if len(args) > 0 && args[0] == "-n" {
		newline = false
		args = args[1:]
	}
	fmt.Fprint(s.out, strings.Join(args, " "))
	if newline {
		fmt.Fprintln(s.out)
	}
	return 0
}

func runClear(s *Shell, args []string) int {
	if s.color {
		fmt.Fprint(s.out, "\x1b[2J\x1b[3J\x1b[H")
		return 0
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	}
	return 0
}

func runWhich(s *Shell, args []string) int {
	if len(args) == 0 {
		s.errorf("usage : which <commande...>")
		return 1
	}
	code := 0
	for _, a := range args {
		if _, ok := builtins[strings.ToLower(a)]; ok {
			fmt.Fprintf(s.out, "%s : commande intégrée\n", a)
			continue
		}
		if p, err := exec.LookPath(a); err == nil {
			fmt.Fprintln(s.out, p)
			continue
		}
		s.errorf("%s : introuvable", a)
		code = 1
	}
	return code
}

func runExport(s *Shell, args []string) int {
	if len(args) == 0 {
		return runEnv(s, nil)
	}
	code := 0
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			s.errorf("export: « %s » : utilise NOM=valeur", a)
			code = 1
			continue
		}
		_ = os.Setenv(k, v)
	}
	return code
}

func runUnset(s *Shell, args []string) int {
	for _, a := range args {
		_ = os.Unsetenv(a)
	}
	return 0
}

func runEnv(s *Shell, args []string) int {
	env := os.Environ()
	sort.Slice(env, func(i, j int) bool { return strings.ToLower(env[i]) < strings.ToLower(env[j]) })
	for _, e := range env {
		if strings.HasPrefix(e, "=") { // variables internes de cmd (=C:=C:\...)
			continue
		}
		fmt.Fprintln(s.out, e)
	}
	return 0
}

func runOpen(s *Shell, args []string) int {
	target := "."
	if len(args) > 0 {
		target = convertPath(args[0], runtime.GOOS)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		s.errorf("open: %v", err)
		return 1
	}
	if _, err := os.Stat(abs); err != nil {
		s.errorf("open: %s : %v", target, unwrap(err))
		return 1
	}
	opener := "xdg-open"
	if runtime.GOOS == "windows" {
		opener = "explorer.exe"
	}
	if err := exec.Command(opener, abs).Start(); err != nil {
		s.errorf("open: %v", err)
		return 1
	}
	return 0
}

func runHelp(s *Shell, args []string) int {
	if len(args) > 0 {
		code := 0
		for i, name := range args {
			if i > 0 {
				fmt.Fprintln(s.out)
			}
			if c := helpFor(s, name); c != 0 {
				code = c
			}
		}
		return code
	}
	fmt.Fprintf(s.out, "%s %s — commandes intégrées :\n\n", AppName, apt.Version)
	for _, name := range builtinOrder {
		b := builtins[name]
		fmt.Fprintf(s.out, "  %-36s %s\n", b.usage, b.help)
	}
	fmt.Fprint(s.out, `
Aide d'une commande : help <commande> (ex. help prompt).
Aussi : apt-get (= apt), docker-compose (= docker compose), ll (= ls -l).
Enchaîne avec  &&  ||  ;   et relie avec  |  >  >>  <  2>&1  (comme bash).
Tout le reste est lancé tel quel : programmes Windows (git, code, notepad...)
et commandes PowerShell (dir, Get-Process...).
`)
	if coreutilsDir() != "" {
		fmt.Fprintln(s.out, "Coreutils de Microsoft : 78 commandes Unix (grep, head, sort, find, xargs, wc…).")
	} else {
		fmt.Fprintln(s.out, "Pour 78 commandes Unix de plus (grep, head, sort, xargs…) : apt install coreutils")
	}
	return 0
}

func runExit(s *Shell, args []string) int {
	s.exiting = true
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil {
			return n
		}
	}
	return s.last
}

// unwrap retire l'opération et le chemin des erreurs du système de fichiers (déjà affichés).
func unwrap(err error) error {
	if pe, ok := err.(*fs.PathError); ok {
		return pe.Err
	}
	if le, ok := err.(*os.LinkError); ok {
		return le.Err
	}
	return err
}
