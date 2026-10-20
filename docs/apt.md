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
| `apt alias [nom identifiant]` | Tes noms de paquets, voir [plus bas](#tes-noms--apt-alias) |
| `apt help`, `apt version` | Aide, version |

`apt-get` est un synonyme d'`apt`. L'option `-y` est acceptée : les accords de licence sont acceptés automatiquement.
`autoremove` et `clean` existent mais n'ont rien à faire sous Windows.

## Noms de paquets

Un même logiciel ne porte pas le même nom partout : VS Code s'appelle `code` sous Ubuntu,
`visual-studio-code` dans Homebrew et `Microsoft.VisualStudioCode` dans winget. `apt` accepte les noms Linux
et trouve le bon logiciel, dans cet ordre :

| Étape | Exemple |
|---|---|
| 1. **Tes noms**, créés avec `apt alias` ou retenus après un choix | `apt alias code Microsoft.VisualStudioCode` |
| 2. **La table des noms connus** : 236 noms Debian, Ubuntu, Arch et Homebrew pour 130 logiciels | `fd-find` → `sharkdp.fd`, `openjdk-21-jdk` → `EclipseAdoptium.Temurin.21.JDK`, `google-chrome-stable` → `Google.Chrome` |
| 3. **Le nom court déclaré dans winget** (moniker) | `vscode` → `Microsoft.VisualStudioCode` |
| 4. **La commande fournie** : sous Linux, le paquet porte souvent le nom de sa commande | `mytool` → le logiciel qui installe `mytool.exe` |
| 5. **Une recherche winget** : un résultat évident est pris directement, sinon tu choisis | voir plus bas |

`apt` affiche toujours ce qu'il a compris avant d'installer :

```text
$ apt install fd-find
==> Installation de fd-find
fd-find → sharkdp.fd
```

Quand plusieurs logiciels correspondent, `apt` te les propose, et **retient ton choix** pour la prochaine fois :

```text
$ apt install thing
Plusieurs logiciels correspondent à « thing » :
  1. A.Thing                          Thing A
  2. B.Thing                          Thing B
Lequel installer ? [1-2, Entrée pour annuler] 2
Retenu : « apt install thing » installera B.Thing. Pour changer : apt alias thing <identifiant>
```

Avec `-y`, ou hors d'une console (script, `run`), `apt` ne devine jamais : il affiche les choix et s'arrête.
Un identifiant winget complet marche toujours : `apt install Spotify.Spotify`.

### Tes noms : apt alias

| Commande | Effet |
|---|---|
| `apt alias` | Liste tes noms |
| `apt alias <nom> <identifiant>` | `apt install <nom>` installera ce logiciel ; passe avant la table |
| `apt alias -r <nom>` | Oublie un nom |

Ils sont rangés dans `%APPDATA%\OrvaenTerminal\paquets.json`, que tu peux aussi modifier à la main.

### Déjà dans Windows

Certains paquets Linux n'ont rien à installer sous Windows, ou s'installent autrement. `apt` l'explique au lieu
d'installer autre chose :

| Paquet | Ce qu'`apt` répond |
|---|---|
| `build-essential`, `gcc`, `g++` | Les outils de compilation : `Microsoft.VisualStudio.2022.BuildTools` (MSVC) ou `MSYS2.MSYS2` (gcc) |
| `openssh-client`, `ssh` | OpenSSH est intégré à Windows |
| `tree`, `tar` | Intégrés à Windows |
| `zip`, `unzip` | `tar -a -cf archive.zip dossier` et `tar -xf archive.zip`, intégrés à Windows |

`docker`, `docker.io`, `docker-ce` et `docker-compose` installent le [Docker léger](docker.md), sans passer par winget.
`python` installe la dernière version de Python (3.14), `java` et `default-jdk` le JDK Temurin 25 (LTS).

La table est dans [`internal/apt/aliases.go`](../internal/apt/aliases.go). Ses identifiants ont été vérifiés dans
le dépôt des manifestes winget ; si l'un d'eux disparaît de winget, `apt` cherche automatiquement un équivalent.

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
