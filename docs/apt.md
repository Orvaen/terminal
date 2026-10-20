# apt

[← Retour à l'accueil](../README.md)

`apt` reprend les commandes du gestionnaire de paquets de Debian et Ubuntu, mais s'appuie sur **winget**,
le gestionnaire de paquets intégré à Windows, et sur **Windows Update** pour le système. Il est intégré au terminal,
et `apt.exe` permet de l'utiliser aussi depuis PowerShell ou cmd.

## Commandes

| Commande | Ce que ça fait |
|---|---|
| `apt update` | Rafraîchit les sources et liste les mises à jour disponibles |
| `apt upgrade` | Met à jour tous les logiciels (et le Docker léger s'il est installé), puis Orvaen Terminal |
| `apt upgrade <paquet...>` | Met à jour seulement les logiciels indiqués |
| `apt upgrade terminal` | Met à jour Orvaen Terminal, sans fermer ce qui tourne (voir [Installation › Mettre à jour](installation.md#mettre-à-jour)) |
| `apt full-upgrade` | `upgrade`, puis les mises à jour Windows (alias `dist-upgrade`) |
| `apt install <paquet...>` | Installe un ou plusieurs logiciels |
| `apt remove <paquet...>` | Désinstalle (alias `purge`, `uninstall`) |
| `apt search <mot>` | Cherche un logiciel |
| `apt show <paquet>` | Affiche les détails d'un logiciel |
| `apt list` | Liste les logiciels installés |
| `apt list --upgradable` | Liste ceux qui ont une mise à jour |
| `apt windows-update` | Installe uniquement les mises à jour Windows (administrateur) |
| `apt help`, `apt version` | Aide, version |

`apt-get` est un synonyme d'`apt`. L'option `-y` est acceptée : les accords de licence sont acceptés automatiquement.
`autoremove` et `clean` existent mais n'ont rien à faire sous Windows.

## Noms de paquets

Les noms courts habituels sont traduits en identifiants winget exacts :

| Nom | Identifiant winget |
|---|---|
| `git` | `Git.Git` |
| `node`, `nodejs` | `OpenJS.NodeJS.LTS` |
| `node-current` | `OpenJS.NodeJS` (version « Current », hors LTS) |
| `python`, `python3` | `Python.Python.3.12` |
| `vscode`, `code` | `Microsoft.VisualStudioCode` |
| `go`, `golang` | `GoLang.Go` |
| `rust`, `rustup` | `Rustlang.Rustup` |
| `gh` | `GitHub.cli` |
| `pwsh` | `Microsoft.PowerShell` |
| `neovim`, `nvim` | `Neovim.Neovim` |
| `chrome`, `firefox` | `Google.Chrome`, `Mozilla.Firefox` |
| `discord`, `vlc`, `7zip`, `curl` | `Discord.Discord`, `VideoLAN.VLC`, `7zip.7zip`, `cURL.cURL` |
| `terminal` | `Microsoft.WindowsTerminal` |
| `docker` | Le [Docker léger](docker.md), sans passer par winget |
| `docker-desktop` | `Docker.DockerDesktop` |

Tout autre nom est transmis tel quel à winget. Pour trouver l'identifiant exact d'un logiciel :

```text
$ apt search spotify
$ apt install Spotify.Spotify
```

La liste est définie dans [`internal/apt/aliases.go`](../internal/apt/aliases.go).

## Sources officielles

Le catalogue winget est tenu par la communauté et peut avoir quelques jours de retard sur les éditeurs.
Pour **Node.js**, `apt` compare donc aussi ta version avec [nodejs.org](https://nodejs.org) :

- `apt update` et `apt list --upgradable` signalent une version plus récente, même si winget ne la connaît pas encore ;
- `apt upgrade` (ou `apt upgrade node`) l'installe directement depuis nodejs.org, après vérification de sa somme SHA-256 ;
- `apt` reste sur ta version majeure (par exemple 24.x) : une nouvelle majeure (26.x) est signalée, jamais installée
  sans que tu le demandes.

```text
==> Depuis les sources officielles
node  v24.20.0 → v24.22.0  (nodejs.org)  : apt upgrade node
node  v26.12.0 Current disponible (hors LTS) : apt install node-current
```

## Droits administrateur

`apt install`, `upgrade` et `remove` n'ont pas besoin d'être lancés en administrateur : quand un installeur en a
besoin, Windows affiche sa fenêtre de confirmation (UAC). Seules les **mises à jour Windows** (`full-upgrade`,
`windows-update`) demandent un terminal administrateur ; si tu n'en es pas un, `apt` en ouvre un pour cette étape.

Les mises à jour Windows passent par l'API officielle de Windows Update : aucun module à installer. Si un redémarrage
est nécessaire, `apt` te le signale.

## Messages

`apt` écrit comme son modèle Linux :

```text
==> Installation de git      une étape
W: Un redémarrage est nécessaire pour terminer l'installation.
E: précise au moins un paquet : apt install <nom>
```

`W:` signale un avertissement, `E:` une erreur. Quand plusieurs paquets sont demandés, `apt` continue après un
échec et récapitule à la fin ceux qui n'ont pas pu être traités.
