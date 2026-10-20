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
	special bool // contient | < > $ & ` : la ligne est confiée à PowerShell
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
		if quote != 0 {
			b.WriteRune(r)
			if r == '\\' && quote == '"' && i+1 < len(runes) && runes[i+1] == '"' {
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
		case '<', '>', '$', '`':
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

// tokenize découpe une commande simple en arguments, retire les guillemets et développe ~.
func tokenize(text, home string) ([]string, error) {
	var (
		args    []string
		b       strings.Builder
		inToken bool
		quoted  bool
		quote   rune
	)
	push := func() {
		if !inToken {
			return
		}
		tok := b.String()
		if !quoted {
			tok = expandHome(tok, home)
		}
		args = append(args, tok)
		b.Reset()
		inToken, quoted = false, false
	}

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			switch {
			case r == quote:
				quote = 0
			case r == '\\' && quote == '"' && i+1 < len(runes) && runes[i+1] == '"':
				b.WriteRune('"')
				i++
			default:
				b.WriteRune(r)
			}
			continue
		}
		switch r {
		case ' ', '\t':
			push()
		case '\'', '"':
			quote = r
			inToken, quoted = true, true
		default:
			inToken = true
			b.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, errUnclosedQuote
	}
	push()
	return args, nil
}

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
		if a == "" || strings.ContainsAny(a, " \t\"';&|<>$`") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
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
