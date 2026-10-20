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

`build.ps1` compile les quatre exécutables, puis lance l'installeur.

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
| Copie | `terminal.exe`, `apt.exe`, `apt-get.exe`, `docker.exe`, `req.exe` et l'icône dans `%LOCALAPPDATA%\Programs\apt-windows` |
| PATH | Ajoute ce dossier à ton PATH utilisateur, pour utiliser `apt` et `docker` depuis PowerShell, cmd ou VS Code |
| Raccourcis | **Orvaen Terminal** dans le menu Démarrer et sur le Bureau |
| Windows Terminal | Ajoute un profil **Orvaen Terminal** (menu ⌄ des onglets) |
| VS Code | Ajoute **Orvaen Terminal** aux profils du terminal intégré, et masque `orvaen-docker (WSL)`, la machine interne du Docker léger |
| Lancement | Ouvre le terminal |

L'installation se fait pour ton compte uniquement : aucun droit administrateur n'est demandé.

> [!NOTE]
> Les fenêtres déjà ouvertes ne voient pas le nouveau PATH. Pour utiliser `apt` dans PowerShell ou cmd,
> ouvre une **nouvelle** fenêtre. Dans le terminal, `apt` est intégré et marche tout de suite.

> [!TIP]
> Si tu avais ajouté l'ancien script `apt-windows.ps1` à ton profil PowerShell, retire la ligne correspondante
> de `$PROFILE` : une fonction PowerShell `apt` passerait avant `apt.exe`.

## Dans VS Code

Dans le terminal intégré, ouvre le menu **⌄** à côté du **+** : **Orvaen Terminal** y apparaît.
Pour l'ouvrir par défaut : *Terminal: Select Default Profile* dans la palette de commandes (Ctrl+Maj+P),
puis **Orvaen Terminal**.

L'installeur modifie ton `settings.json` (une copie est gardée dans `settings.json.bak`). S'il contient des
commentaires, il n'y touche pas et affiche les lignes à ajouter toi-même. Pour refaire cette étape seule :

```powershell
terminal --setup-vscode
```

## Mettre à jour

```text
$ apt upgrade terminal
```

Pas besoin de recompiler ni de relancer l'installeur : la commande télécharge la dernière
[release](https://github.com/Orvaen/terminal/releases), vérifie sa somme SHA-256 et remplace les exécutables.
`apt upgrade` le fait aussi, après tes logiciels. Hors du terminal : `terminal --update`.

| Ce qui tourne | Pendant la mise à jour |
|---|---|
| Fenêtres du terminal déjà ouvertes | Continuent avec l'ancienne version ; la nouvelle s'applique aux fenêtres ouvertes ensuite |
| Docker léger, conteneurs, serveurs lancés depuis le terminal | Continuent sans interruption |
| Réglages, historique, PATH, raccourcis | Conservés |

Windows ne permet pas d'écraser un programme en cours d'exécution, mais il permet de le renommer : l'ancienne
version devient `terminal.exe.<…>.old`, la nouvelle prend sa place, et les `.old` sont supprimés au lancement
suivant. Si un téléchargement échoue ou si sa somme ne correspond pas, rien n'est modifié.

Une fois par jour, le terminal vérifie en arrière-plan s'il existe une nouvelle version, et l'affiche à
l'ouverture suivante :

```text
Mise à jour : r43 est disponible, tape apt upgrade terminal (ce qui tourne n'est pas interrompu).
```

`apt update` le signale aussi.

> [!NOTE]
> Une version compilée avec `build.ps1` (version `dev`) n'est jamais remplacée par `apt upgrade` sans le demander.
> `apt upgrade terminal` la remplace par la dernière release.

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
| Accents illisibles (`├á` au lieu de `à`) | Mets le terminal à jour : il passe la console en UTF-8. Pour garder la page de code de Windows : `ORVAEN_NO_UTF8=1` |
| `apt upgrade terminal` affiche « Mise à jour de terminal » puis passe par winget | Ta version date d'avant la mise à jour à chaud : recompile-la une dernière fois (`git pull` puis `build.ps1`) |
| `winget est introuvable` | Installe **App Installer** depuis le Microsoft Store |
| `build.ps1` ne trouve pas `go` | Installe Go, puis ouvre une nouvelle fenêtre |
| Le script est bloqué par la stratégie d'exécution | Lance-le avec `powershell -ExecutionPolicy Bypass -File …` comme ci-dessus |

Pour Docker, voir [Docker léger › En cas de problème](docker.md#en-cas-de-problème).
