// Package req exécute des requêtes HTTP décrites dans un fichier .http
// (format de l'extension REST Client de VS Code), avec des vérifications en français.
package req

import (
	"fmt"
	"regexp"
	"strings"
)

// File est un fichier .http analysé.
type File struct {
	Path     string
	Vars     []Var // variables déclarées avant la première requête
	Requests []*Request
}

// Var : @nom = valeur
type Var struct {
	Name, Value string
	Line        int
}

// Request : une requête, de « ### titre » jusqu'à la suivante.
type Request struct {
	Title    string
	Line     int // ligne de « METHODE URL »
	Vars     []Var
	Method   string
	URL      string
	Headers  []Header
	Body     string
	BodyFile string // corps lu dans un fichier : « < ./donnees.json »
	Checks   []Check
}

// Header : un en-tête HTTP.
type Header struct{ Name, Value string }

// Check : « > vérification » ou « >> capture ».
type Check struct {
	Text    string // sans le > ni le >>
	Capture bool
	Line    int
}

// Name : le titre de la requête, ou « MÉTHODE URL » à défaut.
func (r *Request) Name() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Method + " " + r.URL
}

var (
	varLine = regexp.MustCompile(`^@([A-Za-z_][\w.-]*)\s*=\s*(.*)$`)
	methods = map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true,
	}
)

// ParseError signale une ligne incompréhensible.
type ParseError struct {
	Path string
	Line int
	Msg  string
}

func (e *ParseError) Error() string { return fmt.Sprintf("%s:%d : %s", e.Path, e.Line, e.Msg) }

func isComment(t string) bool {
	return (strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "###")) || strings.HasPrefix(t, "//")
}

// Parse analyse le contenu d'un fichier .http.
func Parse(path, content string) (*File, error) {
	f := &File{Path: path}
	var (
		cur       *Request
		state     string // "debut", "entetes", "corps", "verifs"
		bodyLines []string
	)
	fail := func(line int, format string, a ...any) error {
		return &ParseError{Path: path, Line: line, Msg: fmt.Sprintf(format, a...)}
	}
	finish := func() error {
		if cur == nil {
			return nil
		}
		if cur.Method == "" {
			if cur.Title != "" || len(cur.Vars) > 0 {
				return fail(cur.Line, "la requête « %s » n'a pas de ligne « MÉTHODE URL »", cur.Title)
			}
			cur = nil
			return nil
		}
		// lignes vides et commentaires en fin de corps ignorés
		for len(bodyLines) > 0 {
			t := strings.TrimSpace(bodyLines[len(bodyLines)-1])
			if t != "" && !isComment(t) {
				break
			}
			bodyLines = bodyLines[:len(bodyLines)-1]
		}
		body := strings.Join(bodyLines, "\n")
		if t := strings.TrimSpace(body); len(bodyLines) == 1 && strings.HasPrefix(t, "< ") {
			cur.BodyFile = strings.TrimSpace(t[2:])
		} else {
			cur.Body = body
		}
		f.Requests = append(f.Requests, cur)
		cur, bodyLines = nil, nil
		return nil
	}
	startRequest := func(t string, n int) error {
		fields := strings.Fields(t)
		switch {
		case methods[strings.ToUpper(fields[0])] && len(fields) >= 2:
			cur.Method, cur.URL = strings.ToUpper(fields[0]), fields[1]
		case methods[strings.ToUpper(fields[0])]:
			return fail(n, "il manque l'adresse après %s", fields[0])
		case strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") || strings.HasPrefix(t, "{{"):
			cur.Method, cur.URL = "GET", fields[0]
		default:
			return fail(n, "« %s » n'est pas une requête : écris par exemple  GET https://exemple.com", t)
		}
		cur.Line = n
		state = "entetes"
		return nil
	}

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i, raw := range lines {
		n := i + 1
		line := strings.TrimRight(raw, " \t\r")
		t := strings.TrimSpace(line)

		if strings.HasPrefix(t, "###") {
			if err := finish(); err != nil {
				return nil, err
			}
			cur = &Request{Title: strings.TrimSpace(strings.TrimLeft(t, "#")), Line: n}
			state = "debut"
			continue
		}

		if cur == nil { // avant la première requête
			switch {
			case t == "" || isComment(t):
			case varLine.MatchString(t):
				m := varLine.FindStringSubmatch(t)
				f.Vars = append(f.Vars, Var{m[1], strings.TrimSpace(m[2]), n})
			default: // requête sans « ### »
				cur = &Request{Line: n}
				if err := startRequest(t, n); err != nil {
					return nil, err
				}
			}
			continue
		}

		switch state {
		case "debut":
			switch {
			case t == "" || isComment(t):
			case varLine.MatchString(t):
				m := varLine.FindStringSubmatch(t)
				cur.Vars = append(cur.Vars, Var{m[1], strings.TrimSpace(m[2]), n})
			default:
				if err := startRequest(t, n); err != nil {
					return nil, err
				}
			}
		case "entetes":
			switch {
			case t == "":
				state = "corps"
			case strings.HasPrefix(t, ">"):
				cur.Checks = append(cur.Checks, parseCheck(t, n))
				state = "verifs"
			case isComment(t):
			default:
				name, value, ok := strings.Cut(t, ":")
				if !ok || strings.ContainsAny(strings.TrimSpace(name), " \t") {
					return nil, fail(n, "en-tête invalide « %s » : écris  Nom: valeur  (et laisse une ligne vide avant le corps)", t)
				}
				cur.Headers = append(cur.Headers, Header{strings.TrimSpace(name), strings.TrimSpace(value)})
			}
		case "corps":
			switch {
			case strings.HasPrefix(t, ">"):
				cur.Checks = append(cur.Checks, parseCheck(t, n))
				state = "verifs"
			case len(bodyLines) == 0 && (t == "" || isComment(t)):
				// lignes vides et commentaires avant le corps
			default:
				bodyLines = append(bodyLines, line)
			}
		case "verifs":
			switch {
			case t == "" || isComment(t):
			case strings.HasPrefix(t, ">"):
				cur.Checks = append(cur.Checks, parseCheck(t, n))
			default:
				return nil, fail(n, "« %s » : après les vérifications (>), commence une nouvelle requête avec ###", t)
			}
		}
	}
	if err := finish(); err != nil {
		return nil, err
	}
	return f, nil
}

func parseCheck(t string, n int) Check {
	if strings.HasPrefix(t, ">>") {
		return Check{Text: strings.TrimSpace(t[2:]), Capture: true, Line: n}
	}
	return Check{Text: strings.TrimSpace(t[1:]), Line: n}
}
