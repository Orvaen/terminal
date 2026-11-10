<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/brand/terminal-logo-blanc.svg">
  <img src="docs/brand/terminal-logo-noir.svg" alt="Terminal" height="56">
</picture>

<br>

# Terminal

Le terminal façon Linux pour Windows, par **Arkerion**. Il s'ouvre d'un double-clic comme Git Bash, comprend `sudo apt update`, `ls`, `rm -rf` ou `docker run`, et laisse marcher les programmes et commandes Windows.

**Plateforme :** Windows 10 et 11

```bash
sudo apt update && apt install git node vscode
mkdir -p projet/src && cd projet && code .
apt install docker && docker run --rm hello-world
sudo apt full-upgrade
```

## Démarrer

Il faut [Go](https://go.dev/dl/) 1.22 ou plus récent (`winget install GoLang.Go`) et Git.

```powershell
git clone https://github.com/Orvaen/terminal
cd terminal
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

Le script compile, installe et ouvre le terminal. Il apparaît ensuite dans le menu Démarrer, sur le Bureau et dans Windows Terminal.

## Mise à jour

```bash
apt upgrade terminal
```

Chaque push sur `main` publie une nouvelle version. La mise à jour se fait sans réinstaller ni couper ce qui tourne.

## Fonctionnalités

- **Shell Linux** : `ls`, `cd`, `cat`, `rm`, `cp`, `mv`, `mkdir -p`, `&&` et `||`, tubes et redirections (`|`, `>`, `2>&1`), jokers (`*.log`, `src/**/*.ts`, `{a,b}`), variables (`$HOME`, `${f%.*}`, `$(commande)`, `$((1+2))`), complétion avec Tab.
- **Alias** : `alias ll='ls -la'`, gardés d'une session à l'autre dans `~/.terminalrc`.
- **apt** : installe et met à jour les logiciels via winget, et Windows via Windows Update. Comprend les noms Linux (`code`, `fd-find`…).
- **apt java** : plusieurs JDK Temurin côte à côte, sans droits administrateur (`apt java install 21`, `apt java use 21`).
- **apt deps** : détecte les outils d'un projet (Node, Go, Python, Docker, make…) et les installe avec ses dépendances.
- **apt optimiser** : exclusions Windows Defender, compilation sur tous les cœurs, cache sccache. Demande avant chaque changement, `apt optimiser annuler` remet tout comme avant.
- **sudo** : lance une commande en administrateur.
- **Coreutils** : `apt install coreutils` ajoute 78 commandes Unix natives de Microsoft (`grep`, `head`, `sort`, `find`, `xargs`…).
- **Scripts** : `./build.sh` se lance comme dans Git Bash, d'après son shebang. Un `.ps1` part sur PowerShell.
- **Programmes à fenêtre** : `code .` ou `notepad` rendent la main tout de suite et restent ouverts après la fermeture du terminal.
- **Docker léger** : le vrai Docker Engine, sans Docker Desktop. Il s'éteint tout seul quand il ne sert pas.
- **run** : les scripts du projet dans `terminal.json`, comme ceux de `package.json`. En parallèle avec `run -p`.
- **req** : teste des API (GET, POST, DELETE…) depuis un fichier `.http`, avec des vérifications.
- **prompt** : 8 styles (minimal, flèche, blocs, Git, PowerShell…) ou le tien.

## Documentation

- [Installation](docs/installation.md)
- [Commandes](docs/commandes.md)
- [apt](docs/apt.md) et les [paquets connus](docs/paquets.md)
- [Docker léger](docs/docker.md)
- [run](docs/scripts.md)
- [req](docs/req.md)
- [Logo](docs/identite.md)
- [Développement](docs/developpement.md)

## Contenu

```
cmd/
  terminal/          terminal.exe, le shell
  apt/  docker/  req/   les mêmes outils en exécutables séparés
internal/
  shell/             saisie, prompt, commandes intégrées, tubes, variables, alias
  apt/  deps/        logiciels (winget), Windows Update, dépendances de projet
  docker/            Docker Engine léger sur WSL2
  scripts/  req/     run et terminal.json, requêtes HTTP
  update/            mise à jour à chaud
  platform/  ui/  setup/
docs/                documentation
  brand/             logo noir et blanc, icône, générateur
build.ps1            compile et installe
install.ps1          installeur
```

---

© Arkerion. Tous droits réservés. Le logo est la propriété d'Arkerion.
