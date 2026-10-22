# Docker léger

[← Retour à l'accueil](../README.md)

`apt install docker` installe le **vrai Docker Engine**, sans Docker Desktop : pas d'interface, pas de service
Windows, pas d'icône dans la barre des tâches, et rien qui tourne quand tu ne t'en sers pas.

```text
$ apt install docker
$ docker run --rm hello-world
$ docker run -d -p 8080:80 -v C:\Users\dev\site:/usr/share/nginx/html nginx:alpine
$ docker compose up -d
```

## Comment ça marche

Les conteneurs ont besoin d'un noyau Linux, que Windows n'a pas : il faut donc une petite machine virtuelle Linux.
Docker Desktop en utilise déjà une, dans WSL2, mais il ajoute tout le reste par-dessus. Le Docker léger garde
seulement l'essentiel :

1. **WSL2**, le Linux intégré à Windows, activé si besoin ;
2. une mini machine **Alpine Linux** de quelques Mo, nommée `orvaen-docker`, séparée de tes autres distributions ;
3. **Docker Engine**, **Docker Compose** et **BuildKit** (`docker buildx`), installés depuis les dépôts officiels d'Alpine.

La commande `docker` (intégrée au terminal, et `docker.exe` pour PowerShell, cmd ou VS Code) transmet tes commandes
à ce moteur.

## Installation

```text
$ apt install docker
```

| Étape | Détail |
|---|---|
| WSL2 | S'il n'est pas activé, une fenêtre administrateur s'ouvre pour l'activer. **Redémarre ton PC**, puis relance `apt install docker`. Cette étape n'arrive qu'une fois |
| Alpine Linux | Téléchargée depuis `dl-cdn.alpinelinux.org`, avec vérification de la somme SHA-256 |
| Docker Engine | Installé dans la machine avec Docker Compose |
| Premier démarrage | Le moteur démarre et `docker --version` s'affiche |

## Un moteur qui ne consomme rien au repos

- **Démarrage à la demande** : rien ne tourne tant que tu ne tapes pas `docker`. La première commande démarre
  le moteur en quelques secondes.
- **Arrêt automatique** : après **10 minutes** sans commande docker et sans conteneur actif, le moteur s'arrête et
  WSL libère toute la mémoire.
- **Indépendant du terminal** : tu peux fermer la fenêtre, tes conteneurs continuent de tourner.

Pour changer le délai, définis la variable d'environnement `ORVAEN_DOCKER_IDLE` (en secondes ; `0` désactive
l'arrêt automatique), puis redémarre le moteur avec `docker engine stop`. `apt optimiser` propose de le passer à 1 h.

Quand le moteur tourne, une commande `docker` ne lance qu'un seul `wsl.exe` : le script de la machine vérifie
lui-même que le moteur est prêt. Au démarrage à froid, l'attente du moteur se fait aussi dans la machine, en un seul
`wsl.exe`, au lieu d'un nouveau lancement toutes les 0,7 s.

### Builds rapides : sans passer par le partage de fichiers

Dans la machine Linux, ton dossier Windows est lu à travers `/mnt/c`, le partage de fichiers de WSL. Il est très lent
pour des milliers de petits fichiers, et c'est lui qui rendait les builds longs. Comme Docker Desktop, les builds
sont donc lancés par le **client docker officiel pour Windows** : il lit ton dossier directement sur le disque et
l'envoie au moteur en un seul flux.

| Commande | Passe par |
|---|---|
| `docker build`, `docker buildx build`, `docker image build` | Client Windows |
| `docker compose build` | Client Windows |
| `docker compose up --build` (et `run`, `create`) | Build par le client Windows, puis `up` sans `--build` par WSL |
| Tout le reste (`run`, `ps`, `compose up`…) | WSL, comme avant |

- Le client (`docker.exe`, avec les plugins **BuildKit** et **Compose**) est téléchargé une fois dans
  `%LOCALAPPDATA%\OrvaenTerminal\docker-client`, depuis `download.docker.com` et les releases GitHub de Docker
  (sommes SHA-256 vérifiées pour les plugins). Il est mis à jour par `apt upgrade docker`.
- Il parle au moteur sur `localhost:23760`, en **TLS avec certificat client** : seul ton compte Windows a les clés,
  les autres comptes du PC ne peuvent pas utiliser le moteur.
- Tes identifiants `docker login` sont repris pour les builds (images de base privées).
- Une machine déjà installée reçoit tout ça au prochain démarrage du moteur : `docker engine stop`, puis relance ton build.
- Si quelque chose bloque (port, relais localhost de WSL désactivé…), le build repasse tout seul par WSL.
  Pour forcer l'ancien chemin : variable d'environnement `ORVAEN_DOCKER_NATIVE=0`.

Ajoute aussi un fichier `.dockerignore` pour ne pas envoyer les gros dossiers inutiles : le terminal te prévient s'il
voit un `node_modules`, `.git`, `vendor`… sans `.dockerignore`.

```text
node_modules
.git
dist
```

### Projets lents dans un conteneur

Un dossier Windows monté (`-v C:\projet:/app`, ou `.:/app` dans `compose.yaml`) passe par le partage de fichiers
de WSL, beaucoup plus lent que le disque de la machine pour les petits fichiers. `node_modules` (des dizaines de
milliers de fichiers) en souffre le plus. Garde le code monté, mais mets les dépendances dans un volume :

```yaml
services:
  api:
    volumes:
      - .:/app
      - node_modules:/app/node_modules   # dans la machine Docker : rapide
volumes:
  node_modules:
```

## Au quotidien

| Situation | Ce qui se passe |
|---|---|
| Chemin Windows dans un volume | Converti automatiquement : `-v C:\projet:/app` devient `/mnt/c/projet:/app`, aussi dans `--mount source=…` |
| Droits des fichiers Windows | Les disques Windows sont montés avec l'option `metadata` de WSL : un conteneur non-root peut créer des fichiers dans ton projet et changer leurs droits (`npm ci`, `pip install`…), comme avec Docker Desktop. Une machine déjà installée est mise à jour toute seule au prochain démarrage du moteur |
| Port publié | `-p 8080:80` est accessible depuis Windows sur `localhost:8080` |
| Dossier courant | La commande s'exécute dans ton dossier courant : `docker compose up` trouve ton `compose.yaml` |
| Hors du terminal | `docker` marche aussi dans PowerShell, cmd et VS Code, grâce à `docker.exe` |

## Commandes du moteur

| Commande | Rôle |
|---|---|
| `docker engine status` | État du moteur et place occupée sur le disque |
| `docker engine start` | Démarre le moteur (automatique à la première commande) |
| `docker engine stop` | Arrête le moteur tout de suite et libère la mémoire |
| `docker engine logs` | Affiche les dernières lignes du journal du moteur |
| `apt upgrade docker` | Met à jour Docker Engine et Alpine (aussi fait par `apt upgrade`) |
| `apt remove docker` | Supprime la machine, avec tous ses conteneurs, images et volumes, après confirmation |

## Bon à savoir

- Les conteneurs s'arrêtent avec le moteur. Après un arrêt automatique ou un redémarrage du PC,
  relance-les (`docker compose up -d`).
- Les fichiers lus depuis `C:\` à travers `/mnt/c` sont plus lents que dans Linux. Pour un gros projet
  (des `node_modules` par exemple), préfère un volume Docker nommé.
- Si Docker Desktop est aussi installé, son `docker.exe` passe avant le nôtre dans PowerShell et cmd.
  Dans le terminal, c'est toujours le Docker léger qui répond. Pour ne garder que lui : `apt remove docker-desktop`.

## En cas de problème

| Symptôme | Piste |
|---|---|
| `Docker n'est pas installé` | Lance `apt install docker` |
| L'installation s'arrête après l'activation de WSL2 | C'est normal : redémarre le PC, puis relance `apt install docker` |
| `EPERM: operation not permitted, chmod` dans un conteneur | Machine encore sur l'ancien montage : `docker engine stop`, puis relance ta commande |
| `docker build` très lent | `docker engine stop`, puis relance le build : il passera par le client Windows. Ajoute un `.dockerignore` |
| `le moteur Docker ne répond pas` | Regarde `docker engine logs`, puis essaie `docker engine stop` et relance ta commande |
| Tout recommencer | `apt remove docker -y`, puis `apt install docker` |

Si le problème persiste, ouvre une [issue](https://github.com/Orvaen/terminal/issues) avec la sortie de
`docker engine logs` et de `wsl --version`.
