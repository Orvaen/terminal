<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/brand/terminal-logo-blanc.svg">
  <img src="docs/brand/terminal-logo-noir.svg" alt="Terminal" height="56">
</picture>

<br>

Le terminal façon Linux pour Windows. Un logiciel Arkerion.

Tu l'ouvres d'un double-clic, comme Git Bash, et tu tapes `sudo apt update`, `apt install git`, `ls`, `rm -rf`
ou `docker run`, sans passer par un installateur. Les programmes et commandes Windows continuent de marcher.

```text
$ sudo apt update && apt install git node vscode
$ mkdir -p projet/src && cd projet && code .
$ apt install docker && docker run --rm hello-world
$ sudo apt full-upgrade
```

## Fonctionnalités

- **Prompt au choix** : 8 styles (minimal, flèche, blocs, Git, PowerShell…) ou le tien, avec `prompt`.
- **Commandes Linux** : `ls`, `cd`, `cat`, `rm`, `cp`, `mv`, `mkdir -p`, enchaînements `&&` et `||`, vrais tubes et redirections (`|`, `>`, `2>&1`), jokers (`*.log`, `src/**/*.ts`, `{a,b}`), complétion avec Tab.
- **Coreutils de Microsoft** : `apt install coreutils` ajoute `grep`, `head`, `sort`, `find`, `xargs`… (78 commandes Unix natives), branchées sur le terminal.
- **apt** : installe et met à jour tes logiciels via winget, et Windows via Windows Update. Comprend les noms Linux (`code`, `fd-find`…).
- **apt deps** : détecte les outils dont un projet a besoin (Node, Go, Python, Docker, make…) et les installe, avec les dépendances du projet.
- **sudo** : lance une commande en administrateur.
- **Docker léger** : le vrai Docker Engine sans Docker Desktop, qui s'éteint tout seul quand il ne sert pas.
- **Mise à jour à chaud** : `apt upgrade terminal`, sans réinstaller ni fermer ce qui tourne.
- **run** : les scripts du projet dans `terminal.json`, comme ceux de `package.json`, et en parallèle avec `run -p` (à la place de concurrently).
- **req** : teste tes API (GET, POST, DELETE…) depuis un simple fichier `.http`, avec des vérifications.

## Installation

Il faut [Go](https://go.dev/dl/) (`winget install GoLang.Go`).

```powershell
git clone https://github.com/Orvaen/terminal
cd terminal
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

Le script compile, installe et ouvre le terminal. Tu le retrouves ensuite dans le menu Démarrer, sur le Bureau
et dans Windows Terminal.

## Documentation

- [Installation](docs/installation.md)
- [Commandes](docs/commandes.md)
- [apt](docs/apt.md) et la [liste des paquets connus](docs/paquets.md)
- [Docker léger](docs/docker.md)
- [run : les scripts du projet](docs/scripts.md)
- [req : tester des requêtes HTTP](docs/req.md)
- [Logo](docs/identite.md)
- [Développement](docs/developpement.md)
