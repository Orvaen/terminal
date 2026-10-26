# run : les scripts du projet

[← Retour à l'accueil](../README.md)

`run` lance les commandes de ton projet décrites dans un fichier `terminal.json`, comme les `scripts` de
`package.json`. Il lance aussi plusieurs commandes **en même temps**, avec une sortie lisible, sans installer
`concurrently`.

```text
$ run                 # liste les scripts
$ run up              # lance le script « up »
$ run add test test   # passe des arguments au script
$ run add --user test --password test   # … ou nommés
$ run up test         # l'un après l'autre, arrêt au premier échec
$ run -p api web      # en même temps
```

## Le fichier terminal.json

Mets-le à la racine du projet. `run` le cherche dans le dossier courant, puis dans ses parents : tu peux
lancer tes scripts depuis n'importe quel sous-dossier.

```json
{
  "scripts": {
    "up": "make up",
    "check": ["make up", "make test f=00"],
    "api": {
      "run": "npm run dev",
      "cwd": "api",
      "env": { "PORT": "3000" },
      "description": "Serveur de l'API"
    },
    "dev": {
      "parallel": ["run api", "run web", "make logs"],
      "description": "Tout le projet en même temps"
    }
  }
}
```

| Forme | Effet |
|---|---|
| `"nom": "commande"` | Une commande |
| `"nom": ["a", "b"]` | Plusieurs commandes l'une après l'autre ; s'arrête à la première qui échoue |
| `"nom": { "run": … }` | Pareil (texte ou liste), avec des options |
| `"nom": { "parallel": [ … ] }` | Commandes lancées en même temps |

| Option | Rôle |
|---|---|
| `cwd` | Dossier où lancer la commande, relatif à `terminal.json` (par défaut : le dossier de `terminal.json`) |
| `env` | Variables d'environnement ajoutées, lues par les programmes (`process.env.PORT` en Node) |
| `description` | Texte affiché par `run` |
| `args` | Arguments du script, voir [Arguments](#arguments) |

Chaque commande est une ligne du terminal : `&&`, `||`, `ls`, `rm`, `apt`, `docker`, les programmes du PATH…
tout marche comme quand tu la tapes. Pour lancer un autre script, écris `run <nom>`.

Comme avec npm, le dossier `node_modules/.bin` du projet est ajouté au PATH : `"build": "tsc"` utilise le
TypeScript installé dans le projet.

## Arguments

Tout ce que tu tapes après le nom du script lui est passé.

### Arguments déclarés (`args`)

```json
{
  "scripts": {
    "add": {
      "run": "node scripts/add-user.js --user $user --password $password --role $role",
      "args": ["user", "password", "role=membre"],
      "description": "Ajoute un utilisateur"
    }
  }
}
```

```text
$ run add test secret                             # dans l'ordre de "args"
$ run add --user test --password secret           # par leur nom
$ run add --password=secret test --role admin     # on peut mélanger
$ run add --help                                  # rappelle comment l'appeler
```

- `"role=membre"` donne une valeur par défaut ; sans `=`, l'argument est **obligatoire**.
- Dans la commande, écris `$user` ou `${user}` (accolades utiles si un texte suit : `${user}_backup`).
- Un argument manquant, une option inconnue ou un argument en trop arrête tout avant de lancer quoi que ce
  soit, avec la bonne façon d'appeler le script.
- Après `run add`, **Tab** propose `--user`, `--password`, `--role`.

### Sans déclarer : `$1`, `$2`, `$@`

Comme en shell :

| Dans la commande | Devient |
|---|---|
| `$1`, `$2` … `$9` | Le 1er, 2e … argument tapé |
| `$@` (ou `$*`) | Tous les arguments, tels quels |

```json
"deploy": "scp dist/app.zip $1:/srv/$2"
```

`run deploy serveur1 prod` lance `scp dist/app.zip serveur1:/srv/prod`.

### Sans rien : ajoutés à la fin

Si la commande n'utilise ni `$1`, ni `$@`, ni `args`, les arguments sont collés à la fin, comme
`npm run x -- …` : avec `"test": "make test"`, `run test f=00` lance `make test f=00`.

> [!NOTE]
> Les valeurs arrivent intactes : espaces, guillemets, `$` ou `&` sont protégés (`run add "Jean Dupont" 'p@$$'`).
> `$PATH`, `$HOME` et les autres variables d'environnement ne sont pas touchées : seuls `$1`…`$9`, `$@`, `$*`
> et les noms déclarés dans `args` sont remplacés.

### Arguments ou plusieurs scripts ?

`run a b` enchaîne les scripts `a` puis `b` **seulement si** `a` n'attend pas d'arguments (pas d'`args`, pas de
`$1`/`$@`) et que `b` est un script. Dans tous les autres cas, `b` est un argument de `a` : `run add test test`
passe bien `test test` à `add`, même s'il existe un script `test`.

Pour forcer des arguments, mets `--` : `run lint -- test` passe `test` à `lint`.
Les options de `run` lui-même (`-p`, `--init`) se placent **avant** le nom du script.

## En même temps (à la place de concurrently)

```text
$ run dev
[api ] > run api
[web ] > run web
[make] > make logs
[api ] Serveur prêt sur http://localhost:3000
[web ] VITE ready in 312 ms
[make] database-1  | database system is ready to accept connections
```

- Chaque ligne commence par le nom de sa commande, en couleur : `run api` s'affiche `[api]`, une autre commande
  par son premier mot (`[make]`).
- Les lignes de deux commandes ne se mélangent jamais.
- **Si une commande échoue, les autres sont arrêtées** (avec tout ce qu'elles ont lancé, comme `npm` → `node`),
  et `run` renvoie son code d'erreur.
- **Ctrl+C** arrête tout.
- Les couleurs des outils sont gardées (`FORCE_COLOR=1`).

Sans rien écrire dans `terminal.json`, `run -p a b c` lance des scripts existants en même temps.

> [!NOTE]
> En parallèle, les commandes ne peuvent pas lire le clavier. Pour un outil interactif (une question, un menu),
> lance son script seul : `run <nom>`.

## Créer le fichier

```text
$ run --init
terminal.json créé avec 5 script(s) repris de Makefile et package.json. Lance-les avec : run <nom>
```

`run --init` reprend les cibles du `Makefile` (`make up` devient le script `up`) et les scripts de
`package.json` (`npm run dev` devient `dev`). Sans eux, il crée un exemple. Il ne remplace jamais un
`terminal.json` existant.

## Complétion

Après `run`, **Tab** propose les scripts de `terminal.json`.

## Depuis PowerShell, cmd ou VS Code

`run` est une commande du terminal. Ailleurs, passe par lui :

```powershell
terminal -c "run dev"
```

Par exemple dans une tâche VS Code (`.vscode/tasks.json`) : `"command": "terminal -c \"run dev\""`.

## En cas de problème

| Message | Solution |
|---|---|
| `terminal.json introuvable…` | Crée-le avec `run --init`, ou place-toi dans le projet |
| `script inconnu : x` | La liste des scripts s'affiche juste en dessous ; vérifie le nom |
| `JSON invalide` | Une virgule en trop ou un guillemet manquant ; VS Code souligne l'erreur |
| `il faut "run" … ou "parallel"` | Un objet doit contenir l'un des deux |
| `le dossier "cwd" … n'existe pas` | Le chemin de `cwd` est relatif au dossier de `terminal.json` |
| `argument manquant : x` | Ajoute-le (`run add valeur` ou `--x valeur`) ou donne-lui un défaut : `"x=défaut"` |
| `option inconnue : --x` | Ajoute `x` dans `args`, ou utilise `$@` pour tout faire passer |
