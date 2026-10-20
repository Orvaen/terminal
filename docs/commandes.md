# Commandes

[← Retour à l'accueil](../README.md)

Le terminal (`terminal.exe`) est un shell : il lit ce que tu tapes, exécute lui-même les commandes intégrées,
et passe tout le reste à Windows.

## Le prompt

```text
dev@PC WIN ~/projets/site
$
```

| Élément | Signification |
|---|---|
| `dev@PC` | Ton nom d'utilisateur et le nom du PC |
| `WIN` | Session normale. Devient `ADMIN` quand le terminal tourne en administrateur |
| `~/projets/site` | Le dossier courant ; `~` est ton dossier personnel, les `\` deviennent `/` |
| `$` | Prêt. Devient `#` en administrateur, et passe en rouge si la dernière commande a échoué |

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
2. sinon, la ligne est confiée à **PowerShell** : ses commandes (`dir`, `Get-Process`…) fonctionnent donc aussi.

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

Pour désactiver ce comportement : variable d'environnement `ORVAEN_NO_UNIX_TOOLS=1`.

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
