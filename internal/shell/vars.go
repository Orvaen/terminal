package shell

// Variables, comme dans bash :
//
//	$NOM ${NOM}                      la valeur (variable du terminal ou d'environnement)
//	${NOM:-défaut} ${NOM:=défaut}    valeur par défaut (et affectation)
//	${NOM:+autre} ${NOM:?message}    autre valeur si définie ; erreur si vide
//	${#NOM}                          longueur
//	${NOM#motif} ${NOM##motif}       retire un début (court, long)
//	${NOM%motif} ${NOM%%motif}       retire une fin (court, long)
//	${NOM/motif/par} ${NOM//…/…}     remplace (la première, toutes)
//	${NOM:début:longueur}            extrait
//	${NOM^^} ${NOM,,} ${NOM^}        majuscules, minuscules, première lettre
//	$(commande) `commande`           la sortie d'une commande
//	$((1 + 2 * 3))                   un calcul entier
//	$? $$ $! $0 $# $@ $* $- $_       les paramètres spéciaux
//	$env:NOM                         la syntaxe de PowerShell, acceptée aussi
//
// NOM=valeur seul crée une variable du terminal (non transmise aux programmes, sauf si elle existe
// déjà dans l'environnement) ; export la transmet. NOM=valeur commande ne la donne qu'à cette
// commande. Hors guillemets, une valeur est découpée en mots (IFS) et ses jokers sont actifs ;
// entre "…" elle reste un seul argument ; entre '…' rien n'est développé.

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	osuser "os/user"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/platform"
)

// readonlyVars : comme dans bash, elles ne se changent pas.
var readonlyVars = map[string]bool{"UID": true, "EUID": true, "PPID": true, "BASHPID": true}

// dynamicVars : variables calculées à chaque lecture (listées par set).
var dynamicVars = []string{"BASHPID", "COLUMNS", "EPOCHREALTIME", "EPOCHSECONDS", "EUID", "HOSTNAME",
	"HOSTTYPE", "LINENO", "LINES", "MACHTYPE", "OSTYPE", "PPID", "PS1", "RANDOM", "SECONDS", "SRANDOM", "UID"}

const defaultIFS = " \t\n"

// ---------------------------------------------------------------------------
// Valeurs par défaut

// initVars prépare les variables qu'un shell Linux définit au démarrage.
func (s *Shell) initVars() {
	s.started = time.Now()
	s.secondsAt = s.started
	s.locals()["IFS"] = defaultIFS
	s.vars["PS2"] = "> "
	s.vars["PS4"] = "+ "
	s.vars["HISTSIZE"] = strconv.Itoa(historyMax)
	s.vars["HISTFILESIZE"] = strconv.Itoa(historyMax)
	s.vars["HISTCONTROL"] = "ignoreboth" // les lignes qui commencent par une espace et les doublons
	if s.historyPath != "" {
		s.vars["HISTFILE"] = s.historyPath
	}
}

// initEnv complète l'environnement transmis aux programmes, comme une session Linux.
// Une variable déjà définie (par Windows ou par le programme qui a lancé le terminal) est gardée.
func (s *Shell) initEnv() {
	setDefault := func(k, v string) {
		if _, ok := os.LookupEnv(k); !ok && v != "" {
			os.Setenv(k, v)
		}
	}
	h, _ := os.UserHomeDir()
	setDefault("HOME", h)
	user := firstNonEmpty(os.Getenv("USER"), os.Getenv("USERNAME"))
	if user == "" {
		if u, err := osuser.Current(); err == nil {
			user = u.Username[strings.LastIndex(u.Username, `\`)+1:] // DOMAINE\nom → nom
		}
	}
	setDefault("USER", user)
	setDefault("LOGNAME", user)
	lvl, _ := strconv.Atoi(os.Getenv("SHLVL"))
	os.Setenv("SHLVL", strconv.Itoa(lvl+1))
	if cwd, err := os.Getwd(); err == nil {
		os.Setenv("PWD", cwd)
	}
	setDefault("TERM", "xterm-256color")
	if s.color {
		setDefault("COLORTERM", "truecolor")
	}
	setDefault("LANG", platform.Locale()+".UTF-8")
	if _, ok := os.LookupEnv("TERM_PROGRAM"); !ok {
		os.Setenv("TERM_PROGRAM", "OrvaenTerminal")
		os.Setenv("TERM_PROGRAM_VERSION", apt.Version)
	}
	os.Setenv("ORVAEN_TERMINAL", apt.Version)
}

func (s *Shell) locals() map[string]string {
	if s.vars == nil {
		s.vars = map[string]string{}
	}
	return s.vars
}

func (s *Shell) rng() *rand.Rand {
	if s.rnd == nil {
		s.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return s.rnd
}

func (s *Shell) ifs() string {
	if v, ok := s.getVar("IFS"); ok {
		return v
	}
	return defaultIFS
}

// ---------------------------------------------------------------------------
// Lecture et écriture

// getVar renvoie la valeur d'une variable et si elle est définie.
func (s *Shell) getVar(name string) (string, bool) {
	switch name {
	case "?":
		return strconv.Itoa(s.last), true
	case "$", "BASHPID":
		return strconv.Itoa(os.Getpid()), true
	case "!":
		return "", false // pas de tâches de fond
	case "0":
		return "terminal", true
	case "#":
		return "0", true
	case "@", "*":
		return "", true
	case "-":
		if s.interactive {
			return "hiB", true
		}
		return "hB", true
	case "_":
		return s.lastArg, true
	case "RANDOM":
		return strconv.Itoa(s.rng().Intn(32768)), true
	case "SRANDOM":
		return strconv.FormatUint(uint64(s.rng().Uint32()), 10), true
	case "SECONDS":
		at := s.secondsAt
		if at.IsZero() {
			at = time.Now()
		}
		return strconv.Itoa(int(time.Since(at).Seconds())), true
	case "EPOCHSECONDS":
		return strconv.FormatInt(time.Now().Unix(), 10), true
	case "EPOCHREALTIME":
		now := time.Now()
		return fmt.Sprintf("%d.%06d", now.Unix(), now.Nanosecond()/1000), true
	case "LINENO":
		return strconv.Itoa(s.lineno), true
	case "PPID":
		return strconv.Itoa(os.Getppid()), true
	case "UID", "EUID":
		return strconv.Itoa(s.uid(name == "EUID")), true
	case "PS1":
		if s.ps1 != "" {
			return s.ps1, true
		}
		if s.promptFmt != "" {
			return s.promptFmt, true
		}
		return promptFormat(promptConfig{}), true
	}
	if name != "" && name[0] >= '0' && name[0] <= '9' {
		return "", false // $1, $2… : pas d'arguments dans une session
	}
	if v, ok := s.vars[name]; ok {
		return v, true
	}
	if v, ok := os.LookupEnv(name); ok {
		return v, true
	}
	switch name { // valeurs par défaut, qu'une affectation peut remplacer
	case "HOSTNAME":
		h, _ := os.Hostname()
		return h, true
	case "HOSTTYPE":
		return hostType(), true
	case "OSTYPE":
		return osType(), true
	case "MACHTYPE":
		return machType(), true
	case "COLUMNS":
		w := 80
		if s.width != nil {
			w = s.width()
		}
		return strconv.Itoa(w), true
	case "LINES":
		return strconv.Itoa(platform.TerminalHeight()), true
	}
	return "", false
}

// uid : sous Windows, 0 en administrateur (comme root) et 1000 sinon, comme un premier compte Linux.
func (s *Shell) uid(effective bool) int {
	if runtime.GOOS != "windows" {
		if effective {
			return os.Geteuid()
		}
		return os.Getuid()
	}
	if s.admin {
		return 0
	}
	return 1000
}

func hostType() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	}
	return runtime.GOARCH
}

// osType : « msys » sous Windows, comme Git Bash, pour que les scripts reconnaissent Windows.
func osType() string {
	switch runtime.GOOS {
	case "windows":
		return "msys"
	case "linux":
		return "linux-gnu"
	}
	return runtime.GOOS
}

func machType() string {
	if runtime.GOOS == "windows" {
		return hostType() + "-pc-msys"
	}
	return hostType() + "-pc-" + osType()
}

// validName : un nom de variable (lettres, chiffres, _ ; pas de chiffre en premier).
func validName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if !(r == '_' || isASCIILetter(r) || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// setVar affecte une variable : celle de l'environnement si elle y est déjà, sinon une variable du terminal.
func (s *Shell) setVar(name, value string) error {
	if !validName(name) {
		return fmt.Errorf("« %s » : identifiant non valable", name)
	}
	if readonlyVars[name] {
		return fmt.Errorf("%s : variable en lecture seule", name)
	}
	switch name {
	case "PS1":
		s.ps1 = value
		if _, ok := os.LookupEnv(name); ok {
			os.Setenv(name, value)
		}
		return nil
	case "SECONDS":
		n, _ := strconv.Atoi(value)
		s.secondsAt = time.Now().Add(-time.Duration(n) * time.Second)
		return nil
	case "RANDOM":
		n, _ := strconv.ParseInt(value, 10, 64)
		s.rnd = rand.New(rand.NewSource(n))
		return nil
	}
	if _, ok := os.LookupEnv(name); ok {
		delete(s.vars, name)
		return os.Setenv(name, value)
	}
	s.locals()[name] = value
	return nil
}

// exportVar transmet une variable aux programmes lancés ensuite.
func (s *Shell) exportVar(name string) {
	if v, ok := s.getVar(name); ok {
		os.Setenv(name, v)
		delete(s.vars, name)
	}
}

// unexportVar la garde dans le terminal sans la transmettre (export -n).
func (s *Shell) unexportVar(name string) {
	if v, ok := os.LookupEnv(name); ok {
		os.Unsetenv(name)
		s.locals()[name] = v
	}
}

func (s *Shell) unsetVar(name string) error {
	if readonlyVars[name] {
		return fmt.Errorf("%s : variable en lecture seule", name)
	}
	if name == "PS1" && s.ps1 != "" {
		s.ps1 = "" // retour au style choisi avec la commande prompt
	}
	delete(s.vars, name)
	return os.Unsetenv(name)
}

// allVars : toutes les variables (terminal, environnement, calculées), triées par nom.
func (s *Shell) allVars() [][2]string {
	seen := map[string]string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "=") { // variables internes de cmd (=C:=C:\...)
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		seen[k] = v
	}
	for k, v := range s.vars {
		seen[k] = v
	}
	for _, k := range dynamicVars {
		if _, ok := seen[k]; !ok {
			if v, ok := s.getVar(k); ok {
				seen[k] = v
			}
		}
	}
	out := make([][2]string, 0, len(seen))
	for k, v := range seen {
		out = append(out, [2]string{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// shellQuote : une valeur réutilisable telle quelle dans le terminal ('…' si besoin).
func shellQuote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\n\"'`$&|;<>()*?[]{}~\\#") {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'"'"'`) + "'"
}

// ---------------------------------------------------------------------------
// Commande : affectations, développement, environnement temporaire

type assignment struct{ name, value string }

// expandCommand découpe et développe une commande : les affectations NOM=valeur de tête, puis les arguments.
func (s *Shell) expandCommand(text string) ([]assignment, []string, error) {
	words, err := tokenizeWords(text, homeDir(), s)
	if err != nil {
		return nil, nil, err
	}
	var assigns []assignment
	i := 0
	for ; i < len(words) && words[i].assign; i++ {
		k, v, _ := strings.Cut(words[i].String(), "=")
		assigns = append(assigns, assignment{k, v})
	}
	args, err := expandWords(words[i:])
	return assigns, args, err
}

// expandTarget développe la cible d'une redirection.
func (s *Shell) expandTarget(text string) ([]string, error) {
	words, err := tokenizeWords(text, homeDir(), s)
	if err != nil {
		return nil, err
	}
	return expandWords(words)
}

// withEnv donne des variables à une seule commande (NOM=valeur commande) ; la fonction rendue les retire.
func (s *Shell) withEnv(assigns []assignment) func() {
	type saved struct {
		name, value string
		had         bool
	}
	var old []saved
	for _, a := range assigns {
		v, had := os.LookupEnv(a.name)
		old = append(old, saved{a.name, v, had})
		os.Setenv(a.name, a.value)
	}
	return func() {
		for i := len(old) - 1; i >= 0; i-- {
			if old[i].had {
				os.Setenv(old[i].name, old[i].value)
			} else {
				os.Unsetenv(old[i].name)
			}
		}
	}
}

// psOnly : la ligne est pour PowerShell (une cmdlet que le terminal ne connaît pas, ou $x = …) ;
// elle lui est alors confiée telle quelle, sans développer ses $.
func (s *Shell) psOnly(text string) bool {
	toks, err := tokenize(text, "")
	if err != nil || len(toks) == 0 {
		return false
	}
	name := toks[0]
	if strings.HasPrefix(name, "$") && len(toks) > 1 && (toks[1] == "=" || toks[1] == "+=") {
		return true // $x = Get-Date
	}
	if strings.ContainsAny(name, "$`=") {
		return false
	}
	if _, ok := builtins[strings.ToLower(name)]; ok {
		return false
	}
	if _, err := exec.LookPath(name); err == nil {
		return false
	}
	knows, known := psKnows(name)
	return known && knows
}

// ---------------------------------------------------------------------------
// Développement

var (
	errUnclosedParen = errors.New("parenthèse non fermée après $(")
	errUnclosedBrace = errors.New("accolade non fermée après ${")
	errUnclosedTick  = errors.New("` non fermé")
)

// skipExpansion : à partir d'un $ ou d'un `, l'indice du dernier caractère de $(…), ${…} ou `…`
// (i lui-même pour un simple $ ou $NOM).
func skipExpansion(runes []rune, i int) (int, error) {
	n := len(runes)
	if runes[i] == '`' {
		for j := i + 1; j < n; j++ {
			if runes[j] == '\\' && j+1 < n && runes[j+1] == '`' {
				j++
				continue
			}
			if runes[j] == '`' {
				return j, nil
			}
		}
		return 0, errUnclosedTick
	}
	if i+1 >= n {
		return i, nil
	}
	switch runes[i+1] {
	case '(':
		if j := matchClose(runes, i+1, '(', ')'); j >= 0 {
			return j, nil
		}
		return 0, errUnclosedParen
	case '{':
		if j := matchClose(runes, i+1, '{', '}'); j >= 0 {
			return j, nil
		}
		return 0, errUnclosedBrace
	}
	return i, nil
}

// matchClose : l'indice de la parenthèse (ou accolade) qui ferme celle en open, -1 sinon.
func matchClose(runes []rune, open int, o, c rune) int {
	depth := 0
	var quote rune
	for j := open; j < len(runes); j++ {
		r := runes[j]
		if quote == '\'' {
			if r == '\'' {
				quote = 0
			}
			continue
		}
		if r == '\\' && j+1 < len(runes) && strings.ContainsRune("\"'`$\\", runes[j+1]) {
			j++
			continue
		}
		if quote == '"' {
			if r == '"' {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case o:
			depth++
		case c:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func isNameRune(r rune, first bool) bool {
	return r == '_' || isASCIILetter(r) || (!first && r >= '0' && r <= '9')
}

// expandAt développe ce qui commence en runes[i] ($ ou `) ; ok=false pour un $ ordinaire.
// last : l'indice du dernier caractère consommé.
func (s *Shell) expandAt(runes []rune, i int) (value string, last int, ok bool, err error) {
	n := len(runes)
	if runes[i] == '`' {
		end, err := skipExpansion(runes, i)
		if err != nil {
			return "", i, false, err
		}
		cmd := strings.ReplaceAll(string(runes[i+1:end]), "\\`", "`")
		v, err := s.substitute(cmd)
		return v, end, true, err
	}
	if i+1 >= n {
		return "", i, false, nil
	}
	c := runes[i+1]
	switch {
	case c == '(':
		end, err := skipExpansion(runes, i)
		if err != nil {
			return "", i, false, err
		}
		if i+2 < end && runes[i+2] == '(' && runes[end-1] == ')' { // $(( calcul ))
			v, err := s.arith(string(runes[i+3 : end-1]))
			return strconv.FormatInt(v, 10), end, true, err
		}
		v, err := s.substitute(string(runes[i+2 : end]))
		return v, end, true, err
	case c == '{':
		end, err := skipExpansion(runes, i)
		if err != nil {
			return "", i, false, err
		}
		v, err := s.paramExpand(string(runes[i+2 : end]))
		return v, end, true, err
	case isNameRune(c, true):
		j := i + 1
		for j < n && isNameRune(runes[j], false) {
			j++
		}
		name := string(runes[i+1 : j])
		// $env:NOM, comme dans PowerShell
		if strings.EqualFold(name, "env") && j+1 < n && runes[j] == ':' && isNameRune(runes[j+1], true) {
			k := j + 1
			for k < n && isNameRune(runes[k], false) {
				k++
			}
			return os.Getenv(string(runes[j+1 : k])), k - 1, true, nil
		}
		v, _ := s.getVar(name)
		return v, j - 1, true, nil
	case c >= '0' && c <= '9', strings.ContainsRune("?$!#@*-", c):
		v, _ := s.getVar(string(c))
		return v, i + 1, true, nil
	}
	return "", i, false, nil
}

// expandText développe un texte comme entre guillemets doubles, en retirant les guillemets
// (valeur par défaut de ${…}, calcul $((…)), PS1).
func (s *Shell) expandText(text string) (string, error) {
	runes := []rune(text)
	var b strings.Builder
	var quote rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote == '\'' {
			if r == '\'' {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		switch {
		case r == '\\' && i+1 < len(runes) && strings.ContainsRune("$`\"", runes[i+1]):
			b.WriteRune(runes[i+1])
			i++
		case r == '"':
			if quote == '"' {
				quote = 0
			} else {
				quote = '"'
			}
		case r == '\'' && quote == 0:
			quote = '\''
		case r == '$' || r == '`':
			v, last, ok, err := s.expandAt(runes, i)
			if err != nil {
				return "", err
			}
			if !ok {
				b.WriteRune(r)
				continue
			}
			b.WriteString(v)
			i = last
		default:
			b.WriteRune(r)
		}
	}
	return b.String(), nil
}

// ---------------------------------------------------------------------------
// ${…}

// paramName sépare le nom du reste : « NOM:-x » → « NOM », « :-x ».
func paramName(inner string) (string, string) {
	if inner == "" {
		return "", ""
	}
	r, _ := utf8.DecodeRuneInString(inner)
	switch {
	case isNameRune(r, true):
		i := 0
		for i < len(inner) && isNameRune(rune(inner[i]), false) {
			i++
		}
		return inner[:i], inner[i:]
	case r >= '0' && r <= '9':
		i := 0
		for i < len(inner) && inner[i] >= '0' && inner[i] <= '9' {
			i++
		}
		return inner[:i], inner[i:]
	case strings.ContainsRune("?$!#@*-_", r):
		return inner[:1], inner[1:]
	}
	return "", inner
}

func (s *Shell) paramExpand(inner string) (string, error) {
	bad := fmt.Errorf("${%s} : substitution incorrecte", inner)
	// ${#NOM} : longueur
	if len(inner) > 1 && inner[0] == '#' {
		if name, rest := paramName(inner[1:]); name != "" && rest == "" {
			v, _ := s.getVar(name)
			return strconv.Itoa(utf8.RuneCountInString(v)), nil
		}
	}
	name, rest := paramName(inner)
	if name == "" {
		return "", bad
	}
	v, set := s.getVar(name)
	word := func(w string) (string, error) { return s.expandText(w) }

	for _, op := range []string{":-", ":=", ":+", ":?", "-", "=", "+", "?"} {
		if !strings.HasPrefix(rest, op) {
			continue
		}
		w := rest[len(op):]
		empty := !set || (strings.HasPrefix(op, ":") && v == "")
		switch op[len(op)-1] {
		case '-':
			if empty {
				return word(w)
			}
			return v, nil
		case '=':
			if !empty {
				return v, nil
			}
			nv, err := word(w)
			if err != nil {
				return "", err
			}
			if err := s.setVar(name, nv); err != nil {
				return "", err
			}
			return nv, nil
		case '+':
			if empty {
				return "", nil
			}
			return word(w)
		case '?':
			if !empty {
				return v, nil
			}
			msg, err := word(w)
			if err != nil {
				return "", err
			}
			if msg == "" {
				msg = "paramètre vide ou non défini"
				if op == "?" {
					msg = "paramètre non défini"
				}
			}
			return "", fmt.Errorf("%s : %s", name, msg)
		}
	}

	switch {
	case rest == "":
		return v, nil
	case strings.HasPrefix(rest, "##"), strings.HasPrefix(rest, "#"):
		longest := strings.HasPrefix(rest, "##")
		p := rest[1:]
		if longest {
			p = rest[2:]
		}
		pat, err := word(p)
		if err != nil {
			return "", err
		}
		return trimPattern(v, pat, true, longest), nil
	case strings.HasPrefix(rest, "%"):
		longest := strings.HasPrefix(rest, "%%")
		p := rest[1:]
		if longest {
			p = rest[2:]
		}
		pat, err := word(p)
		if err != nil {
			return "", err
		}
		return trimPattern(v, pat, false, longest), nil
	case strings.HasPrefix(rest, "/"):
		body, mode := rest[1:], byte('1')
		if body != "" && strings.ContainsRune("/#%", rune(body[0])) {
			mode, body = body[0], body[1:]
		}
		p, r, _ := cutUnescaped(body, '/')
		pat, err := word(p)
		if err != nil {
			return "", err
		}
		rep, err := word(r)
		if err != nil {
			return "", err
		}
		return replacePattern(v, pat, rep, mode), nil
	case rest == "^^":
		return strings.ToUpper(v), nil
	case rest == ",,":
		return strings.ToLower(v), nil
	case rest == "^" || rest == ",":
		if v == "" {
			return v, nil
		}
		r, size := utf8.DecodeRuneInString(v)
		if rest == "^" {
			r = unicode.ToUpper(r)
		} else {
			r = unicode.ToLower(r)
		}
		return string(r) + v[size:], nil
	case strings.HasPrefix(rest, ":"):
		offExpr, lenExpr, hasLen := strings.Cut(rest[1:], ":")
		off, err := s.arith(offExpr)
		if err != nil {
			return "", err
		}
		rs := []rune(v)
		total := int64(len(rs))
		if off < 0 {
			off += total
		}
		off = min(max(off, 0), total)
		end := total
		if hasLen {
			l, err := s.arith(lenExpr)
			if err != nil {
				return "", err
			}
			if l < 0 {
				end = total + l
			} else {
				end = off + l
			}
			end = min(max(end, off), total)
		}
		return string(rs[off:end]), nil
	}
	return "", bad
}

// cutUnescaped coupe au premier sep qui n'est pas précédé d'un \.
func cutUnescaped(s string, sep byte) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func patternItems(pat string) []patItem {
	rs := []rune(pat)
	return compilePattern(word{r: rs, q: make([]bool, len(rs))}).items
}

// trimPattern : ${v#motif} (prefix, court), ${v##motif}, ${v%motif}, ${v%%motif}.
func trimPattern(v, pat string, prefix, longest bool) string {
	items := patternItems(pat)
	rs := []rune(v)
	n := len(rs)
	for k := 0; k <= n; k++ {
		i := k // longueur retirée, du plus court au plus long
		if longest {
			i = n - k
		}
		if prefix && matchItems(items, rs[:i], false) {
			return string(rs[i:])
		}
		if !prefix && matchItems(items, rs[n-i:], false) {
			return string(rs[:n-i])
		}
	}
	return v
}

// replacePattern : ${v/motif/par} (mode '1'), ${v//…} ('/'), ${v/#…} (début), ${v/%…} (fin).
func replacePattern(v, pat, rep string, mode byte) string {
	items := patternItems(pat)
	rs := []rune(v)
	n := len(rs)
	switch mode {
	case '#':
		for i := n; i >= 0; i-- {
			if matchItems(items, rs[:i], false) {
				return rep + string(rs[i:])
			}
		}
		return v
	case '%':
		for i := 0; i <= n; i++ {
			if matchItems(items, rs[i:], false) {
				return string(rs[:i]) + rep
			}
		}
		return v
	}
	if pat == "" {
		return v
	}
	var b strings.Builder
	for i := 0; i < n; {
		end := -1
		for j := n; j > i; j-- {
			if matchItems(items, rs[i:j], false) {
				end = j
				break
			}
		}
		if end < 0 {
			b.WriteRune(rs[i])
			i++
			continue
		}
		b.WriteString(rep)
		i = end
		if mode != '/' {
			b.WriteString(string(rs[i:]))
			return b.String()
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// $(commande)

// substitute lance une commande dans le terminal et renvoie sa sortie, sans les retours à la ligne
// de la fin. Comme dans bash, ce qu'elle change (dossier, variables, exit) ne touche pas la session.
func (s *Shell) substitute(cmd string) (string, error) {
	if s.substDepth >= 16 {
		return "", errors.New("trop de $(…) imbriqués")
	}
	var buf bytes.Buffer
	oldOut, oldExiting, oldPrev, oldLastArg := s.out, s.exiting, s.prevDir, s.lastArg
	cwd, _ := os.Getwd()
	env := os.Environ()
	vars := make(map[string]string, len(s.vars))
	for k, v := range s.vars {
		vars[k] = v
	}
	ps1, secondsAt := s.ps1, s.secondsAt

	s.out = &lockedWriter{mu: &sync.Mutex{}, w: &buf}
	s.substDepth++
	code := s.RunLine(cmd)
	s.substDepth--

	s.out, s.exiting, s.prevDir, s.lastArg = oldOut, oldExiting, oldPrev, oldLastArg
	if cwd != "" {
		_ = os.Chdir(cwd)
	}
	restoreEnv(env)
	s.vars, s.ps1, s.secondsAt = vars, ps1, secondsAt
	s.substStatus = code
	out := strings.ReplaceAll(buf.String(), "\r\n", "\n")
	return strings.TrimRight(out, "\n"), nil
}

// restoreEnv remet l'environnement tel qu'il était.
func restoreEnv(snapshot []string) {
	want := map[string]string{}
	for _, kv := range snapshot {
		if strings.HasPrefix(kv, "=") {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		want[k] = v
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "=") {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		if w, ok := want[k]; !ok {
			os.Unsetenv(k)
		} else if w != v {
			os.Setenv(k, w)
		}
		delete(want, k)
	}
	for k, v := range want {
		os.Setenv(k, v)
	}
}

// ---------------------------------------------------------------------------
// $(( calcul ))

var errDivZero = errors.New("division par zéro")

// arith calcule une expression entière : + - * / % ** ( ) < <= > >= == != && || ! ~ & | ^ << >> ?:
// Les noms de variables y valent leur valeur (0 si vide).
func (s *Shell) arith(expr string) (int64, error) {
	text, err := s.expandText(expr)
	if err != nil {
		return 0, err
	}
	toks, err := arithTokens(text)
	if err != nil {
		return 0, err
	}
	p := &arithParser{s: s, toks: toks}
	if len(toks) == 0 {
		return 0, nil
	}
	v, err := p.ternary()
	if err != nil {
		return 0, err
	}
	if p.pos < len(p.toks) {
		return 0, fmt.Errorf("$((%s)) : erreur de syntaxe près de « %s »", strings.TrimSpace(expr), p.toks[p.pos])
	}
	return v, nil
}

var arithOps = []string{"**", "<<", ">>", "<=", ">=", "==", "!=", "&&", "||",
	"+", "-", "*", "/", "%", "(", ")", "<", ">", "!", "~", "&", "|", "^", "?", ":"}

func arithTokens(text string) ([]string, error) {
	var toks []string
	for i := 0; i < len(text); {
		c := text[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || isNameRune(rune(c), true):
			j := i
			for j < len(text) && (isNameRune(rune(text[j]), false)) {
				j++
			}
			toks = append(toks, text[i:j])
			i = j
		default:
			found := false
			for _, op := range arithOps {
				if strings.HasPrefix(text[i:], op) {
					if len(op) == 1 && strings.Contains("+-*/%&|^", op) && i+1 < len(text) && text[i+1] == '=' {
						return nil, errors.New("$((…)) : les affectations (=, +=, ++) ne sont pas gérées")
					}
					toks = append(toks, op)
					i += len(op)
					found = true
					break
				}
			}
			if !found {
				if c == '=' {
					return nil, errors.New("$((…)) : les affectations (=, +=, ++) ne sont pas gérées")
				}
				return nil, fmt.Errorf("$((…)) : caractère inattendu « %c »", c)
			}
		}
	}
	return toks, nil
}

type arithParser struct {
	s     *Shell
	toks  []string
	pos   int
	depth int
}

func (p *arithParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *arithParser) accept(ops ...string) (string, bool) {
	t := p.peek()
	for _, o := range ops {
		if t == o {
			p.pos++
			return o, true
		}
	}
	return "", false
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (p *arithParser) ternary() (int64, error) {
	c, err := p.binary(0)
	if err != nil {
		return 0, err
	}
	if _, ok := p.accept("?"); !ok {
		return c, nil
	}
	a, err := p.ternary()
	if err != nil {
		return 0, err
	}
	if _, ok := p.accept(":"); !ok {
		return 0, errors.New("$((…)) : « : » manquant après « ? »")
	}
	b, err := p.ternary()
	if err != nil {
		return 0, err
	}
	if c != 0 {
		return a, nil
	}
	return b, nil
}

// niveaux de priorité, du plus faible au plus fort
var arithLevels = [][]string{{"||"}, {"&&"}, {"|"}, {"^"}, {"&"}, {"==", "!="}, {"<", "<=", ">", ">="},
	{"<<", ">>"}, {"+", "-"}, {"*", "/", "%"}}

func (p *arithParser) binary(level int) (int64, error) {
	if level == len(arithLevels) {
		return p.power()
	}
	a, err := p.binary(level + 1)
	if err != nil {
		return 0, err
	}
	for {
		op, ok := p.accept(arithLevels[level]...)
		if !ok {
			return a, nil
		}
		b, err := p.binary(level + 1)
		if err != nil {
			return 0, err
		}
		switch op {
		case "||":
			a = b2i(a != 0 || b != 0)
		case "&&":
			a = b2i(a != 0 && b != 0)
		case "|":
			a |= b
		case "^":
			a ^= b
		case "&":
			a &= b
		case "==":
			a = b2i(a == b)
		case "!=":
			a = b2i(a != b)
		case "<":
			a = b2i(a < b)
		case "<=":
			a = b2i(a <= b)
		case ">":
			a = b2i(a > b)
		case ">=":
			a = b2i(a >= b)
		case "<<":
			a <<= uint64(b & 63)
		case ">>":
			a >>= uint64(b & 63)
		case "+":
			a += b
		case "-":
			a -= b
		case "*":
			a *= b
		case "/", "%":
			if b == 0 {
				return 0, errDivZero
			}
			if op == "/" {
				a /= b
			} else {
				a %= b
			}
		}
	}
}

func (p *arithParser) power() (int64, error) {
	a, err := p.unary()
	if err != nil {
		return 0, err
	}
	if _, ok := p.accept("**"); !ok {
		return a, nil
	}
	b, err := p.power()
	if err != nil {
		return 0, err
	}
	if b < 0 {
		return 0, errors.New("$((…)) : exposant négatif")
	}
	r := int64(1)
	for ; b > 0; b-- {
		r *= a
	}
	return r, nil
}

func (p *arithParser) unary() (int64, error) {
	if op, ok := p.accept("-", "+", "!", "~"); ok {
		v, err := p.unary()
		if err != nil {
			return 0, err
		}
		switch op {
		case "-":
			return -v, nil
		case "!":
			return b2i(v == 0), nil
		case "~":
			return ^v, nil
		}
		return v, nil
	}
	return p.primary()
}

func (p *arithParser) primary() (int64, error) {
	t := p.peek()
	switch {
	case t == "":
		return 0, errors.New("$((…)) : expression incomplète")
	case t == "(":
		p.pos++
		v, err := p.ternary()
		if err != nil {
			return 0, err
		}
		if _, ok := p.accept(")"); !ok {
			return 0, errors.New("$((…)) : « ) » manquante")
		}
		return v, nil
	case t[0] >= '0' && t[0] <= '9':
		p.pos++
		v, err := strconv.ParseInt(t, 0, 64)
		if err != nil {
			return 0, fmt.Errorf("$((…)) : nombre incorrect « %s »", t)
		}
		return v, nil
	case isNameRune(rune(t[0]), true):
		p.pos++
		v, _ := p.s.getVar(t)
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, nil
		}
		if n, err := strconv.ParseInt(v, 0, 64); err == nil {
			return n, nil
		}
		if p.depth >= 8 {
			return 0, fmt.Errorf("$((…)) : %s : valeur non numérique « %s »", t, v)
		}
		// comme bash, la valeur peut être elle-même une expression
		toks, err := arithTokens(v)
		if err != nil {
			return 0, fmt.Errorf("$((…)) : %s : valeur non numérique « %s »", t, v)
		}
		sub := &arithParser{s: p.s, toks: toks, depth: p.depth + 1}
		n, err := sub.ternary()
		if err != nil || sub.pos < len(toks) {
			return 0, fmt.Errorf("$((…)) : %s : valeur non numérique « %s »", t, v)
		}
		return n, nil
	}
	return 0, fmt.Errorf("$((…)) : erreur de syntaxe près de « %s »", t)
}

// ---------------------------------------------------------------------------
// PS1 à la manière de bash

// ps1ToFormat traduit les séquences de bash (\u \h \w \W \$ \t \n \[ \] \e…) en format du terminal.
// Un PS1 déjà écrit avec les éléments du terminal ({user}, {dir}…) passe tel quel.
func ps1ToFormat(ps1 string) string {
	var b strings.Builder
	for i := 0; i < len(ps1); i++ {
		c := ps1[i]
		if c != '\\' || i+1 == len(ps1) {
			b.WriteByte(c)
			continue
		}
		i++
		switch ps1[i] {
		case 'u':
			b.WriteString("{user}")
		case 'h', 'H':
			b.WriteString("{host}")
		case 'w':
			b.WriteString("{dir}")
		case 'W':
			b.WriteString("{dossier}")
		case '$':
			b.WriteString("{signe}")
		case 't', 'A', '@', 'T':
			b.WriteString("{heure}")
		case 'n':
			b.WriteString("{nl}")
		case 's':
			b.WriteString("terminal")
		case 'v', 'V':
			b.WriteString(apt.Version)
		case '[', ']':
		case 'e':
			b.WriteByte(0x1b)
		case '0':
			if strings.HasPrefix(ps1[i:], "033") {
				b.WriteByte(0x1b)
				i += 2
			} else {
				b.WriteString(`\0`)
			}
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(ps1[i])
		}
	}
	return b.String()
}
