package shell

// Complétion avec Tab et historique des commandes.

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/docker"
	"github.com/Orvaen/terminal/internal/platform"
)

// complete propose les mots possibles pour le mot sous le curseur.
func (s *Shell) complete(line []rune, pos int) (int, []string) {
	words, cur, start := splitForCompletion(line[:pos])
	for len(words) > 0 && strings.EqualFold(words[0], "sudo") {
		words = words[1:]
	}

	var cands []string
	filtered := true // les listes de noms sont filtrées ici ; les chemins le sont déjà
	switch {
	case len(words) == 0:
		if strings.ContainsAny(cur, `/\`) || strings.HasPrefix(cur, ".") || strings.HasPrefix(cur, "~") {
			cands, filtered = pathCandidates(cur, false), false
		} else {
			cands = commandNames()
		}
	case isApt(words[0]):
		cands = s.aptCandidates(words[1:])
	case strings.EqualFold(words[0], "docker"):
		if c, ok := dockerCandidates(words[1:]); ok {
			cands = c
		} else {
			cands, filtered = pathCandidates(cur, false), false
		}
	case strings.EqualFold(words[0], "cd") || strings.EqualFold(words[0], "rmdir"):
		cands, filtered = pathCandidates(cur, true), false
	default:
		cands, filtered = pathCandidates(cur, false), false
	}
	if filtered {
		cands = filterPrefix(cands, cur, words)
	}
	return start, cands
}

func isApt(w string) bool { return strings.EqualFold(w, "apt") || strings.EqualFold(w, "apt-get") }

func (s *Shell) aptCandidates(args []string) []string {
	if len(args) == 0 {
		return apt.Subcommands
	}
	installed, ok := apt.PackageSubcommand(args[0])
	switch {
	case !ok && strings.EqualFold(args[0], "list"):
		return []string{"--upgradable"}
	case !ok:
		return nil
	case installed:
		if s.installed == nil {
			return nil
		}
		return s.installed()
	default:
		return apt.InstallableNames()
	}
}

func dockerCandidates(args []string) ([]string, bool) {
	switch {
	case len(args) == 0:
		return docker.Subcommands, true
	case len(args) == 1 && strings.EqualFold(args[0], "engine"):
		return []string{"status", "start", "stop", "logs"}, true
	case len(args) == 1 && strings.EqualFold(args[0], "compose"):
		return []string{"up", "down", "ps", "logs", "build", "pull", "restart", "exec", "stop", "start"}, true
	}
	return nil, false
}

// splitForCompletion découpe le début de ligne : mots déjà tapés (de la commande en cours),
// mot en cours (sans guillemets) et position de son début.
func splitForCompletion(r []rune) (words []string, cur string, start int) {
	var b strings.Builder
	inWord := false
	var quote rune
	start = len(r)
	for i, c := range r {
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				b.WriteRune(c)
			}
			continue
		}
		switch {
		case c == '"' || c == '\'':
			if !inWord {
				inWord, start = true, i
			}
			quote = c
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, b.String())
				b.Reset()
				inWord = false
			}
		case c == ';' || c == '&' || c == '|':
			// nouvelle commande après ; && || |
			b.Reset()
			inWord = false
			words = nil
		default:
			if !inWord {
				inWord, start = true, i
			}
			b.WriteRune(c)
		}
	}
	if inWord {
		return words, b.String(), start
	}
	return words, "", len(r)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// filterPrefix garde les noms qui commencent par prefix (sans tenir compte de la casse)
// et retire ceux déjà présents sur la ligne.
func filterPrefix(cands []string, prefix string, typed []string) []string {
	already := map[string]bool{}
	for _, w := range typed {
		already[strings.ToLower(w)] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range cands {
		k := strings.ToLower(c)
		if hasPrefixFold(c, prefix) && !already[k] && !seen[k] {
			seen[k] = true
			out = append(out, c)
		}
	}
	return out
}

// pathCandidates complète un chemin : le texte tapé est gardé, les dossiers finissent par « / ».
func pathCandidates(word string, dirsOnly bool) []string {
	if word == "~" {
		return []string{"~/"}
	}
	dirTyped, base := "", word
	if i := strings.LastIndexAny(word, `/\`); i >= 0 {
		dirTyped, base = word[:i+1], word[i+1:]
	}
	listDir := "."
	if dirTyped != "" {
		listDir = convertPath(expandHome(dirTyped, homeDir()), runtime.GOOS)
	}
	entries, err := os.ReadDir(listDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !hasPrefixFold(name, base) {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(listDir, name)) // suit les liens
		if err != nil {
			continue
		}
		if base == "" && platform.IsHidden(info) {
			continue
		}
		if dirsOnly && !info.IsDir() {
			continue
		}
		c := dirTyped + name
		if info.IsDir() {
			c += "/"
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

var (
	cmdOnce  sync.Once
	cmdNames []string
)

// commandNames : commandes intégrées et programmes du PATH (lus une seule fois).
func commandNames() []string {
	cmdOnce.Do(func() {
		seen := map[string]bool{}
		add := func(n string) {
			if k := strings.ToLower(n); !seen[k] {
				seen[k] = true
				cmdNames = append(cmdNames, n)
			}
		}
		for _, n := range builtinOrder {
			add(n)
		}
		for n := range builtins {
			add(n)
		}
		exts := map[string]bool{".exe": true, ".cmd": true, ".bat": true, ".com": true}
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if runtime.GOOS == "windows" {
					ext := strings.ToLower(filepath.Ext(name))
					if !exts[ext] {
						continue
					}
					name = strings.TrimSuffix(name, filepath.Ext(name))
				} else if info, err := e.Info(); err != nil || info.Mode()&0o111 == 0 {
					continue
				}
				add(name)
			}
		}
	})
	return cmdNames
}

// prefetchForLine lance la lecture des logiciels installés dès qu'on tape une commande apt,
// pour que Tab réponde tout de suite.
func prefetchForLine(line string) {
	l := strings.TrimLeft(line, " ")
	if hasPrefixFold(l, "sudo ") {
		l = strings.TrimLeft(l[5:], " ")
	}
	if hasPrefixFold(l, "apt ") || hasPrefixFold(l, "apt-get ") {
		apt.PrefetchInstalled()
	}
}

// ---------------------------------------------------------------------------
// Historique

const historyMax = 1000

func historyFile() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "OrvaenTerminal", "history")
}

func loadHistory(path string) []string {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := sc.Text(); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > historyMax {
		lines = lines[len(lines)-historyMax:]
	}
	return lines
}

// remember ajoute une ligne à l'historique (sauf vide, doublon immédiat ou ligne commençant par une espace).
func (s *Shell) remember(line string) {
	if s.edit == nil || strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") {
		return
	}
	h := s.edit.history
	if len(h) > 0 && h[len(h)-1] == line {
		return
	}
	s.edit.history = append(h, line)
	if len(s.edit.history) > historyMax {
		s.edit.history = s.edit.history[len(s.edit.history)-historyMax:]
	}
	if s.historyPath == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.historyPath), 0o755)
	if f, err := os.OpenFile(s.historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.WriteString(line + "\n")
		f.Close()
	}
}
