# run : les scripts du projet

[← Retour à l'accueil](../README.md)

`run` lance les commandes de ton projet décrites dans un fichier `terminal.json`, comme les `scripts` de
`package.json`. Il lance aussi plusieurs commandes **en même temps**, avec une sortie lisible, sans installer
`concurrently`.

```text
$ run                 # liste les scripts
$ run up              # lance le script « up »
$ run test -- f=00    # ajoute des arguments : make test f=00
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

Chaque commande est une ligne du terminal : `&&`, `||`, `ls`, `rm`, `apt`, `docker`, les programmes du PATH…
tout marche comme quand tu la tapes. Pour lancer un autre script, écris `run <nom>`.

Comme avec npm, le dossier `node_modules/.bin` du projet est ajouté au PATH : `"build": "tsc"` utilise le
TypeScript installé dans le projet.

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
