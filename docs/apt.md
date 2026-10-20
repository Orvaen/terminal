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
| `apt deps` | Outils dont le projet a besoin, et ce qui manque, voir [plus bas](#les-outils-dun-projet--apt-deps) |
| `apt deps install` | Installe ce qui manque, puis les dépendances du projet (alias `apt build-dep`) |
| `apt alias [nom identifiant]` | Tes noms de paquets, voir [plus bas](#tes-noms--apt-alias) |
| `apt help`, `apt version` | Aide, version |

`apt update` affiche chaque source avec sa durée, puis un seul tableau des mises à jour, qu'elles viennent de
winget, de nodejs.org ou des releases d'Orvaen Terminal :

```text
==> Mise à jour des sources
  ✓ msstore        0,4 s
  ✓ winget         1,2 s
  ✓ winget-font    0,3 s

==> Mises à jour disponibles
  3 logiciel(s) à mettre à jour

  Epic Online Services                        4.2.0    → 4.3.1       apt upgrade EpicGames.EpicOnlineServices
  Microsoft OLE DB Driver 19 pour SQL Server  19.4.1.0 → 19.4.3      apt upgrade Microsoft.SQLServer.OLEDBDriver
  Orvaen Terminal                             r24      → r25         apt upgrade terminal

Tout mettre à jour : apt upgrade    Un seul : apt upgrade <nom>
```

Les téléchargements faits par `apt` lui-même (Orvaen Terminal, Node.js, Composer, Maven, Gradle, Alpine pour
Docker) affichent une barre de progression avec la taille, le débit et le temps restant. Quand la sortie n'est pas
une console (script, tube vers un fichier), rien n'est animé : une seule ligne par étape.

`apt-get` est un synonyme d'`apt`. L'option `-y` est acceptée : les accords de licence sont acceptés automatiquement.
`autoremove` et `clean` existent mais n'ont rien à faire sous Windows.

## Les outils d'un projet : apt deps

Dans un projet que tu viens de cloner, `apt deps` lit ses fichiers et liste les outils nécessaires, avec la
version demandée et celle que tu as :

```text
$ apt deps
Projet : C:\Users\dev\Desktop\j1-express

  ✓ Git                2.47.1
                       .git, Makefile (sh des recettes)
  ✓ Docker             Docker léger
                       Makefile, docker-compose.yml
  ✗ make               manquant → apt install make
                       Makefile
  ! Node.js            18.20.4, trop ancien (≥ 22 voulu par api/.nvmrc)
                       api/package.json, api/.nvmrc
  ✗ Hurl               manquant → apt install hurl
                       tests/*.hurl

Dépendances du projet :
  api : npm ci

Pour installer ce qui manque (3 outil(s)) : apt deps install
```

`apt deps install` (ou `apt build-dep`, comme sous Debian) installe les outils manquants ou trop anciens, les rend
utilisables tout de suite (sans rouvrir le terminal), puis propose d'installer les dépendances du projet
(`npm ci`, `go mod download`…). Avec `-y`, il ne pose pas de question. `apt deps <dossier>` analyse un autre dossier.

Le projet et ses sous-dossiers sont parcourus sur 3 niveaux (`api/`, `web/`, `services/x/`…), sans entrer dans
`node_modules`, `vendor`, `target`, `.venv` et les dossiers cachés.

| Fichier | Outils | Version lue dans | Dépendances du projet |
|---|---|---|---|
| `package.json` | Node.js ; pnpm, Yarn ou Bun selon le fichier de verrouillage ou `packageManager` | `engines.node`, `volta.node`, `.nvmrc`, `.node-version` | `npm ci`, `pnpm install`, `yarn install`, `bun install` |
| `deno.json` | Deno | | |
| `go.mod` | Go | la ligne `go 1.23` | `go mod download` |
| `Cargo.toml`, `rust-toolchain.toml` | Rust | `rust-version`, `channel` | `cargo fetch` |
| `pyproject.toml`, `requirements.txt`, `setup.py`, `Pipfile` | Python ; uv ou Poetry s'ils sont utilisés | `requires-python`, `.python-version`, `runtime.txt` | `uv sync`, `poetry install`, ou un `.venv` avec `pip install -r requirements.txt` |
| `pom.xml`, `build.gradle` | Java (JDK) ; Maven ou Gradle si le projet n'a pas de wrapper (`mvnw`, `gradlew`) | `maven.compiler.release`, `java.version`, `JavaLanguageVersion.of(…)` | |
| `*.csproj`, `*.sln`, `global.json` | .NET SDK | `sdk.version` de `global.json` | `dotnet restore` |
| `composer.json` | PHP, Composer | `require.php` | `composer install` |
| `Gemfile`, `.ruby-version` | Ruby | `.ruby-version` | `bundle install` |
| `Makefile` | make, et Git (son `sh` exécute les recettes) ; Docker si les recettes l'utilisent | | |
| `Dockerfile`, `compose.yaml`, `docker-compose.yml` | Docker (le [Docker léger](docker.md)) | | |
| `CMakeLists.txt` | CMake, un compilateur C/C++ (gcc, clang ou MSVC ; gcc s'il n'y en a aucun) | | |
| `*.tf`, `Chart.yaml`, `*.hurl` | Terraform, Helm, Hurl | | |
| `mix.exs`, `pubspec.yaml`, `Package.swift`, `build.sbt`, `stack.yaml`, `cpanfile` | Elixir, Dart, Swift, sbt, Stack, Perl | | |
| `.tool-versions`, `mise.toml` | les outils listés | leurs versions | |
| `.github/workflows/*.yml` (CI) | ceux des actions `setup-node`, `setup-python`, `setup-go`, `setup-java`, `setup-dotnet`, `setup-ruby`, `setup-php`, `pnpm/action-setup`, `setup-bun`, `setup-deno`, `rust-toolchain`, `setup-terraform`, `setup-helm`, `setup-uv`, `setup-msbuild`… | `node-version`, `python-version`… (une matrice `[18, 20, 22]` compte pour la plus récente) et `node-version-file`, `go-version-file`… | |
| `.devcontainer/devcontainer.json` | les *features* (`node`, `python`, `go`, `java`, `dotnet`, `rust`, `docker-in-docker`…) et l'image `mcr.microsoft.com/devcontainers/…` (aussi dans le `Dockerfile` du devcontainer) | l'option `version` et l'étiquette de l'image (`typescript-node:1-22` → Node 22) | |

Une version demandée est un **minimum** : `engines.node` à `>=20` est satisfait par Node 24. Si plusieurs
fichiers demandent une version, la plus exigeante l'emporte.

Sauf pour Node quand le projet **épingle** sa version : `.nvmrc`, `.node-version`, ou une version exacte dans la
CI (`node-version: 22.11.0`). Le projet est alors construit avec cette version majeure, et une autre peut casser
la compilation de ses modules natifs. `apt deps` le signale (`!`) mais ne remplace pas ton Node, dont tes autres
projets dépendent ; il te propose plutôt nvm, pour avoir les deux versions côte à côte.

### Modules Node natifs

Certaines dépendances Node sont écrites en C/C++ (`node-pty`, `sqlite3`, `bcrypt`, `kerberos`…) et sont compilées
par node-gyp pendant `npm install` quand aucun binaire précompilé n'existe. Sous Windows, il faut alors **Python
et le compilateur MSVC**. Sans eux, l'installation échoue au bout de plusieurs minutes avec une erreur peu claire.

`apt deps` les repère avant, et réclame Python et MSVC (`apt install build-essential`) :

| Où | Comment |
|---|---|
| `node_modules/` déjà présent | les dépendances qui contiennent un `binding.gyp` |
| sinon `package-lock.json` | les dépendances avec un script d'installation qui utilisent `node-addon-api`, `nan`, `bindings`, `node-gyp` ou `prebuild-install` ; celles qui ne livrent que des binaires précompilés (`node-gyp-build`), celles réservées à un autre système (`fsevents` pour macOS) et les scripts connus pour ne rien compiler (husky, esbuild, electron, playwright…) sont ignorées |
| le projet lui-même | un `binding.gyp` à sa racine |

Avec pnpm ou Yarn, seule la première méthode marche : lance `apt deps` après un premier `pnpm install` si
l'installation a échoué.

### Ce que seule la CI demande

Un outil demandé **uniquement** par la CI sert souvent à un job secondaire (tests d'une extension, publication…).
`apt deps` le marque d'un `○` et `apt deps install` ne l'installe pas, sauf avec `apt deps install --ci`.

Sur le dépôt de VS Code, par exemple, `apt deps` trouve : Node 24 épinglé par `.nvmrc`, Python et MSVC pour ses
20 modules natifs, Rust (devcontainer), .NET demandé seulement par la CI, et `npm ci` dans deux dossiers. Pour Java et .NET, la version choisit le paquet :
`maven.compiler.release` à 17 installe le JDK 17, `global.json` en 8.0 installe le SDK .NET 8.

Composer, Maven, Gradle et Poetry sont installés par `apt` lui-même (voir
[Installés par apt lui-même](#installés-par-apt-lui-même)). Pour le C/C++, `apt deps` installe gcc ; MSVC, s'il est
déjà installé (`apt install build-essential`), est reconnu même s'il n'est pas dans le PATH.

## Noms de paquets

Un même logiciel ne porte pas le même nom partout : VS Code s'appelle `code` sous Ubuntu,
`visual-studio-code` dans Homebrew et `Microsoft.VisualStudioCode` dans winget. `apt` accepte les noms Linux
et trouve le bon logiciel, dans cet ordre :

| Étape | Exemple |
|---|---|
| 1. **Tes noms**, créés avec `apt alias` ou retenus après un choix | `apt alias code Microsoft.VisualStudioCode` |
| 2. **La table des noms connus** : 264 noms Debian, Ubuntu, Arch et Homebrew pour 143 logiciels, voir la [liste complète](paquets.md) | `fd-find` → `sharkdp.fd`, `openjdk-21-jdk` → `EclipseAdoptium.Temurin.21.JDK`, `google-chrome-stable` → `Google.Chrome` |
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

### Installés par apt lui-même

Composer, Maven, Gradle et Poetry ne sont pas dans winget : `apt install` les télécharge depuis leur source
officielle (avec vérification de la somme de contrôle quand l'éditeur en publie une), les range dans
`%LOCALAPPDATA%\OrvaenTerminal\outils` et ajoute leur dossier à ton PATH. Ils marchent tout de suite dans le
terminal. `apt upgrade` les met à jour, `apt remove` les désinstalle.

| Paquet | A besoin de |
|---|---|
| `composer` | PHP (`apt install php`) |
| `maven` (ou `mvn`), `gradle` | Java (`apt install java`) |
| `poetry` | Python (`apt install python`) |

Pour compiler du C ou du C++ :

| Paquet | Ce qui est installé |
|---|---|
| `gcc`, `g++`, `gdb` | GCC de [WinLibs](https://winlibs.com/) (MinGW-w64), avec gdb et CMake, utilisables tout de suite dans le terminal |
| `build-essential` (ou `msvc`) | Le compilateur MSVC de Microsoft : les Build Tools de Visual Studio, **avec la charge de travail C++ et les bibliothèques Spectre-mitigated déjà cochées**. `cl` s'utilise depuis « Developer PowerShell for VS », et CMake et node-gyp le trouvent tout seuls |
| `msvc-spectre` | Ajoute les bibliothèques Spectre-mitigated à un MSVC déjà installé (voir plus bas) |

Tu n'as jamais besoin d'ouvrir Visual Studio Installer : `apt` le pilote en ligne de commande.

**Les bibliothèques « Spectre-mitigated »** sont les versions du runtime C++ de MSVC compilées avec la protection
contre la faille Spectre (option `/Qspectre`). Elles ne sont pas installées par défaut avec Visual Studio, et un
projet qui active la protection (VS Code et ses modules natifs, par exemple) échoue sans elles avec l'erreur
`MSB8040: Spectre-mitigated libraries are required for this project`. `apt install build-essential` les inclut ;
pour un MSVC installé autrement, `apt install msvc-spectre` les ajoute. Et si `npm ci` échoue avec MSB8040
pendant `apt deps install`, `apt` l'explique, installe les bibliothèques et relance l'étape tout seul.

Certains paquets Linux n'ont rien à installer sous Windows (`ssh`, `tree`, `tar`, `zip`…) : `apt` l'explique au lieu
d'installer autre chose.

`docker`, `docker.io`, `docker-ce` et `docker-compose` installent le [Docker léger](docker.md), sans passer par winget.
`python` installe la dernière version de Python (3.14), `java` et `default-jdk` le JDK Temurin 25 (LTS).

La liste de tous les noms connus est dans [Paquets connus](paquets.md), générée depuis
[`internal/apt/aliases.go`](../internal/apt/aliases.go). Ses identifiants ont été vérifiés dans
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
