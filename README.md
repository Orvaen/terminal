# apt pour Windows

Les commandes `apt` de Linux, directement dans n'importe quel terminal Windows (Windows Terminal, PowerShell, cmd).
Fini les installateurs à télécharger : `apt` s'appuie sur **winget**, le gestionnaire de paquets intégré à Windows, et sur **Windows Update** pour le système.

```powershell
sudo apt update
sudo apt upgrade
apt install git node vscode
apt search spotify
sudo apt full-upgrade      # logiciels + mises à jour Windows
```

Un seul exécutable, écrit en Go, sans dépendance.

## Installation

### Depuis une release

1. Télécharge le zip de la dernière [release](https://github.com/Orvaen/terminal/releases) (`x64` pour la plupart des PC, `arm64` pour les PC ARM).
2. Dézippe-le, puis lance dans PowerShell :
   ```powershell
   powershell -ExecutionPolicy Bypass -File .\install.ps1
   ```
3. Ouvre un nouveau terminal et tape `apt help`.

Le script copie `apt.exe` (et `apt-get.exe`) dans `%LOCALAPPDATA%\Programs\apt-windows` et ajoute ce dossier à ton PATH.

### Depuis les sources

```powershell
git clone https://github.com/Orvaen/terminal
cd terminal
go build -o apt.exe ./cmd/apt
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

> Si tu avais chargé l'ancien script `apt-windows.ps1` dans ton profil PowerShell, retire la ligne correspondante de `$PROFILE` : une fonction PowerShell `apt` passerait avant `apt.exe`.

## Et `sudo` ?

Windows 11 (24H2 et plus) contient un vrai `sudo`. Active-le une fois, dans un terminal administrateur :

```powershell
sudo config --enable normal
```

(ou *Paramètres → Système → Pour les développeurs → Activer sudo*). Ensuite `sudo apt …` lance `apt` en administrateur, dans le même terminal.

Sans `sudo`, tout fonctionne quand même :
- `install`, `upgrade` et `remove` : winget affiche lui-même la fenêtre UAC quand un installeur a besoin des droits admin ;
- `full-upgrade` : si tu n'es pas admin, `apt` ouvre une fenêtre administrateur pour la partie Windows Update.

## Commandes

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

`apt-get` fonctionne aussi, et `-y` est accepté (les accords de licence sont acceptés automatiquement).

## Noms de paquets

Les noms courts courants sont traduits en identifiants winget exacts :

| Nom | Identifiant winget |
|---|---|
| `git` | `Git.Git` |
| `node`, `nodejs` | `OpenJS.NodeJS.LTS` |
| `python`, `python3` | `Python.Python.3.12` |
| `vscode`, `code` | `Microsoft.VisualStudioCode` |
| `go`, `golang` | `GoLang.Go` |
| `rust`, `rustup` | `Rustlang.Rustup` |
| `docker` | `Docker.DockerDesktop` |
| `gh` | `GitHub.cli` |

La liste complète est dans `apt help` et dans [`internal/apt/aliases.go`](internal/apt/aliases.go).
Tout autre nom est transmis à winget : `apt search <nom>` donne l'identifiant exact à utiliser (ex. `apt install Spotify.Spotify`).

## Développement

```bash
go test ./...
GOOS=windows go build -o apt.exe ./cmd/apt
```

La CI GitHub compile et teste à chaque push. Pousser un tag `vX.Y.Z` publie une release avec les zips x64 et arm64.

```
cmd/apt/            point d'entrée
internal/apt/       commandes, alias, winget, Windows Update
internal/platform/  droits admin, élévation UAC, couleurs de la console
internal/ui/        affichage (==>, W:, E:)
```
