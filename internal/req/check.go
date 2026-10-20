package req

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Response : ce que les vérifications peuvent examiner.
type Response struct {
	Status   int
	Header   http.Header
	Body     []byte
	Duration time.Duration

	parsed    any
	parsedErr error
	parsedOK  bool
}

func (r *Response) json() (any, error) {
	if !r.parsedOK {
		r.parsedOK = true
		r.parsedErr = json.Unmarshal(r.Body, &r.parsed)
		if r.parsedErr != nil {
			r.parsedErr = fmt.Errorf("la réponse n'est pas du JSON")
		}
	}
	return r.parsed, r.parsedErr
}

// value renvoie la valeur désignée : statut, temps, corps, en-tête X, json.chemin.
func (r *Response) value(subject string) (any, bool, error) {
	low := strings.ToLower(subject)
	switch {
	case low == "statut" || low == "status":
		return float64(r.Status), true, nil
	case low == "temps" || low == "time" || low == "durée" || low == "duree":
		return float64(r.Duration.Milliseconds()), true, nil
	case low == "corps" || low == "body":
		return string(r.Body), true, nil
	case low == "json" || strings.HasPrefix(low, "json.") || strings.HasPrefix(low, "json["):
		root, err := r.json()
		if err != nil {
			return nil, false, err
		}
		v, found, err := jsonPath(root, strings.TrimPrefix(subject[4:], "."))
		return v, found, err
	}
	for _, p := range []string{"en-tête ", "entête ", "en-tete ", "entete ", "header "} {
		if strings.HasPrefix(low, p) {
			name := strings.TrimSpace(subject[len(p):])
			vals := r.Header.Values(name)
			if len(vals) == 0 {
				return nil, false, nil
			}
			return strings.Join(vals, ", "), true, nil
		}
	}
	return nil, false, fmt.Errorf("je ne connais pas « %s » : utilise statut, temps, corps, en-tête Nom ou json.chemin", subject)
}

// jsonPath suit un chemin comme  users[0].nom  ou  items.length.
func jsonPath(v any, path string) (any, bool, error) {
	if path == "" {
		return v, true, nil
	}
	for path != "" {
		var key string
		switch {
		case strings.HasPrefix(path, "["):
			end := strings.Index(path, "]")
			if end < 0 {
				return nil, false, fmt.Errorf("crochet non fermé dans le chemin")
			}
			idx, err := strconv.Atoi(strings.TrimSpace(path[1:end]))
			if err != nil {
				return nil, false, fmt.Errorf("index invalide « %s »", path[1:end])
			}
			arr, ok := v.([]any)
			if !ok {
				return nil, false, nil
			}
			if idx < 0 {
				idx += len(arr)
			}
			if idx < 0 || idx >= len(arr) {
				return nil, false, nil
			}
			v = arr[idx]
			path = strings.TrimPrefix(path[end+1:], ".")
			continue
		default:
			end := strings.IndexAny(path, ".[")
			if end < 0 {
				key, path = path, ""
			} else {
				key, path = path[:end], strings.TrimPrefix(path[end:], ".")
			}
		}
		switch cur := v.(type) {
		case map[string]any:
			next, ok := cur[key]
			if !ok {
				if key == "length" || key == "taille" {
					v = float64(len(cur))
					continue
				}
				return nil, false, nil
			}
			v = next
		case []any:
			if key != "length" && key != "taille" {
				return nil, false, nil
			}
			v = float64(len(cur))
		case string:
			if key != "length" && key != "taille" {
				return nil, false, nil
			}
			v = float64(len([]rune(cur)))
		default:
			return nil, false, nil
		}
	}
	return v, true, nil
}

var operators = []string{"==", "!=", "<=", ">=", "<", ">", "ne contient pas", "contient", "contains", "existe", "exists", "absent"}

// evaluate exécute une vérification ; renvoie ok et, en cas d'échec, une explication.
func evaluate(text string, r *Response) (bool, string, error) {
	t := strings.TrimSpace(text)
	low := strings.ToLower(t)

	// raccourci : « statut 200 », « statut 2xx »
	for _, p := range []string{"statut ", "status "} {
		if strings.HasPrefix(low, p) {
			want := strings.TrimSpace(t[len(p):])
			if !strings.ContainsAny(want, "=<>!") {
				return checkStatus(want, r.Status)
			}
		}
	}

	subject, op, operand := splitCheck(t)
	if op == "" {
		return false, "", fmt.Errorf("vérification incomplète : écris par exemple  > statut 200  ou  > json.id == 1")
	}
	got, found, err := r.value(subject)
	if err != nil {
		return false, "", err
	}
	switch op {
	case "existe", "exists":
		return found, fmt.Sprintf("%s est absent", subject), nil
	case "absent":
		return !found, fmt.Sprintf("%s est présent (%s)", subject, show(got)), nil
	}
	if !found {
		return false, fmt.Sprintf("%s est absent", subject), nil
	}
	want := parseOperand(operand)
	switch op {
	case "==":
		return equal(got, want), "obtenu " + show(got), nil
	case "!=":
		return !equal(got, want), "obtenu " + show(got), nil
	case "contient", "contains", "ne contient pas":
		in := contains(got, want)
		if op == "ne contient pas" {
			return !in, "obtenu " + show(got), nil
		}
		return in, "obtenu " + show(got), nil
	default: // comparaisons de nombres
		a, okA := toNumber(got)
		b, okB := toNumber(want)
		if !okA || !okB {
			return false, "", fmt.Errorf("« %s » compare des nombres, obtenu %s", op, show(got))
		}
		ok := map[string]bool{"<": a < b, ">": a > b, "<=": a <= b, ">=": a >= b}[op]
		return ok, "obtenu " + show(got), nil
	}
}

func checkStatus(want string, got int) (bool, string, error) {
	w := strings.ToLower(want)
	msg := fmt.Sprintf("obtenu %d", got)
	if len(w) == 3 && strings.HasSuffix(w, "xx") && w[0] >= '1' && w[0] <= '5' {
		return got/100 == int(w[0]-'0'), msg, nil
	}
	n, err := strconv.Atoi(w)
	if err != nil {
		return false, "", fmt.Errorf("statut attendu invalide « %s » (exemple : 200 ou 2xx)", want)
	}
	return got == n, msg, nil
}

// splitCheck sépare « sujet opérateur valeur » (l'opérateur le plus à gauche gagne).
func splitCheck(t string) (subject, op, operand string) {
	low := strings.ToLower(t)
	best := -1
	for _, o := range operators {
		search := o
		if o[0] >= 'a' { // mots : entourés d'espaces (ou en fin de ligne)
			search = " " + o
		}
		i := strings.Index(low, search)
		if i < 0 {
			continue
		}
		end := i + len(search)
		if o[0] >= 'a' && end < len(low) && low[end] != ' ' {
			continue
		}
		if best < 0 || i < best || (i == best && len(search) > len(op)) {
			best, op = i, o
		}
	}
	if best < 0 {
		return t, "", ""
	}
	sep := op
	if op[0] >= 'a' {
		sep = " " + op
	}
	return strings.TrimSpace(t[:best]), op, strings.TrimSpace(t[best+len(sep):])
}

// parseOperand lit une valeur JSON (nombre, "texte", true, null, [..]) ou un mot nu (texte).
func parseOperand(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

func equal(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	// "200" == 200, en-tête comparé à un nombre, etc.
	if na, ok := toNumber(a); ok {
		if nb, ok := toNumber(b); ok {
			return na == nb
		}
	}
	return show(a) == show(b) || fmt.Sprint(a) == fmt.Sprint(b)
}

func contains(got, want any) bool {
	switch g := got.(type) {
	case string:
		return strings.Contains(strings.ToLower(g), strings.ToLower(fmt.Sprint(want)))
	case []any:
		for _, x := range g {
			if equal(x, want) {
				return true
			}
		}
	case map[string]any:
		_, ok := g[fmt.Sprint(want)]
		return ok
	}
	return false
}

func toNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

// show affiche une valeur de façon lisible (JSON compact, tronqué).
func show(v any) string {
	var s string
	switch x := v.(type) {
	case nil:
		s = "null"
	case string:
		b, _ := json.Marshal(x)
		s = string(b)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			s = fmt.Sprint(x)
		} else {
			s = string(b)
		}
	}
	if r := []rune(s); len(r) > 80 {
		s = string(r[:77]) + "..."
	}
	return s
}

// captured renvoie la valeur à garder dans une variable : texte brut pour une chaîne, JSON sinon.
func captured(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
