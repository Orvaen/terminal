package req

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Orvaen/terminal/internal/ui"
)

const usage = `req — tester des requêtes HTTP

  req nouveau [fichier.http]      Crée un fichier d'exemple commenté (requetes.http)
  req fichier.http                Envoie toutes les requêtes du fichier, dans l'ordre
  req fichier.http 2 connexion    Envoie seulement la requête n°2 et celle dont le titre contient « connexion »
  req liste fichier.http          Liste les requêtes du fichier
  req GET https://exemple.com     Requête rapide, sans fichier
  req POST url -d '{"a": 1}' -H "Authorization: Bearer abc"

Options :
  -v               Affiche les en-têtes et la réponse complète
  --var nom=val    Donne ou remplace une variable du fichier
  -d corps         Corps de la requête rapide (JSON détecté automatiquement)
  -H "Nom: val"    En-tête de la requête rapide (répétable)

Le format des fichiers est expliqué dans le fichier d'exemple et sur docs/req.md.
`

// Run exécute la commande req et renvoie le code de sortie (0 si tout a réussi).
func Run(args []string, out io.Writer) int {
	var (
		verbose bool
		forced  = map[string]string{}
		body    string
		headers []Header
		rest    []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, bool) {
			if i+1 >= len(args) {
				ui.Errorf("req : il manque une valeur après %s", a)
				return "", false
			}
			i++
			return args[i], true
		}
		switch a {
		case "-v", "--verbose":
			verbose = true
		case "--var":
			v, ok := next()
			if !ok {
				return 2
			}
			k, val, found := strings.Cut(v, "=")
			if !found {
				ui.Errorf("req : --var attend nom=valeur")
				return 2
			}
			forced[strings.TrimSpace(k)] = val
		case "-d", "--data":
			v, ok := next()
			if !ok {
				return 2
			}
			body = v
		case "-H", "--header":
			v, ok := next()
			if !ok {
				return 2
			}
			k, val, found := strings.Cut(v, ":")
			if !found {
				ui.Errorf("req : -H attend \"Nom: valeur\"")
				return 2
			}
			headers = append(headers, Header{strings.TrimSpace(k), strings.TrimSpace(val)})
		default:
			rest = append(rest, a)
		}
	}

	runner := NewRunner(out, ui.ColorEnabled() && out == io.Writer(os.Stdout))
	runner.Verbose = verbose
	runner.Forced = forced

	if len(rest) == 0 {
		fmt.Fprint(out, usage)
		return 0
	}
	switch strings.ToLower(rest[0]) {
	case "aide", "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return 0
	case "nouveau", "new", "init":
		name := "requetes.http"
		if len(rest) > 1 {
			name = rest[1]
		}
		return createExample(name, out)
	case "liste", "list":
		if len(rest) < 2 {
			ui.Errorf("req : précise le fichier : req liste fichier.http")
			return 2
		}
		f, code := load(rest[1])
		if f == nil {
			return code
		}
		for i, r := range f.Requests {
			fmt.Fprintf(out, "%3d  %-7s %s\n", i+1, r.Method, r.Name())
		}
		return 0
	}

	if methods[strings.ToUpper(rest[0])] {
		if len(rest) < 2 {
			ui.Errorf("req : il manque l'adresse : req %s https://exemple.com", strings.ToUpper(rest[0]))
			return 2
		}
		r := &Request{Method: strings.ToUpper(rest[0]), URL: rest[1], Headers: headers, Body: body}
		if res := runner.Do(r, 1, 1, false); res.Failed {
			return 1
		}
		return 0
	}

	f, code := load(rest[0])
	if f == nil {
		return code
	}
	return runFile(runner, f, rest[1:], out)
}

func load(path string) (*File, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			ui.Errorf("req : « %s » introuvable. Pour créer un exemple : req nouveau", path)
		} else {
			ui.Errorf("req : %v", err)
		}
		return nil, 2
	}
	f, err := Parse(path, string(data))
	if err != nil {
		ui.Errorf("%v", err)
		return nil, 2
	}
	if len(f.Requests) == 0 {
		ui.Errorf("req : aucune requête dans %s", path)
		return nil, 2
	}
	return f, 0
}

func runFile(runner *Runner, f *File, selectors []string, out io.Writer) int {
	runner.Dir = filepath.Dir(f.Path)
	for _, v := range f.Vars {
		runner.Vars[v.Name] = v.Value
	}

	selected, err := selectRequests(f, selectors)
	if err != nil {
		ui.Errorf("req : %v", err)
		return 2
	}

	failed := 0
	var failedNames []string
	for i, idx := range selected {
		if i > 0 {
			fmt.Fprintln(out)
		}
		r := f.Requests[idx]
		res := runner.Do(r, idx+1, len(f.Requests), true)
		if res.Failed {
			failed++
			failedNames = append(failedNames, fmt.Sprintf("%d. %s", idx+1, r.Name()))
			if _, ok := res.Err.(*VarError); ok || strings.Contains(fmt.Sprint(res.Err), "variable inconnue") {
				if len(selectors) > 0 {
					fmt.Fprintln(out, runner.paint("2", "   (si elle est gardée par une requête précédente avec >>, lance tout le fichier)"))
				}
			}
		}
	}

	if len(selected) > 1 {
		fmt.Fprintln(out)
		ok := len(selected) - failed
		summary := fmt.Sprintf("%d requêtes : %d réussie%s", len(selected), ok, plural(ok))
		if failed > 0 {
			summary += fmt.Sprintf(", %d en échec (%s)", failed, strings.Join(failedNames, ", "))
			fmt.Fprintln(out, runner.paint("1;31", "==>")+" "+summary)
		} else {
			fmt.Fprintln(out, runner.paint("1;32", "==>")+" "+summary)
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// selectRequests : numéros (2) ou morceaux de titre (connexion) ; tout le fichier si rien n'est donné.
func selectRequests(f *File, selectors []string) ([]int, error) {
	if len(selectors) == 0 {
		all := make([]int, len(f.Requests))
		for i := range all {
			all[i] = i
		}
		return all, nil
	}
	seen := map[int]bool{}
	var out []int
	for _, s := range selectors {
		if n, err := strconv.Atoi(s); err == nil {
			if n < 1 || n > len(f.Requests) {
				return nil, fmt.Errorf("pas de requête n°%d (le fichier en contient %d)", n, len(f.Requests))
			}
			if !seen[n-1] {
				seen[n-1] = true
				out = append(out, n-1)
			}
			continue
		}
		found := false
		for i, r := range f.Requests {
			if strings.Contains(strings.ToLower(r.Name()), strings.ToLower(s)) {
				found = true
				if !seen[i] {
					seen[i] = true
					out = append(out, i)
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("aucune requête ne correspond à « %s » (voir : req liste %s)", s, f.Path)
		}
	}
	return out, nil
}

func createExample(name string, out io.Writer) int {
	if !strings.Contains(filepath.Base(name), ".") {
		name += ".http"
	}
	if _, err := os.Stat(name); err == nil {
		ui.Errorf("req : %s existe déjà, choisis un autre nom : req nouveau autre.http", name)
		return 1
	}
	if err := os.WriteFile(name, []byte(Example), 0o644); err != nil {
		ui.Errorf("req : %v", err)
		return 1
	}
	fmt.Fprintf(out, "%s créé. Ouvre-le pour voir comment il est écrit, puis lance :  req %s\n", name, name)
	return 0
}

// Example : le fichier créé par « req nouveau », pensé pour débuter.
const Example = `# Fichier de requêtes HTTP. Pour l'envoyer :  req requetes.http
#
#  - Chaque requête commence par ###, suivi d'un titre.
#  - Les lignes qui commencent par # sont des commentaires.
#  - On peut n'envoyer qu'une requête :  req requetes.http 2
#
# Ce fichier utilise JSONPlaceholder, une fausse API publique faite pour s'entraîner :
# rien n'y est vraiment enregistré, tu peux tout essayer.

# Une variable se déclare avec @nom = valeur et s'utilise avec {{nom}}.
@api = https://jsonplaceholder.typicode.com


### Lire un article
GET {{api}}/posts/1

# Les lignes qui commencent par > vérifient la réponse.
> statut 200
> json.id == 1
# >> garde une valeur de la réponse dans une variable, pour les requêtes suivantes.
>> auteur = json.userId


### Les articles du même auteur
GET {{api}}/posts?userId={{auteur}}

> statut 200
> json.length == 10


### Créer un article
# Les en-têtes se mettent juste sous la requête,
# puis une ligne vide, puis le corps (ici du JSON).
POST {{api}}/posts
Content-Type: application/json

{
  "title": "Mon premier article",
  "body": "Bonjour !",
  "userId": 1
}

> statut 201
> json.title == "Mon premier article"
> json.id existe


### Modifier le titre
PATCH {{api}}/posts/1
Content-Type: application/json

{ "title": "Nouveau titre" }

> statut 2xx
> json.title == "Nouveau titre"


### Supprimer l'article
DELETE {{api}}/posts/1

> statut 200
> temps < 2000


# Autres vérifications possibles :
#   > corps contient "texte"
#   > en-tête Content-Type contient json
#   > json.liste[0].nom != "Bob"
#   > json.prix >= 10
#   > json.erreur absent
# Variables toutes prêtes : {{$uuid}}, {{$timestamp}}, {{$env.MA_VARIABLE}}
`
