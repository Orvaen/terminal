# Installation

[← Retour à l'accueil](../README.md)

Terminal tourne sous **Windows 10 ou 11**. Il s'appuie sur des outils déjà présents dans Windows :
**winget** pour les logiciels (application « App Installer », installée d'office sur Windows 11),
et **WSL2** pour le Docker léger, activé seulement si tu installes Docker.

## Depuis les sources

Il faut [Go](https://go.dev/dl/) 1.22 ou plus récent (`winget install GoLang.Go`) et Git.

```powershell
git clone https://github.com/Orvaen/terminal
cd terminal
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

`build.ps1` compile les trois exécutables, puis lance l'installeur.

## Depuis une release

1. Télécharge le zip de la dernière [release](https://github.com/Orvaen/terminal/releases) :
   `x64` pour la plupart des PC, `arm64` pour les PC ARM (Snapdragon).
2. Dézippe-le, puis dans PowerShell, depuis le dossier dézippé :

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

## Ce que fait l'installeur

| Étape | Détail |
|---|---|
| Copie | `terminal.exe`, `apt.exe`, `apt-get.exe`, `docker.exe` et l'icône dans `%LOCALAPPDATA%\Programs\apt-windows` |
| PATH | Ajoute ce dossier à ton PATH utilisateur, pour utiliser `apt` et `docker` depuis PowerShell, cmd ou VS Code |
| Raccourcis | **Orvaen Terminal** dans le menu Démarrer et sur le Bureau |
| Windows Terminal | Ajoute un profil **Orvaen Terminal** (menu ⌄ des onglets) |
| Lancement | Ouvre le terminal |

L'installation se fait pour ton compte uniquement : aucun droit administrateur n'est demandé.

> [!NOTE]
> Les fenêtres déjà ouvertes ne voient pas le nouveau PATH. Pour utiliser `apt` dans PowerShell ou cmd,
> ouvre une **nouvelle** fenêtre. Dans le terminal, `apt` est intégré et marche tout de suite.

> [!TIP]
> Si tu avais ajouté l'ancien script `apt-windows.ps1` à ton profil PowerShell, retire la ligne correspondante
> de `$PROFILE` : une fonction PowerShell `apt` passerait avant `apt.exe`.

## Mettre à jour

```powershell
cd terminal
git pull
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

Avec une release, télécharge la nouvelle version et relance `install.ps1`. Tes réglages et le Docker léger sont conservés.

## Désinstaller

1. Si tu utilises le Docker léger : `apt remove docker` (supprime la machine et ses données, après confirmation).
2. Supprime le dossier `%LOCALAPPDATA%\Programs\apt-windows`.
3. Supprime les raccourcis **Orvaen Terminal** du menu Démarrer et du Bureau.
4. Supprime le dossier `%LOCALAPPDATA%\Microsoft\Windows Terminal\Fragments\OrvaenTerminal`.
5. Retire `…\Programs\apt-windows` de ton PATH : *Paramètres → Système → Informations système → Paramètres
   avancés du système → Variables d'environnement*.

## En cas de problème

| Symptôme | Solution |
|---|---|
| `apt : Le terme « apt » n'est pas reconnu` | Ouvre une nouvelle fenêtre, ou utilise le terminal où `apt` est intégré |
| `winget est introuvable` | Installe **App Installer** depuis le Microsoft Store |
| `build.ps1` ne trouve pas `go` | Installe Go, puis ouvre une nouvelle fenêtre |
| Le script est bloqué par la stratégie d'exécution | Lance-le avec `powershell -ExecutionPolicy Bypass -File …` comme ci-dessus |

Pour Docker, voir [Docker léger › En cas de problème](docker.md#en-cas-de-problème).
