package req

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakePlaceholder imite les réponses de JSONPlaceholder utilisées par le fichier d'exemple.
func fakePlaceholder(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/posts/1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			fmt.Fprint(w, `{"userId": 1, "id": 1, "title": "sunt aut facere", "body": "quia et suscipit"}`)
		case "PATCH":
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			out := map[string]any{"userId": 1, "id": 1, "body": "quia"}
			for k, v := range in {
				out[k] = v
			}
			json.NewEncoder(w).Encode(out)
		case "DELETE":
			fmt.Fprint(w, `{}`)
		}
	})
	mux.HandleFunc("/posts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "type", 415)
				return
			}
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			in["id"] = 101
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(in)
			return
		}
		if r.URL.Query().Get("userId") != "1" {
			fmt.Fprint(w, `[]`)
			return
		}
		var posts []map[string]any
		for i := 1; i <= 10; i++ {
			posts = append(posts, map[string]any{"userId": 1, "id": i})
		}
		json.NewEncoder(w).Encode(posts)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "requetes.http")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExampleFileRunsEndToEnd(t *testing.T) {
	srv := fakePlaceholder(t)
	path := writeFile(t, Example)
	var out bytes.Buffer
	code := Run([]string{path, "--var", "api=" + srv.URL}, &out)
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out.String())
	}
	for _, want := range []string{"5 requêtes : 5 réussies", "garde auteur = \"1\"", "201 Created", "ok    json.length == 10"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("sortie sans %q :\n%s", want, out.String())
		}
	}
	// l'ordre des clés de la réponse est conservé à l'affichage
	if i, j := strings.Index(out.String(), `"userId"`), strings.Index(out.String(), `"title"`); i < 0 || j < 0 || i > j {
		t.Error("les clés JSON doivent garder l'ordre de la réponse")
	}
}

func TestFailuresAreReported(t *testing.T) {
	srv := fakePlaceholder(t)
	path := writeFile(t, `@api = `+srv.URL+`
### Bon
GET {{api}}/posts/1
> statut 200

### Mauvais
GET {{api}}/posts/1
> statut 404
> json.title == "autre"
> json.inconnu existe
`)
	var out bytes.Buffer
	if code := Run([]string{path}, &out); code != 1 {
		t.Fatalf("code %d, attendu 1", code)
	}
	o := out.String()
	for _, want := range []string{"ÉCHEC statut 404", "(obtenu 200)", `(obtenu "sunt aut facere")`, "json.inconnu est absent", "1 en échec (2. Mauvais)"} {
		if !strings.Contains(o, want) {
			t.Errorf("sortie sans %q :\n%s", want, o)
		}
	}
}

func TestSelectByNumberAndTitle(t *testing.T) {
	srv := fakePlaceholder(t)
	path := writeFile(t, Example)
	var out bytes.Buffer
	if code := Run([]string{path, "créer", "--var", "api=" + srv.URL}, &out); code != 0 {
		t.Fatalf("code %d\n%s", code, out.String())
	}
	if strings.Contains(out.String(), "Lire un article") || !strings.Contains(out.String(), "Créer un article") {
		t.Errorf("seule la requête « Créer » devait partir :\n%s", out.String())
	}
	out.Reset()
	// la n°2 utilise {{auteur}}, gardé par la n°1 : seule, elle échoue avec une explication
	if code := Run([]string{path, "2", "--var", "api=" + srv.URL}, &out); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(out.String(), "variable inconnue {{auteur}}") || !strings.Contains(out.String(), "lance tout le fichier") {
		t.Errorf("explication attendue :\n%s", out.String())
	}
}

func TestQuickRequest(t *testing.T) {
	srv := fakePlaceholder(t)
	var out bytes.Buffer
	if code := Run([]string{"POST", srv.URL + "/posts", "-d", `{"title":"x"}`}, &out); code != 0 {
		t.Fatalf("code %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "201 Created") || !strings.Contains(out.String(), `"id": 101`) {
		t.Errorf("sortie :\n%s", out.String())
	}
}

func TestParse(t *testing.T) {
	f, err := Parse("t.http", `# commentaire
@base = http://localhost:3000
@token = abc

GET {{base}}/sans-titre

### Connexion
@email = a@b.c
POST {{base}}/login
Authorization: Bearer {{token}}
// commentaire dans les en-têtes

{"email": "{{email}}"}

# commentaire après le corps
> statut 200
>> jeton = json.token

### Fichier
PUT {{base}}/upload
Content-Type: application/octet-stream

< ./donnees.bin
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Vars) != 2 || len(f.Requests) != 3 {
		t.Fatalf("vars %d requêtes %d", len(f.Vars), len(f.Requests))
	}
	r0, r1, r2 := f.Requests[0], f.Requests[1], f.Requests[2]
	if r0.Method != "GET" || r0.Name() != "GET {{base}}/sans-titre" {
		t.Errorf("requête sans ### : %+v", r0)
	}
	if r1.Title != "Connexion" || len(r1.Vars) != 1 || len(r1.Headers) != 1 || r1.Body != `{"email": "{{email}}"}` {
		t.Errorf("requête 2 : %+v", r1)
	}
	if len(r1.Checks) != 2 || !r1.Checks[1].Capture || r1.Checks[1].Text != "jeton = json.token" {
		t.Errorf("vérifications : %+v", r1.Checks)
	}
	if r2.BodyFile != "./donnees.bin" || r2.Body != "" {
		t.Errorf("corps depuis un fichier : %+v", r2)
	}
}

func TestParseErrorsAreHelpful(t *testing.T) {
	cases := map[string]string{
		"### Titre\nbonjour": "n'est pas une requête",
		"### A\nGET http://x\nContent-Type application/json": "en-tête invalide",
		"### A\nGET http://x\n> statut 200\nGET http://y":    "commence une nouvelle requête avec ###",
		"### A\nPOST": "il manque l'adresse",
	}
	for src, want := range cases {
		_, err := Parse("f.http", src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q : %v, attendu %q", src, err, want)
		}
	}
}

func TestEvaluate(t *testing.T) {
	res := &Response{
		Status:   201,
		Header:   http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body:     []byte(`{"id": 7, "nom": "Rayan", "tags": ["go", "web"], "prix": 12.5, "vide": null, "items": [{"n": "a"}, {"n": "b"}]}`),
		Duration: 120 * time.Millisecond,
	}
	pass := []string{
		"statut 201", "statut 2xx", "status == 201", "json.id == 7", "json.id == \"7\"", "json.nom == Rayan",
		`json.nom == "Rayan"`, "json.nom != Bob", "json.tags contient web", "json.tags.length == 2",
		"json.items[1].n == b", "json.items[-1].n == b", "json.prix >= 10", "json.prix < 13", "json.vide == null",
		"json.vide existe", "json.manque absent", "corps contient rayan", "en-tête Content-Type contient json",
		"header content-type contains JSON", "temps < 500", "json.nom ne contient pas Bob", "json.nom.length == 5",
	}
	for _, c := range pass {
		ok, why, err := evaluate(c, res)
		if err != nil || !ok {
			t.Errorf("%q devrait réussir (%s, %v)", c, why, err)
		}
	}
	fail := []string{"statut 200", "statut 4xx", "json.id == 8", "json.tags contient rust", "json.manque existe", "temps > 500"}
	for _, c := range fail {
		if ok, _, err := evaluate(c, res); ok || err != nil {
			t.Errorf("%q devrait échouer sans erreur (ok=%v, %v)", c, ok, err)
		}
	}
	for _, c := range []string{"json.id", "truc == 1", "json.nom > 3"} {
		if _, _, err := evaluate(c, res); err == nil {
			t.Errorf("%q devrait être signalée comme mal écrite", c)
		}
	}
}

func TestExpand(t *testing.T) {
	t.Setenv("MON_JETON", "secret")
	vars := map[string]string{"base": "http://h", "url": "{{base}}/api"}
	got, err := expand("{{url}}/x?t={{$env.MON_JETON}}&id={{ $uuid }}", vars)
	if err != nil || !strings.HasPrefix(got, "http://h/api/x?t=secret&id=") || len(got) != len("http://h/api/x?t=secret&id=")+36 {
		t.Errorf("%q %v", got, err)
	}
	if _, err := expand("{{manque}}", vars); err == nil || !strings.Contains(err.Error(), "{{manque}}") {
		t.Errorf("variable inconnue : %v", err)
	}
}

func TestNewCreatesExampleOnce(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(old) })
	if code := Run([]string{"nouveau"}, io.Discard); code != 0 {
		t.Fatalf("code %d", code)
	}
	if b, err := os.ReadFile("requetes.http"); err != nil || string(b) != Example {
		t.Fatal("requetes.http doit contenir l'exemple")
	}
	if code := Run([]string{"nouveau"}, io.Discard); code != 1 {
		t.Fatal("ne doit pas écraser un fichier existant")
	}
	if code := Run([]string{"nouveau", "api"}, io.Discard); code != 0 {
		t.Fatal("api.http")
	}
	if _, err := os.Stat("api.http"); err != nil {
		t.Fatal("l'extension .http doit être ajoutée")
	}
}
