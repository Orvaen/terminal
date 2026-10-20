# req : tester des requêtes HTTP

[← Retour à l'accueil](../README.md)

`req` envoie des requêtes HTTP (GET, POST, PUT, PATCH, DELETE…) décrites dans un fichier `.http`, affiche les
réponses et vérifie qu'elles sont correctes. Il est intégré au terminal, et `req.exe` marche aussi dans PowerShell
ou cmd.

Le format est celui de l'extension **REST Client** de VS Code : tes fichiers y ont la coloration et peuvent
aussi y être lancés.

## Pour commencer

```text
$ req nouveau
requetes.http créé. Ouvre-le pour voir comment il est écrit, puis lance :  req requetes.http
$ req requetes.http
```

`req nouveau` crée un fichier d'exemple commenté, qui utilise une fausse API publique faite pour s'entraîner.

## Écrire un fichier

```http
# Les lignes qui commencent par # sont des commentaires.
@api = http://localhost:3000

### Se connecter
POST {{api}}/login
Content-Type: application/json

{
  "email": "moi@exemple.com",
  "motDePasse": "secret"
}

> statut 200
>> jeton = json.token

### Mon profil
GET {{api}}/moi
Authorization: Bearer {{jeton}}

> statut 200
> json.email == "moi@exemple.com"
```

| Élément | Écriture |
|---|---|
| Nouvelle requête | `###` suivi d'un titre |
| Requête | `MÉTHODE adresse`, par exemple `GET https://exemple.com/users` |
| En-têtes | `Nom: valeur`, juste sous la requête |
| Corps | après une ligne vide ; un corps JSON reçoit `Content-Type: application/json` tout seul |
| Corps depuis un fichier | `< ./donnees.json` |
| Variable | `@nom = valeur`, puis `{{nom}}` |
| Vérification | `> …` après le corps |
| Garder une valeur | `>> nom = json.chemin`, utilisable ensuite avec `{{nom}}` |

Variables toutes prêtes : `{{$uuid}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$randomInt}}`,
et `{{$env.NOM}}` pour une variable d'environnement (pratique pour un mot de passe qu'on ne veut pas écrire dans le fichier).

## Vérifications

| Vérification | Réussit si |
|---|---|
| `> statut 200` | le statut vaut 200 |
| `> statut 2xx` | le statut commence par 2 (200, 201, 204…) |
| `> json.id == 1` | la valeur vaut 1 (aussi `!=`) |
| `> json.nom == "Rayan"` | le texte est exactement celui-là |
| `> json.prix >= 10` | comparaison de nombres (`<`, `>`, `<=`, `>=`) |
| `> json.users.length == 3` | la liste contient 3 éléments |
| `> json.users[0].nom == "Ana"` | le premier élément de la liste a ce nom (`[-1]` : le dernier) |
| `> json.tags contient "go"` | la liste contient cette valeur, ou le texte contient ce mot |
| `> json.id existe` / `> json.erreur absent` | le champ est présent / absent |
| `> corps contient "bienvenue"` | la réponse brute contient ce texte |
| `> en-tête Content-Type contient json` | l'en-tête contient ce texte |
| `> temps < 500` | la réponse est arrivée en moins de 500 ms |

Les mots anglais marchent aussi : `status`, `body`, `header`, `contains`, `exists`.

## Commandes

| Commande | Effet |
|---|---|
| `req fichier.http` | Envoie toutes les requêtes, dans l'ordre, puis affiche le bilan |
| `req fichier.http 2` | Envoie seulement la requête n°2 |
| `req fichier.http connexion` | Envoie les requêtes dont le titre contient « connexion » |
| `req liste fichier.http` | Liste les requêtes numérotées |
| `req nouveau [nom]` | Crée un fichier d'exemple (`requetes.http` par défaut) |
| `req GET https://exemple.com` | Requête rapide, sans fichier |
| `req POST url -d '{"a": 1}' -H "Authorization: Bearer abc"` | Requête rapide avec corps et en-tête |

| Option | Effet |
|---|---|
| `-v` | Affiche les en-têtes envoyés et reçus, et la réponse complète (sinon coupée après 40 lignes) |
| `--var nom=valeur` | Donne ou remplace une variable, par exemple `--var api=https://staging.exemple.com` |

`req` renvoie le code 1 si une requête échoue ou si une vérification n'est pas respectée : tu peux l'utiliser dans
un script ou une intégration continue.

Dans le terminal, **Tab** complète les fichiers `.http` et les sous-commandes de `req`.
