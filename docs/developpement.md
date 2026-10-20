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
GOOS=windows go build -o req.exe ./cmd/req
```

Sous Windows, `build.ps1` compile les quatre exécutables et lance l'installeur.

## Structure

```text
cmd/terminal/       le terminal (shell interactif) : terminal.exe
cmd/apt/            la commande apt seule : apt.exe
cmd/docker/         la commande docker seule : docker.exe
cmd/req/            la commande req seule : req.exe
internal/shell/     analyse des lignes, prompt, commandes intégrées
internal/apt/       commandes apt, noms de paquets, winget, Windows Update
internal/docker/    Docker léger : machine Alpine dans WSL2, moteur à la demande
internal/req/       req : fichiers .http, requêtes, vérifications
internal/scripts/   run : scripts de terminal.json, en séquence ou en parallèle
internal/update/    mise à jour du terminal depuis les releases GitHub (apt upgrade terminal)
internal/setup/     profil du terminal intégré de VS Code (terminal --setup-vscode)
internal/platform/  droits administrateur, élévation UAC, console Windows
internal/ui/        affichage des messages (==>, W:, E:)
docs/               documentation
docs/brand/         logo, généré par build.py
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
`install.ps1`, plus `SHA256SUMS.txt`.

Chaque push sur `main` publie aussi une release automatique, `r<numéro>` (le numéro du run de la CI) :
c'est elle que `apt upgrade terminal` installe. La CI garde les 10 dernières releases `r<numéro>` et supprime
les plus anciennes ; les versions `v*` ne sont jamais supprimées.

Le code de la mise à jour est dans `internal/update/` : dernière release via l'API GitHub, vérification de la
somme, puis remplacement des exécutables par renommage (`.old`), avec retour en arrière en cas d'erreur.

## Logo

Le logo, l'icône et la carte de partage sont générés par un script Python, à partir de la police Arkerion Sans :

```bash
pip install fonttools pillow
ARKERION_FONTS=../Arkerion-Docs/src/fonts python docs/brand/build.py
```

Sans `ARKERION_FONTS`, le script cherche un dossier `arkerion-docs` ou `Arkerion-Docs` à côté du dépôt.

## Conventions

- Messages, documentation et commentaires en français.
- Une commande qui échoue renvoie un code de sortie non nul et explique quoi faire, au format `E: …`.
- Toute action irréversible (comme `apt remove docker`) demande une confirmation, sauf avec `-y`.
