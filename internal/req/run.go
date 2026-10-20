package req

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Runner exécute des requêtes et garde les variables (fichier, captures, --var).
type Runner struct {
	Out     io.Writer
	Color   bool
	Verbose bool
	Client  *http.Client
	Dir     string            // dossier du fichier .http (pour « < fichier »)
	Vars    map[string]string // variables du fichier puis captures
	Forced  map[string]string // --var : prioritaires sur tout
}

// NewRunner prépare un exécuteur avec les réglages par défaut.
func NewRunner(out io.Writer, color bool) *Runner {
	return &Runner{
		Out:    out,
		Color:  color,
		Client: &http.Client{Timeout: 30 * time.Second},
		Vars:   map[string]string{},
		Forced: map[string]string{},
	}
}

func (r *Runner) paint(code, s string) string {
	if !r.Color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (r *Runner) vars(extra []Var) map[string]string {
	m := map[string]string{}
	for k, v := range r.Vars {
		m[k] = v
	}
	for _, v := range extra {
		m[v.Name] = v.Value
	}
	for k, v := range r.Forced {
		m[k] = v
	}
	return m
}

// Result : le bilan d'une requête.
type Result struct {
	Failed   bool // erreur réseau, variable inconnue ou vérification en échec
	Checks   int
	Passed   int
	Err      error
	Response *Response
}

// Do envoie la requête, affiche la réponse et les vérifications.
func (r *Runner) Do(req *Request, index, total int, showTitle bool) Result {
	if showTitle {
		label := req.Name()
		if total > 1 {
			label = fmt.Sprintf("%d/%d  %s", index, total, label)
		}
		fmt.Fprintln(r.Out, r.paint("1;34", "==>")+" "+r.paint("1", label))
	}
	vars := r.vars(req.Vars)

	httpReq, shown, err := r.build(req, vars)
	if err != nil {
		var ve *VarError
		if errors.As(err, &ve) {
			err = fmt.Errorf("%w : déclare-la avec  @%s = valeur  ou donne-la avec  --var %s=valeur", err, ve.Name, ve.Name)
		}
		r.fail(err)
		return Result{Failed: true, Err: err}
	}
	fmt.Fprintln(r.Out, r.paint("2", shown))
	if r.Verbose {
		for name, vals := range httpReq.Header {
			fmt.Fprintln(r.Out, r.paint("2", name+": "+strings.Join(vals, ", ")))
		}
	}

	start := time.Now()
	resp, err := r.Client.Do(httpReq)
	if err != nil {
		err = explainNetwork(err)
		r.fail(err)
		return Result{Failed: true, Err: err}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	resp.Body.Close()
	res := &Response{Status: resp.StatusCode, Header: resp.Header, Body: body, Duration: time.Since(start)}
	if err != nil {
		r.fail(fmt.Errorf("réponse interrompue : %v", err))
		return Result{Failed: true, Err: err, Response: res}
	}

	r.printResponse(resp, res)
	result := Result{Response: res}
	for _, c := range req.Checks {
		if c.Capture {
			if err := r.capture(c, res); err != nil {
				fmt.Fprintf(r.Out, "  %s %s : %v\n", r.paint("1;31", "ÉCHEC"), c.Text, err)
				result.Failed = true
			} else {
				name, _, _ := strings.Cut(c.Text, "=")
				fmt.Fprintf(r.Out, "  %s %s = %s\n", r.paint("2", "garde"), strings.TrimSpace(name), show(r.Vars[strings.TrimSpace(name)]))
			}
			continue
		}
		result.Checks++
		ok, why, err := evaluate(c.Text, res)
		switch {
		case err != nil:
			fmt.Fprintf(r.Out, "  %s %s : %v\n", r.paint("1;31", "ÉCHEC"), c.Text, err)
			result.Failed = true
		case ok:
			fmt.Fprintf(r.Out, "  %s    %s\n", r.paint("1;32", "ok"), c.Text)
			result.Passed++
		default:
			fmt.Fprintf(r.Out, "  %s %s  %s\n", r.paint("1;31", "ÉCHEC"), c.Text, r.paint("2", "("+why+")"))
			result.Failed = true
		}
	}
	return result
}

func (r *Runner) fail(err error) {
	fmt.Fprintln(r.Out, r.paint("1;31", "E:")+" "+err.Error())
}

func (r *Runner) build(req *Request, vars map[string]string) (*http.Request, string, error) {
	url, err := expand(req.URL, vars)
	if err != nil {
		return nil, "", err
	}
	if !strings.Contains(url, "://") {
		url = "http://" + url
	}
	var body []byte
	switch {
	case req.BodyFile != "":
		path, err := expand(req.BodyFile, vars)
		if err != nil {
			return nil, "", err
		}
		if !filepath.IsAbs(path) && r.Dir != "" {
			path = filepath.Join(r.Dir, path)
		}
		if body, err = os.ReadFile(path); err != nil {
			return nil, "", fmt.Errorf("corps : %v", err)
		}
	case req.Body != "":
		s, err := expand(req.Body, vars)
		if err != nil {
			return nil, "", err
		}
		body = []byte(s)
	}
	httpReq, err := http.NewRequest(req.Method, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("adresse invalide « %s »", url)
	}
	httpReq.Header.Set("User-Agent", "req (Orvaen Terminal)")
	for _, h := range req.Headers {
		v, err := expand(h.Value, vars)
		if err != nil {
			return nil, "", err
		}
		httpReq.Header.Add(h.Name, v)
	}
	if len(body) > 0 && httpReq.Header.Get("Content-Type") == "" {
		if json.Valid(body) {
			httpReq.Header.Set("Content-Type", "application/json")
		} else {
			httpReq.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	}
	return httpReq, req.Method + " " + url, nil
}

func explainNetwork(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "actively refused"):
		return fmt.Errorf("connexion refusée : le serveur est-il démarré ? (%v)", err)
	case strings.Contains(msg, "no such host"):
		return fmt.Errorf("adresse introuvable : vérifie le nom du serveur (%v)", err)
	case strings.Contains(msg, "Client.Timeout") || strings.Contains(msg, "deadline exceeded"):
		return fmt.Errorf("pas de réponse après 30 secondes")
	case strings.Contains(msg, "certificate"):
		return fmt.Errorf("certificat HTTPS refusé : %v", err)
	}
	return err
}

func (r *Runner) capture(c Check, res *Response) error {
	name, subject, ok := strings.Cut(c.Text, "=")
	name, subject = strings.TrimSpace(name), strings.TrimSpace(subject)
	if !ok || name == "" || subject == "" {
		return fmt.Errorf("écris  >> nom = json.chemin")
	}
	v, found, err := res.value(subject)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s est absent de la réponse", subject)
	}
	r.Vars[name] = captured(v)
	return nil
}

const bodyPreviewLines = 40

func (r *Runner) printResponse(resp *http.Response, res *Response) {
	code := "1;32"
	switch {
	case res.Status >= 400:
		code = "1;31"
	case res.Status >= 300:
		code = "1;36"
	}
	status := resp.Status
	if status == "" {
		status = fmt.Sprint(res.Status)
	}
	fmt.Fprintf(r.Out, "%s  %s  %s\n", r.paint(code, status),
		r.paint("2", formatDuration(res.Duration)), r.paint("2", formatSize(len(res.Body))))
	if r.Verbose {
		for name, vals := range res.Header {
			fmt.Fprintln(r.Out, r.paint("2", name+": "+strings.Join(vals, ", ")))
		}
	}
	if len(res.Body) == 0 {
		return
	}

	text := ""
	if parsed, err := decodeOrdered(res.Body); err == nil {
		text = r.prettyJSON(parsed, 0)
	} else if utf8.Valid(res.Body) {
		text = strings.TrimRight(string(res.Body), "\r\n")
	} else {
		fmt.Fprintln(r.Out, r.paint("2", "[contenu binaire, "+formatSize(len(res.Body))+"]"))
		return
	}
	lines := strings.Split(text, "\n")
	if !r.Verbose && len(lines) > bodyPreviewLines {
		fmt.Fprintln(r.Out, strings.Join(lines[:bodyPreviewLines], "\n"))
		fmt.Fprintln(r.Out, r.paint("2", fmt.Sprintf("… %d lignes de plus (ajoute -v pour tout voir)", len(lines)-bodyPreviewLines)))
		return
	}
	fmt.Fprintln(r.Out, text)
}

// object : objet JSON qui garde l'ordre de ses clés, pour l'affichage.
type object struct {
	keys []string
	vals map[string]any
}

// decodeOrdered décode du JSON en gardant l'ordre des clés des objets.
func decodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("contenu après le JSON")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := object{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				if _, dup := o.vals[k]; !dup {
					o.keys = append(o.keys, k)
				}
				o.vals[k] = v
			}
			_, err := dec.Token()
			return o, err
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			_, err := dec.Token()
			return arr, err
		}
	}
	return tok, nil
}

// prettyJSON indente le JSON dans l'ordre de la réponse, avec des couleurs discrètes.
func (r *Runner) prettyJSON(v any, depth int) string {
	ind := strings.Repeat("  ", depth)
	switch x := v.(type) {
	case object:
		if len(x.keys) == 0 {
			return "{}"
		}
		keys := x.keys
		var b strings.Builder
		b.WriteString("{\n")
		for i, k := range keys {
			kb, _ := json.Marshal(k)
			b.WriteString(ind + "  " + r.paint("1", string(kb)) + ": " + r.prettyJSON(x.vals[k], depth+1))
			if i < len(keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(ind + "}")
		return b.String()
	case []any:
		if len(x) == 0 {
			return "[]"
		}
		var b strings.Builder
		b.WriteString("[\n")
		for i, e := range x {
			b.WriteString(ind + "  " + r.prettyJSON(e, depth+1))
			if i < len(x)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(ind + "]")
		return b.String()
	case string:
		sb, _ := json.Marshal(x)
		return r.paint("32", string(sb))
	case nil:
		return r.paint("35", "null")
	case json.Number:
		return r.paint("33", x.String())
	default:
		sb, _ := json.Marshal(x)
		return r.paint("33", string(sb))
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return strings.Replace(fmt.Sprintf("%.2f s", d.Seconds()), ".", ",", 1)
}

func formatSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d o", n)
	case n < 1024*1024:
		return strings.Replace(fmt.Sprintf("%.1f Ko", float64(n)/1024), ".", ",", 1)
	}
	return strings.Replace(fmt.Sprintf("%.1f Mo", float64(n)/(1024*1024)), ".", ",", 1)
}
