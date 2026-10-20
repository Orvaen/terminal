package shell

import (
	"errors"
	"os"
	"strings"
)

type chainOp int

const (
	opEnd chainOp = iota // dernière commande de la ligne
	opSeq                // ;
	opAnd                // &&
	opOr                 // ||
)

// segment est une commande simple d'une ligne, avec l'opérateur qui la suit.
type segment struct {
	text    string
	special bool // contient | < > ou & : tubes et redirections (pipeline.go)
	op      chainOp
}

var errUnclosedQuote = errors.New("guillemet non fermé")

// splitChain découpe une ligne selon ; && et || (en dehors des guillemets).
func splitChain(line string) ([]segment, error) {
	var (
		segs    []segment
		b       strings.Builder
		special bool
		quote   rune
	)
	flush := func(op chainOp, symbol string) error {
		text := strings.TrimSpace(b.String())
		b.Reset()
		if text == "" {
			if op == opAnd || op == opOr {
				return errors.New("erreur de syntaxe près de « " + symbol + " »")
			}
			special = false
			return nil
		}
		segs = append(segs, segment{text: text, special: special, op: op})
		special = false
		return nil
	}

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// $(…), ${…}, `…` : recopiés d'un bloc (leurs ; | && ne coupent pas la ligne).
		if (r == '$' || r == '`') && quote != '\'' {
			end, err := skipExpansion(runes, i)
			if err != nil {
				return nil, err
			}
			b.WriteString(string(runes[i : end+1]))
			i = end
			continue
		}
		if r == '\\' && quote == 0 && i+1 < len(runes) && (runes[i+1] == '$' || runes[i+1] == '`') {
			b.WriteRune(r)
			b.WriteRune(runes[i+1])
			i++
			continue
		}
		if quote != 0 {
			b.WriteRune(r)
			if r == '\\' && quote == '"' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '$' || runes[i+1] == '`') {
				b.WriteRune(runes[i+1])
				i++
			} else if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			b.WriteRune(r)
		case ';':
			if err := flush(opSeq, ";"); err != nil {
				return nil, err
			}
		case '&':
			if i+1 < len(runes) && runes[i+1] == '&' {
				if err := flush(opAnd, "&&"); err != nil {
					return nil, err
				}
				i++
			} else {
				special = true
				b.WriteRune(r)
			}
		case '|':
			if i+1 < len(runes) && runes[i+1] == '|' {
				if err := flush(opOr, "||"); err != nil {
					return nil, err
				}
				i++
			} else {
				special = true
				b.WriteRune(r)
			}
		case '<', '>':
			special = true
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, errUnclosedQuote
	}
	if err := flush(opEnd, ""); err != nil {
		return nil, err
	}
	// Une ligne qui finit par && ou || est incomplète.
	if n := len(segs); n > 0 && (segs[n-1].op == opAnd || segs[n-1].op == opOr) {
		return nil, errors.New("erreur de syntaxe : commande manquante en fin de ligne")
	}
	return segs, nil
}

// word : un argument, avec pour chaque caractère s'il était entre guillemets (et donc protégé :
// pas de joker ni d'accolades).
type word struct {
	r      []rune
	q      []bool
	assign bool // NOM=valeur : ni découpage, ni accolades, ni jokers
}

func (w word) String() string { return string(w.r) }

// hasUnquoted indique si l'un des caractères de set apparaît hors guillemets.
func (w word) hasUnquoted(set string) bool {
	for i, r := range w.r {
		if !w.q[i] && strings.ContainsRune(set, r) {
			return true
		}
	}
	return false
}

// tokenize découpe une commande simple en arguments, retire les guillemets et développe ~
// (sans les variables : voir Shell.expandCommand).
func tokenize(text, home string) ([]string, error) {
	words, err := tokenizeWords(text, home, nil)
	if err != nil {
		return nil, err
	}
	args := make([]string, len(words))
	for i, w := range words {
		args[i] = w.String()
	}
	return args, nil
}

// declBuiltins : commandes dont les arguments NOM=valeur sont des affectations (pas de découpage
// ni de jokers dans la valeur), comme dans bash : export CHEMIN=$HOME/mes docs.
var declBuiltins = map[string]bool{"export": true, "readonly": true, "declare": true, "local": true, "typeset": true}

// tokenizeWords découpe comme tokenize, en gardant la trace des parties entre guillemets.
// Avec x non nil, développe aussi $VAR, ${…}, $(…), `…` et $((…)) (voir vars.go).
func tokenizeWords(text, home string, x *Shell) ([]word, error) {
	var (
		words   []word
		cur     word
		inToken bool
		quote   rune
		cmdSeen bool // un mot qui n'est pas une affectation a déjà été lu
		decl    bool // la commande est export, readonly…
	)
	homeWord := func() word {
		hr := []rune(home)
		hq := make([]bool, len(hr))
		for i := range hq {
			hq[i] = true // le chemin du dossier perso n'est jamais un joker
		}
		return word{r: hr, q: hq}
	}
	push := func() {
		if !inToken {
			return
		}
		// ~ ou ~/… hors guillemets : le dossier personnel
		if home != "" && len(cur.r) > 0 && cur.r[0] == '~' && !cur.q[0] &&
			(len(cur.r) == 1 || cur.r[1] == '/' || cur.r[1] == '\\') {
			h := homeWord()
			cur = word{r: append(h.r, cur.r[1:]...), q: append(h.q, cur.q[1:]...), assign: cur.assign}
		}
		// Dans une affectation, ~ après = ou : aussi (PATH=$PATH:~/bin).
		if cur.assign && home != "" {
			var nw word
			nw.assign = true
			for i := 0; i < len(cur.r); i++ {
				r := cur.r[i]
				if r == '~' && !cur.q[i] && i > 0 && !cur.q[i-1] && (cur.r[i-1] == '=' || cur.r[i-1] == ':') &&
					(i+1 == len(cur.r) || cur.r[i+1] == '/' || cur.r[i+1] == '\\' || cur.r[i+1] == ':') {
					h := homeWord()
					nw.r, nw.q = append(nw.r, h.r...), append(nw.q, h.q...)
					continue
				}
				nw.r, nw.q = append(nw.r, r), append(nw.q, cur.q[i])
			}
			cur = nw
		}
		if !cur.assign {
			if !cmdSeen {
				decl = declBuiltins[strings.ToLower(cur.String())]
			}
			cmdSeen = true
		}
		words = append(words, cur)
		cur = word{}
		inToken = false
	}
	add := func(r rune, quoted bool) {
		cur.r = append(cur.r, r)
		cur.q = append(cur.q, quoted)
		inToken = true
	}
	// addValue ajoute le résultat d'un développement. Hors guillemets, il est découpé en mots
	// (aux espaces, selon IFS) et ses jokers restent actifs, comme dans bash ; ses accolades et
	// son ~ ne sont jamais développés.
	addValue := func(v string, quoted bool) {
		if quoted || cur.assign {
			for _, r := range v {
				add(r, true)
			}
			if quoted {
				inToken = true // "$VIDE" reste un argument vide
			}
			return
		}
		ifs := x.ifs()
		for _, r := range v {
			if strings.ContainsRune(ifs, r) {
				push()
				continue
			}
			add(r, r == '{' || r == '}' || r == '~')
		}
	}
	expand := func(runes []rune, i int, quoted bool) (int, bool, error) {
		v, last, ok, err := x.expandAt(runes, i)
		if err != nil || !ok {
			return i, ok, err
		}
		addValue(v, quoted)
		return last, true, nil
	}

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch quote {
		case '\'':
			if r == '\'' {
				quote = 0
			} else {
				add(r, true)
			}
			continue
		case '"':
			switch {
			case r == '"':
				quote = 0
			case r == '\\' && i+1 < len(runes) && strings.ContainsRune("\"$`", runes[i+1]):
				add(runes[i+1], true)
				i++
			case (r == '$' || r == '`') && x != nil:
				last, ok, err := expand(runes, i, true)
				if err != nil {
					return nil, err
				}
				if !ok {
					add(r, true)
				}
				i = last
			default:
				add(r, true)
			}
			continue
		}
		switch {
		case r == ' ' || r == '\t':
			push()
		case r == '\'' || r == '"':
			quote = r
			inToken = true
		case r == '\\' && i+1 < len(runes) && (runes[i+1] == '$' || runes[i+1] == '`'):
			add(runes[i+1], true) // \$ : un vrai dollar
			i++
		case (r == '$' || r == '`') && x != nil:
			last, ok, err := expand(runes, i, false)
			if err != nil {
				return nil, err
			}
			if !ok {
				add(r, false)
			}
			i = last
		case r == '=' && !cur.assign && (!cmdSeen || decl) && isName(cur):
			cur.assign = true
			add(r, false)
		default:
			add(r, false)
		}
	}
	if quote != 0 {
		return nil, errUnclosedQuote
	}
	push()
	return words, nil
}

// isName : le mot en cours est un nom de variable (lettres, chiffres, _), sans guillemets.
func isName(w word) bool {
	if len(w.r) == 0 {
		return false
	}
	for i, r := range w.r {
		if w.q[i] || !(r == '_' || isASCIILetter(r) || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func isASCIILetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

func expandHome(tok, home string) string {
	if home == "" || !strings.HasPrefix(tok, "~") {
		return tok
	}
	if len(tok) == 1 || tok[1] == '/' || tok[1] == '\\' {
		return home + tok[1:]
	}
	return tok
}

// joinArgs reconstruit une ligne à partir d'arguments déjà découpés.
func joinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		// Entre '…' : ni $, ni jokers, ni accolades ne sont redéveloppés.
		if a == "" || strings.ContainsAny(a, " \t\"';&|<>$`*?[{~") {
			a = "'" + strings.ReplaceAll(a, "'", `'"'"'`) + "'"
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

// convertPath accepte les chemins façon Git Bash (/c/Users/...) et les slashs sous Windows.
func convertPath(p, goos string) string {
	if goos != "windows" {
		return p
	}
	if len(p) >= 2 && p[0] == '/' && isLetter(p[1]) && (len(p) == 2 || p[2] == '/') {
		rest := p[2:]
		if rest == "" {
			rest = "/"
		}
		p = strings.ToUpper(p[1:2]) + ":" + rest
	}
	return strings.ReplaceAll(p, "/", `\`)
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

// displayPath affiche le dossier courant façon Linux : ~ pour le dossier perso, / comme séparateur.
func displayPath(cwd, home string) string {
	if home != "" && len(cwd) >= len(home) && strings.EqualFold(cwd[:len(home)], home) {
		rest := cwd[len(home):]
		if rest == "" || rest[0] == '\\' || rest[0] == '/' {
			cwd = "~" + rest
		}
	}
	return strings.ReplaceAll(cwd, `\`, "/")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}
