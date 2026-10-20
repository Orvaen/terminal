# Orvaen Terminal

Un terminal façon Linux pour Windows, à la manière de Git Bash : tu l'ouvres d'un double-clic et tu tapes
`sudo apt update`, `apt install git`, `ls`, `cd`, `rm -rf`… comme sous Linux, tout en gardant les programmes
et commandes Windows.

- **`apt` intégré** : installe et met à jour tes logiciels via **winget** (le gestionnaire de paquets de Windows),
  et Windows lui-même via **Windows Update**. Fini les installateurs à télécharger.
- **`sudo`** : lance une commande en administrateur.
- **Docker léger** : `apt install docker` donne le vrai Docker Engine, sans Docker Desktop, qui s'éteint tout seul quand tu ne t'en sers pas.
- **Commandes Linux** : `ls`, `cd`, `pwd`, `cat`, `touch`, `mkdir`, `rm`, `cp`, `mv`, `echo`, `which`, `export`, `clear`…
- **Tout le reste marche** : `git`, `code .`, `notepad`, et les commandes PowerShell (`dir`, `Get-Process`, tubes `|`…).

```text
Rayan@PC WIN ~/Desktop
$ sudo apt update && apt install git node vscode
$ mkdir -p projet/src && cd projet && code .
$ sudo apt full-upgrade      # logiciels + mises à jour Windows
```

Trois exécutables Go, sans dépendance :
- `terminal.exe` — le terminal (avec `apt` et `sudo` intégrés) ;
- `apt.exe` — la commande `apt` seule, utilisable aussi depuis PowerShell, cmd ou Windows Terminal ;
- `docker.exe` — le **Docker léger** (voir plus bas), sans Docker Desktop.

## Installation

### Depuis les sources

Il faut [Go](https://go.dev/dl/) (ou `winget install GoLang.Go`).

```powershell
git clone https://github.com/Orvaen/terminal
cd terminal
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

### Depuis une release

Télécharge le zip de la dernière [release](https://github.com/Orvaen/terminal/releases) (`x64` pour la plupart des PC,
`arm64` pour les PC ARM), dézippe-le et lance :

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

### Ce que fait l'installeur

- copie `terminal.exe`, `apt.exe`, `apt-get.exe` et `docker.exe` dans `%LOCALAPPDATA%\Programs\apt-windows` et ajoute ce dossier au PATH ;
- crée un raccourci **Orvaen Terminal** dans le menu Démarrer et sur le Bureau ;
- ajoute un profil **Orvaen Terminal** dans Windows Terminal (menu ⌄ des onglets) ;
- lance le terminal.

> Les fenêtres déjà ouvertes ne voient pas le nouveau PATH : pour utiliser `apt` dans PowerShell ou cmd, ouvre une nouvelle fenêtre.
> Dans Orvaen Terminal, `apt` est intégré et marche tout de suite.

> Si tu avais chargé l'ancien script `apt-windows.ps1` dans ton profil PowerShell, retire la ligne correspondante de `$PROFILE`.

## Docker léger (sans Docker Desktop)

```text
$ apt install docker
$ docker run --rm hello-world
$ docker run -d -p 8080:80 -v C:\Users\Rayan\site:/usr/share/nginx/html nginx:alpine
$ docker compose up -d
```

`apt install docker` n'installe **pas** Docker Desktop. À la place :

1. WSL2 est activé si besoin (une fenêtre admin s'ouvre, puis **un redémarrage** — une seule fois, ensuite relance `apt install docker`) ;
2. une mini machine **Alpine Linux** (quelques Mo) est créée dans WSL2, à part de tes autres distributions ;
3. **Docker Engine** et **Docker Compose** y sont installés, sans interface ni service Windows.

Ensuite :

- **Démarrage à la demande** : rien ne tourne tant que tu ne tapes pas de commande `docker`. La première démarre le moteur (quelques secondes).
- **Arrêt automatique** : après 10 min sans commande ni conteneur actif, le moteur s'arrête et WSL libère la mémoire.
  Délai réglable avec la variable d'environnement `ORVAEN_DOCKER_IDLE` (en secondes, `0` = jamais).
- **Chemins Windows** convertis automatiquement : `-v C:\projet:/app` devient `/mnt/c/projet:/app`.
- **Ports** : `-p 8080:80` est accessible depuis Windows sur `localhost:8080`.
- **Partout** : `docker` marche dans Orvaen Terminal, et aussi dans PowerShell, cmd ou VS Code grâce à `docker.exe`.

| Commande | Rôle |
|---|---|
| `docker engine status` | État du moteur et place prise sur le disque |
| `docker engine stop` | Arrête le moteur tout de suite et libère la mémoire |
| `docker engine start` | Démarre le moteur (automatique à la première commande) |
| `docker engine logs` | Journal du moteur |
| `apt upgrade docker` | Met à jour Docker Engine (aussi fait par `apt upgrade`) |
| `apt remove docker` | Supprime la machine, avec tous les conteneurs, images et volumes (confirmation demandée) |

Bon à savoir :
- les conteneurs s'arrêtent avec le moteur : relance-les (`docker compose up -d`) après un arrêt automatique ou un redémarrage du PC ;
- les fichiers lus depuis `C:\` à travers `/mnt/c` sont plus lents que dans Linux : pour un gros projet (ex. `node_modules`), préfère un volume nommé ;
- Docker Desktop reste installable sous le nom `docker-desktop`.

## `sudo`

- `sudo apt install …`, `sudo apt upgrade` : pas besoin d'être admin, winget affiche lui-même la fenêtre UAC quand un installeur le demande.
- `sudo apt full-upgrade` et toute autre commande : relancée en administrateur.
  - Si le **sudo de Windows 11** est activé en mode normal (`sudo config --enable normal` dans un terminal admin,
    ou *Paramètres → Système → Pour les développeurs → Activer sudo*), la commande s'exécute dans la même fenêtre.
  - Sinon, une fenêtre administrateur s'ouvre (confirmation UAC), exécute la commande et attend Entrée avant de se fermer.

Le prompt affiche `ADMIN` et `#` quand le terminal tourne en administrateur.

## Commandes intégrées

| Commande | Rôle |
|---|---|
| `apt <commande> [paquets]` | `update`, `upgrade`, `full-upgrade`, `install`, `remove`, `search`, `show`, `list [--upgradable]` |
| `sudo <commande>` | Exécute en administrateur |
| `docker`, `docker-compose` | Docker léger (voir plus haut) |
| `ls [-a] [-l] [-h]`, `ll` | Liste un dossier (les fichiers cachés de Windows sont masqués sans `-a`) |
| `cd [dossier\|-\|~]` | Change de dossier — accepte `/c/Users/...` comme Git Bash |
| `pwd`, `cat`, `touch`, `mkdir [-p]` | Comme sous Linux |
| `rm [-r] [-f]`, `rmdir` | Supprime (refuse la racine d'un disque et ton dossier perso) |
| `cp [-r]`, `mv` | Copie, déplace, renomme |
| `echo`, `clear`, `which`, `export`, `unset`, `env` | Comme sous Linux |
| `open [chemin]` | Ouvre dans l'Explorateur Windows |
| `help`, `exit` | Aide, quitter |

Enchaînement : `&&`, `||` et `;`. Les tubes (`|`), redirections (`>`, `<`) et variables (`$env:...`) sont confiés à PowerShell.
Tout nom qui n'est pas une commande intégrée est lancé comme programme Windows, ou à défaut comme commande PowerShell.

`terminal -c "commande"` exécute une ligne et quitte.

## `apt` en détail

| Commande | Ce que ça fait |
|---|---|
| `apt update` | Rafraîchit les sources winget et liste les mises à jour disponibles |
| `apt upgrade [paquet…]` | Met à jour tous les logiciels, ou seulement ceux indiqués |
| `apt full-upgrade` | `upgrade` + mises à jour Windows (alias `dist-upgrade`) |
| `apt install <paquet…>` | Installe un ou plusieurs logiciels |
| `apt remove <paquet…>` | Désinstalle (alias `purge`, `uninstall`) |
| `apt search <mot>` | Cherche un logiciel |
| `apt show <paquet>` | Détails d'un logiciel |
| `apt list [--upgradable]` | Logiciels installés, ou ceux qui ont une mise à jour |
| `apt windows-update` | Uniquement les mises à jour Windows (admin) |

`apt-get` fonctionne aussi, et `-y` est accepté.

Noms courts traduits en identifiants winget exacts : `git`, `node`/`nodejs`, `python`/`python3`, `vscode`/`code`,
`go`, `rust`, `docker`, `gh`, `chrome`, `firefox`, `discord`, `vlc`, `7zip`… (liste complète : `apt help` ou
[`internal/apt/aliases.go`](internal/apt/aliases.go)). Les autres noms sont transmis à winget ;
`apt search <nom>` donne l'identifiant exact (ex. `apt install Spotify.Spotify`).

## Limites actuelles

- Pas encore de complétion avec Tab. L'historique marche avec les flèches ↑/↓ (et F7 pour la liste).
- Les commandes confiées à PowerShell (tubes, `dir`…) prennent une demi-seconde de plus au démarrage de PowerShell.

## Développement

```bash
go test ./...
GOOS=windows go build -o terminal.exe ./cmd/terminal
```

La CI GitHub vérifie, teste et compile à chaque push. Pousser un tag `vX.Y.Z` publie une release avec les zips x64 et arm64.

```
cmd/terminal/       le terminal (shell interactif)
cmd/apt/            la commande apt seule
cmd/docker/         la commande docker seule
internal/shell/     analyse des lignes, prompt, commandes intégrées
internal/apt/       commandes apt, alias, winget, Windows Update
internal/docker/    Docker léger : machine Alpine dans WSL2, moteur à la demande
internal/platform/  droits admin, élévation UAC, console Windows
internal/ui/        affichage (==>, W:, E:)
```
