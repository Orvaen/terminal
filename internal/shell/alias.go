package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Alias à la bash : « alias code='"C:\…\Code.exe"' » puis « code . ».
// Comme dans bash, un alias tapé dans le terminal ne dure que la session ;
// pour le garder, on l'écrit dans ~/.terminalrc, relu à chaque ouverture.

// rcFile : le fichier de démarrage du terminal (~/.terminalrc).
func rcFile() string {
	return filepath.Join(homeDir(), ".terminalrc")
}

// loadRC exécute ~/.terminalrc ligne par ligne (alias, export, prompt…), s'il existe.
func (s *Shell) loadRC() {
	f, err := os.Open(rcFile())
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if code := s.RunLine(line); code != 0 {
			s.errorf("~/.terminalrc, ligne %d : « %s » a échoué (code %d)", n, line, code)
		}
	}
	s.last = 0
}

func runAlias(s *Shell, args []string) int {
	if len(args) > 0 && args[0] == "-p" {
		args = args[1:]
	}
	if len(args) == 0 {
		names := make([]string, 0, len(s.aliases))
		for name := range s.aliases {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(s.out, "alias %s=%s\n", name, quoteAlias(s.aliases[name]))
		}
		return 0
	}
	code := 0
	for _, a := range args {
		name, value, ok := strings.Cut(a, "=")
		if !ok {
			if v, found := s.aliases[name]; found {
				fmt.Fprintf(s.out, "alias %s=%s\n", name, quoteAlias(v))
			} else {
				s.errorf("alias: %s : non trouvé", name)
				code = 1
			}
			continue
		}
		if !validAliasName(name) {
			s.errorf("alias: « %s » : nom d'alias invalide", name)
			code = 1
			continue
		}
		if s.aliases == nil {
			s.aliases = map[string]string{}
		}
		s.aliases[name] = value
	}
	return code
}

func runUnalias(s *Shell, args []string) int {
	if len(args) == 0 {
		s.errorf("unalias: usage : unalias [-a] nom [nom…]")
		return 2
	}
	if args[0] == "-a" {
		s.aliases = nil
		return 0
	}
	code := 0
	for _, name := range args {
		if _, ok := s.aliases[name]; !ok {
			s.errorf("unalias: %s : non trouvé", name)
			code = 1
			continue
		}
		delete(s.aliases, name)
	}
	return code
}

func validAliasName(name string) bool {
	return name != "" && !strings.ContainsAny(name, " \t'\"`$=/\\|&;<>()")
}

// quoteAlias met la valeur entre apostrophes, comme bash l'affiche (et la relit).
func quoteAlias(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// expandAliases remplace le premier mot d'une commande s'il s'agit d'un alias.
// Comme bash : un mot entre guillemets n'est pas remplacé, un alias ne se remplace
// pas lui-même (alias ls='ls -la'), et une valeur finie par une espace fait aussi
// regarder le mot suivant (alias sudo='sudo ').
func (s *Shell) expandAliases(text string) string {
	if len(s.aliases) == 0 {
		return text
	}
	seen := map[string]bool{}
	var done strings.Builder
	rest := strings.TrimLeft(text, " \t")
	for {
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			end = len(rest)
		}
		word := rest[:end]
		value, ok := s.aliases[word]
		if !ok || seen[word] || strings.ContainsAny(word, "'\"\\$`") {
			break
		}
		seen[word] = true
		rest = value + rest[end:]
		if strings.HasSuffix(value, " ") || strings.HasSuffix(value, "\t") {
			// Le mot suivant peut être un alias aussi.
			done.WriteString(value)
			rest = strings.TrimLeft(rest[len(value):], " \t")
			seen = map[string]bool{}
			continue
		}
		// La valeur peut commencer par un autre alias (alias ll='ls -l', alias l=ll).
		rest = strings.TrimLeft(rest, " \t")
	}
	return done.String() + rest
}
