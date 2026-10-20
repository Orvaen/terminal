# Commandes

[← Retour à l'accueil](../README.md)

Le terminal (`terminal.exe`) est un shell : il lit ce que tu tapes, exécute lui-même les commandes intégrées,
et passe tout le reste à Windows.

## Le prompt

Le prompt existe en plusieurs styles. `prompt` les affiche tous, avec un aperçu dans ton dossier actuel,
et `prompt <nom>` en choisit un (gardé pour les prochaines fenêtres) :

```text
$ prompt fleche
```

| Style | Aperçu |
|---|---|
| `classique` (par défaut) | `dev@PC WIN ~/projets/site`, puis `$` à la ligne |
| `minimal` | `site ❯` |
| `chemin` | `~/projets/site main ❯` |
| `git` | `dev@PC ~/projets/site (main)`, puis `$` à la ligne, comme Git Bash |
| `fleche` | `╭─ dev dans ~/projets/site sur main à 14:05`, puis `╰─❯` |
| `blocs` | des blocs de couleur ` site ` ` main ` puis `❯`, style powerline sans police spéciale |
| `windows` | `C:\Users\dev\projets\site>`, comme cmd |
| `powershell` | `PS C:\Users\dev\projets\site>` |

Dans tous les styles, le symbole (`$`, `❯`) passe en rouge si la dernière commande a échoué, et devient `#` en
administrateur. La branche Git est lue directement dans `.git` : le prompt reste instantané, même dans un gros dépôt.

### Ton propre prompt

```text
$ prompt perso "{nl}{cyan}{dossier}{/}[[ {violet}{git}{/}]] {symbole-fleche}"
```

| Élément | Affiche |
|---|---|
| `{user}`, `{host}` | Ton nom d'utilisateur, le nom du PC |
| `{dir}` | Le dossier façon Linux : `~/projets/site` |
| `{dossier}` | Seulement le nom du dossier : `site` |
| `{chemin}` | Le chemin Windows : `C:\Users\dev\projets\site` |
| `{git}` | La branche Git (vide hors d'un dépôt) |
| `{heure}` | L'heure : `14:05` |
| `{code}` | Le code de la dernière commande, si elle a échoué |
| `{admin}` | `ADMIN` en administrateur, sinon `WIN` |
| `{symbole}`, `{symbole-fleche}` | `$` ou `❯` (vert ou rouge), `#` en administrateur |
| `{nl}` | Retour à la ligne : la dernière ligne est celle où tu tapes |
| `{rouge}` `{vert}` `{jaune}` `{bleu}` `{violet}` `{cyan}` `{blanc}` `{gris}` `{gras}` | Couleurs du texte |
| `{fond-bleu}` `{fond-violet}` `{fond-vert}` `{fond-rouge}` `{fond-gris}` | Couleurs de fond |
| `{/}` | Revient à la couleur normale |
| `[[ … ]]` | Partie affichée seulement si ses éléments ont une valeur : `[[ ({git})]]` n'affiche rien hors d'un dépôt Git |

`prompt aide` rappelle cette liste. Le choix est enregistré dans `%APPDATA%\OrvaenTerminal\terminal-config.json`.

## Saisie et complétion

**Tab** complète le mot en cours. S'il y a plusieurs possibilités, il complète la partie commune ; un second **Tab**
les affiche toutes.

| Après | Tab propose |
|---|---|
| le début de la ligne | les commandes intégrées et les programmes du PATH |
| `apt` | les sous-commandes (`install`, `remove`, `upgrade`…) |
| `apt remove`, `purge`, `upgrade`, `show` | les logiciels installés : noms courts (`node`) et identifiants winget (`Spotify.Spotify`) |
| `apt install` | les noms courts connus (`git`, `node`, `docker`…) |
| `docker`, `docker engine`, `docker compose` | leurs sous-commandes |
| `run` | les scripts de `terminal.json` |
| `prompt` | les styles de prompt |
| `path` | ses sous-commandes ; après `path qui`, les commandes |
| `cd`, `rmdir` | les dossiers |
| tout le reste | les fichiers et dossiers ; un nom avec une espace est mis entre guillemets |

La liste des logiciels installés est lue auprès de winget dès que tu commences à taper `apt`, puis gardée
quelques minutes. Seuls les logiciels que winget connaît y figurent.

| Touche | Effet |
|---|---|
| ← → , Ctrl+← Ctrl+→ | Déplace le curseur d'un caractère, d'un mot |
| Début, Fin (ou Ctrl+A, Ctrl+E) | Début, fin de la ligne |
| ↑ ↓ | Commandes précédentes, gardées d'une session à l'autre |
| Ctrl+W, Ctrl+U, Ctrl+K | Efface le mot précédent, le début, la fin de la ligne |
| Échap | Efface la ligne |
| Ctrl+L | Efface l'écran |
| Ctrl+C | Abandonne la ligne en cours |
| Ctrl+D (ligne vide) | Quitte le terminal |

Une ligne qui commence par une espace n'est pas gardée dans l'historique.

## Commandes intégrées

| Commande | Rôle |
|---|---|
| `apt <commande> [paquets]` | Logiciels et mises à jour Windows, voir [apt](apt.md) |
| `sudo <commande>` | Exécute en administrateur, voir [plus bas](#sudo) |
| `docker`, `docker-compose` | Docker léger, voir [Docker](docker.md) |
| `req <fichier.http>`, `req nouveau` | Requêtes HTTP, voir [req](req.md) |
| `run [script]`, `run -p <a> <b>` | Scripts du projet (`terminal.json`), en parallèle avec `-p`, voir [run](scripts.md) |
| `ls [-a] [-l] [-h] [-1]`, `ll` | Liste un dossier. Les fichiers cachés de Windows sont masqués sans `-a` |
| `cd [dossier]` | Change de dossier. `cd` seul : dossier perso ; `cd -` : dossier précédent ; accepte `/c/Users/...` comme Git Bash |
| `pwd` | Affiche le dossier courant |
| `cat <fichier...>` | Affiche le contenu de fichiers |
| `touch <fichier...>` | Crée un fichier vide ou met à jour sa date |
| `mkdir [-p] <dossier...>` | Crée des dossiers (`-p` : avec les dossiers parents) |
| `rm [-r] [-f] <chemin...>` | Supprime. `-r` pour les dossiers, `-f` ignore les fichiers absents |
| `rmdir <dossier...>` | Supprime des dossiers vides |
| `cp [-r] <source...> <destination>` | Copie des fichiers, ou des dossiers avec `-r` |
| `mv <source...> <destination>` | Déplace ou renomme |
| `echo [-n] [texte]` | Affiche du texte |
| `clear` | Efface l'écran |
| `which <commande>` | Indique si une commande est intégrée, ou où se trouve le programme |
| `export NOM=valeur`, `unset NOM`, `env` | Variables d'environnement |
| `open [chemin]` | Ouvre un dossier ou un fichier dans l'Explorateur Windows |
| `help`, `exit [code]` | Aide, quitter |

> [!IMPORTANT]
> `rm` refuse de supprimer la racine d'un disque (`C:\`) et ton dossier personnel, même avec `-rf`.

## Enchaîner des commandes

| Opérateur | Effet | Exemple |
|---|---|---|
| `&&` | La suivante ne s'exécute que si la précédente a réussi | `mkdir site && cd site` |
| `\|\|` | La suivante ne s'exécute que si la précédente a échoué | `cd build \|\| mkdir build` |
| `;` | Les deux s'exécutent, dans l'ordre | `clear; ls` |

Les guillemets simples et doubles protègent les espaces : `cp "mon fichier.txt" ~/docs`.

## Tubes et redirections

Le terminal relie lui-même les commandes, comme bash :

| Syntaxe | Effet | Exemple |
|---|---|---|
| `a \| b` | La sortie de `a` devient l'entrée de `b` | `cat noms.txt \| sort \| uniq -c` |
| `> fichier` | Écrit la sortie dans un fichier (le remplace) | `ls > liste.txt` |
| `>> fichier` | Ajoute à la fin du fichier | `echo fini >> log.txt` |
| `< fichier` | Lit l'entrée depuis un fichier | `sort < noms.txt` |
| `2> fichier`, `2>> fichier` | Écrit les erreurs dans un fichier | `make 2> erreurs.log` |
| `2>&1` | Envoie les erreurs avec la sortie | `npm test > test.log 2>&1` |
| `&> fichier` | Sortie et erreurs dans le même fichier | `make &> build.log` |

Les commandes intégrées y participent : `ls | grep txt` (un nom par ligne, comme sous Linux), `echo … > f`,
`cat < f`. Les fichiers sont écrits en UTF-8. Le code de sortie est celui de la dernière commande du tube.

PowerShell prend le relais pour ce que le terminal ne gère pas : une variable (`$env:PATH`), une commande entre
`` ` ``, un `&` seul, ou une commande PowerShell dans le tube (`Get-Process | Select -First 3`, `dir | …`).

## Tout le reste

Quand ce que tu tapes n'est pas une commande intégrée :

1. si c'est un **programme Windows** (`git`, `code`, `node`, `notepad`…), il est lancé directement ;
2. s'il a été **installé depuis l'ouverture du terminal** (`apt install hurl`, un installeur…), le terminal relit le
   PATH de Windows et le trouve : pas besoin de rouvrir la fenêtre. Si l'installeur n'a pas ajouté son dossier au
   PATH, le terminal le cherche dans le dossier d'installation enregistré par Windows ;
3. si c'est une **commande PowerShell** (`dir`, `Get-Process`…), la ligne est confiée à PowerShell ;
4. sinon, un message court, comme sous Linux, avec la commande pour l'installer :

```text
$ hurl tests/00-games.hurl
hurl : commande introuvable
Pour l'installer : apt install hurl   (Orange-OpenSource.Hurl)
```

Pour reconnaître les commandes PowerShell sans le lancer à chaque fois, le terminal garde leur liste dans un
fichier, rafraîchi chaque semaine.

Les lignes qui contiennent une variable (`$env:...`) sont aussi confiées à PowerShell, voir
[Tubes et redirections](#tubes-et-redirections). Elles prennent alors une demi-seconde de plus, le temps que
PowerShell démarre.

## Coreutils de Microsoft

[Coreutils pour Windows](https://learn.microsoft.com/windows/core-utils/commands) apporte 78 commandes Unix
natives (d'après [uutils](https://uutils.github.io/)) : `grep`, `head`, `tail`, `wc`, `sort`, `uniq`, `cut`,
`tr`, `find`, `xargs`, `tee`, `du`, `df`, `sha256sum`, `base64`, `seq`…

```text
$ apt install coreutils
```

Elles marchent tout de suite, sans rouvrir le terminal. Avec elles :

| | Ce que fait le terminal |
|---|---|
| PATH | Le dossier de Coreutils passe **en tête** : `find`, `sort`, `date`, `hostname` sont ceux de Linux, pas ceux de Windows (comme dans Git Bash) |
| Commandes intégrées | `ls`, `cat`, `rm`, `cp`, `mv`, `mkdir`, `rmdir`, `touch`, `echo` restent celles du terminal pour les usages courants, et passent la main à Coreutils dès qu'une option leur manque : `ls --color -lt`, `cat -n`, `rm -i`, `echo -e`, `env VAR=x commande` |
| Protection | `rm` refuse toujours de supprimer la racine d'un disque ou ton dossier personnel, même via Coreutils |
| Tubes | `find . -name "*.log" \| xargs rm`, `cat f \| grep x \| wc -l` : de vrais tubes, sans les alias PowerShell qui remplacent `cat`, `ls` ou `sort` dans PowerShell |

Pour désactiver ce comportement : variable d'environnement `ORVAEN_NO_COREUTILS=1`. Pour retirer une commande
précise : `coreutils-manager disable <commande>`.

## Outils Linux de Git et Makefile

Si Git pour Windows est installé (`apt install git`), le terminal ajoute ses outils Linux à la fin du PATH :
`sh`, `bash`, `grep`, `sed`, `awk`, `id`, `make` s'il y est… Comme dans Git Bash, `make` trouve alors un shell
Unix et les recettes des Makefile (`mkdir -p`, `[ … ]`, `$(shell id -u)`) fonctionnent. Les commandes Windows
du même nom (`find`, `sort`) restent prioritaires.

Le terminal règle aussi la variable `SHELL` sur ce `sh`, comme Git Bash. Sous Windows, GNU make s'en sert pour
exécuter les recettes : même un ancien `make` qui passe en premier dans le PATH (celui de Free Pascal, par exemple)
utilise alors `sh` au lieu de cmd.exe, qui ne comprend pas `{ … }` ni `[ … ]`. Si tu as déjà défini `SHELL`, il n'y
touche pas.

> [!TIP]
> Un `make` ancien (3.x) abîme aussi les accents des Makefile en UTF-8 (`dÃ©marre`). `which make` montre lequel est
> utilisé ; `apt install make` installe un GNU Make récent.

Pour désactiver ce comportement : variable d'environnement `ORVAEN_NO_UNIX_TOOLS=1`.

## Ranger le PATH : path

Quand deux programmes portent le même nom (deux `make.exe`, deux `python.exe`), c'est le premier dossier du PATH
qui gagne. Un vieux logiciel peut ainsi passer devant sans prévenir : le GNU Make 3.82 de Free Pascal devant un make
récent, par exemple.

```text
$ path qui make
→  1. C:\FPC\3.2.2\bin\i386-Win32\make.exe
      GNU Make 3.82
   2. C:\Users\dev\AppData\Local\Microsoft\WinGet\Links\make.exe
      GNU Make 4.4.1

C'est le n°1 qui est utilisé. Pour utiliser le n°2 : path monter make 2
```

| Commande | Effet |
|---|---|
| `path` | Les dossiers du PATH, dans l'ordre, avec leur origine (Windows système, utilisateur, ajouté par le terminal), les doublons et les dossiers disparus |
| `path qui <commande>` | Toutes les versions d'un programme, avec leur version ; la première gagne |
| `path conflits` | Les outils de développement présents en plusieurs versions (make, python, node, gcc, java…) |
| `path monter <n°\|dossier>` | Met ce dossier en tête |
| `path monter <commande> <n°>` | Met en tête le dossier de cette version : `path monter make 2` |
| `path masquer <n°\|dossier>` | Retire ce dossier |
| `path reset` | Oublie l'ordre choisi |
| `path nettoyer [-y]` | Retire de Windows les doublons et les dossiers disparus, après confirmation |

`monter` et `masquer` changent l'ordre **pour le terminal et les programmes qu'il lance** (make, npm, scripts
`run`…), sans toucher à Windows ni demander de droits. Le choix est enregistré dans `terminal-config.json` et
appliqué à chaque nouvelle fenêtre, après les ajouts du terminal (Coreutils, outils de Git).

`path nettoyer` modifie le PATH de Windows : le PATH utilisateur, et aussi le PATH système avec
`sudo path nettoyer`. Une entrée du PATH utilisateur déjà présente dans le PATH système compte comme un doublon.
Les variables comme `%SystemRoot%` ou `%USERPROFILE%` sont gardées telles quelles.

## sudo

| Commande | Ce qui se passe |
|---|---|
| `sudo apt install …`, `sudo apt upgrade` | Pas besoin d'être administrateur : winget ouvre lui-même la fenêtre UAC quand un installeur le demande |
| `sudo docker …` | Pas besoin non plus : le moteur tourne dans WSL |
| `sudo apt full-upgrade` et toute autre commande | Relancée en administrateur |

Pour qu'une commande relancée en administrateur s'exécute **dans la même fenêtre**, active le sudo de Windows 11
(version 24H2 ou plus récente), une fois, dans un terminal administrateur :

```powershell
sudo config --enable normal
```

Ou par *Paramètres → Système → Pour les développeurs → Activer sudo*. Sans lui, une fenêtre administrateur s'ouvre,
exécute la commande et attend que tu appuies sur Entrée avant de se fermer.

## Lancer une seule commande

```powershell
terminal -c "apt update && apt upgrade"
```

Exécute la ligne puis quitte, avec le code de sortie de la dernière commande. Pratique dans un script ou un raccourci.

## Limites actuelles

- Pas de variables façon Linux (`$HOME`) : une ligne qui contient `$` est confiée à PowerShell, qui utilise `$env:HOME`.
- Pas de `&` pour lancer une commande en arrière-plan : utilise `run -p` (voir [run](scripts.md)).
