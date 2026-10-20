<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/terminal-symbole-blanc.svg">
  <img src="brand/terminal-symbole-noir.svg" width="40" alt="">
</picture>

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

L'historique fonctionne avec les flèches ↑ et ↓, et F7 affiche la liste des commandes précédentes.

## Commandes intégrées

| Commande | Rôle |
|---|---|
| `apt <commande> [paquets]` | Logiciels et mises à jour Windows, voir [apt](apt.md) |
| `sudo <commande>` | Exécute en administrateur, voir [plus bas](#sudo) |
| `docker`, `docker-compose` | Docker léger, voir [Docker](docker.md) |
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

## Tout le reste

Quand ce que tu tapes n'est pas une commande intégrée :

1. si c'est un **programme Windows** (`git`, `code`, `node`, `notepad`…), il est lancé directement ;
2. sinon, la ligne est confiée à **PowerShell** : ses commandes (`dir`, `Get-Process`…) fonctionnent donc aussi.

Les lignes qui contiennent un tube (`|`), une redirection (`>`, `<`) ou une variable (`$env:...`) sont aussi
confiées à PowerShell. Elles prennent alors une demi-seconde de plus, le temps que PowerShell démarre.

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

- Pas encore de complétion avec Tab.
- Pas de variables façon Linux (`$HOME`) : une ligne qui contient `$` est confiée à PowerShell, qui utilise `$env:HOME`.
