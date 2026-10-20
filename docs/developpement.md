<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/terminal-symbole-blanc.svg">
  <img src="brand/terminal-symbole-noir.svg" width="40" alt="">
</picture>

# Développement

[← Retour à l'accueil](../README.md)

Terminal est écrit en **Go**, avec la seule bibliothèque standard : aucune dépendance externe.

## Compiler et tester

```bash
go test ./...                                     # tests (tournent aussi sous Linux et macOS)
go vet ./...
GOOS=windows go build -o terminal.exe ./cmd/terminal
GOOS=windows go build -o apt.exe ./cmd/apt
GOOS=windows go build -o docker.exe ./cmd/docker
```

Sous Windows, `build.ps1` compile les trois exécutables et lance l'installeur.

## Structure

```text
cmd/terminal/       le terminal (shell interactif) : terminal.exe
cmd/apt/            la commande apt seule : apt.exe
cmd/docker/         la commande docker seule : docker.exe
internal/shell/     analyse des lignes, prompt, commandes intégrées
internal/apt/       commandes apt, noms de paquets, winget, Windows Update
internal/docker/    Docker léger : machine Alpine dans WSL2, moteur à la demande
internal/platform/  droits administrateur, élévation UAC, console Windows
internal/ui/        affichage des messages (==>, W:, E:)
docs/               documentation
docs/brand/         logo et visuels, générés par build.py
install.ps1         installeur (copie, PATH, raccourcis, profil Windows Terminal)
build.ps1           compilation + installation
```

Ce qui dépend de Windows est isolé dans des fichiers `_windows.go`, avec un équivalent `_other.go` qui permet de
compiler et de tester ailleurs. Les appels à winget sont remplaçables dans les tests, qui vérifient les commandes
exactes envoyées.

## Intégration continue et releases

À chaque push, la CI GitHub (`.github/workflows/build.yml`) analyse le code, lance les tests et compile les
exécutables pour Windows x64 et ARM64.

Pour publier une version :

```bash
git tag v0.2.0
git push --tags
```

La CI crée alors une release avec deux zips, `x64` et `arm64`, qui contiennent les exécutables, l'icône et
`install.ps1`.

## Visuels

Le logo, l'icône, les bannières, les aperçus et les pictogrammes de la documentation sont générés par un script
Python, à partir des polices Arkerion :

```bash
pip install fonttools pillow
ARKERION_FONTS=../Arkerion-Docs/src/fonts python docs/brand/build.py
```

Sans `ARKERION_FONTS`, le script cherche un dossier `arkerion-docs` ou `Arkerion-Docs` à côté du dépôt.
Le texte est vectorisé : les SVG produits n'ont pas besoin des polices. Pour changer un visuel, modifie
`docs/brand/build.py` puis relance-le, plutôt que d'éditer les SVG à la main.

## Conventions

- Messages, documentation et commentaires en français.
- Une commande qui échoue renvoie un code de sortie non nul et explique quoi faire, au format `E: …`.
- Toute action irréversible (comme `apt remove docker`) demande une confirmation, sauf avec `-y`.
