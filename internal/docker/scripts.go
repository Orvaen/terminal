package docker

// engineScript tourne dans la machine Alpine : il lance dockerd puis l'arrête après
// ORVAEN_DOCKER_IDLE secondes sans commande docker ni conteneur actif (600 par défaut, 0 = jamais).
// Quand dockerd s'arrête, plus rien ne tourne et WSL éteint la machine virtuelle : zéro RAM utilisée.
//
// Si les certificats sont présents, dockerd écoute aussi sur 127.0.0.1:EnginePort (TLS avec certificat
// client), pour les builds lancés par le client docker Windows.
const engineScript = `#!/bin/sh
# ` + engineMarker + `
IDLE_LIMIT="${ORVAEN_DOCKER_IDLE:-600}"
ACTIVITY=/run/orvaen-activity
mkdir -p /run /usr/local/sbin

# Les distributions importées dans WSL n'ont pas de systemd : on monte les cgroups nous-mêmes.
if ! mountpoint -q /sys/fs/cgroup; then
  mkdir -p /sys/fs/cgroup
  mount -t cgroup2 cgroup2 /sys/fs/cgroup
fi

# Certains noyaux WSL ne gèrent pas iptables-nft : on bascule sur iptables-legacy si besoin.
if ! iptables -L -n >/dev/null 2>&1; then
  for t in iptables ip6tables; do
    if command -v "$t-legacy" >/dev/null 2>&1; then ln -sf "$(command -v "$t-legacy")" "/usr/local/sbin/$t"; fi
  done
fi

TLS=/etc/docker/tls
set -- -H unix:///var/run/docker.sock
if [ -f "$TLS/server-cert.pem" ] && [ -f "$TLS/server-key.pem" ] && [ -f "$TLS/ca.pem" ]; then
  set -- "$@" -H tcp://127.0.0.1:` + EnginePort + ` --tlsverify --tlscacert "$TLS/ca.pem" --tlscert "$TLS/server-cert.pem" --tlskey "$TLS/server-key.pem"
fi

rm -f /var/run/docker.pid
dockerd "$@" >/var/log/docker.log 2>&1 &
pid=$!
touch "$ACTIVITY"

while kill -0 "$pid" 2>/dev/null; do
  sleep 30
  if [ -n "$(docker ps -q 2>/dev/null)" ]; then touch "$ACTIVITY"; fi
  # Un build lancé depuis Windows est une connexion ouverte sur le port TCP : c'est de l'activité.
  if netstat -tn 2>/dev/null | grep -q ':` + EnginePort + ` .*ESTABLISHED'; then touch "$ACTIVITY"; fi
  last=$(stat -c %Y "$ACTIVITY" 2>/dev/null || echo 0)
  if [ "$IDLE_LIMIT" -gt 0 ] && [ $(( $(date +%s) - last )) -ge "$IDLE_LIMIT" ]; then
    kill "$pid"
    wait "$pid"
    exit 0
  fi
done
`

// engineMarker identifie la version du script du moteur : les machines plus anciennes le reçoivent au démarrage.
const engineMarker = "orvaen-engine-v2"

// writeEngineScript installe le script du moteur dans la machine.
const writeEngineScript = `cat > /usr/local/bin/orvaen-dockerd <<'ORVAEN_EOF'
` + engineScript + `ORVAEN_EOF
chmod +x /usr/local/bin/orvaen-dockerd
`

// wslConfMarker identifie la version de /etc/wsl.conf écrite par le terminal.
const wslConfMarker = "orvaen-wsl-conf-v1"

// wslConf configure le montage des disques Windows (/mnt/c…) dans la machine Docker.
//
// L'option metadata permet à Linux d'enregistrer propriétaire et droits sur les fichiers Windows :
// sans elle, tout appartient à root et un conteneur non-root qui fait chmod sur un fichier qu'il vient
// de créer (npm ci, pip…) échoue avec « EPERM: operation not permitted ». umask/fmask à 000 laisse les
// fichiers créés depuis Windows accessibles en écriture à tous les utilisateurs des conteneurs,
// comme avec Docker Desktop.
const wslConf = `# ` + wslConfMarker + ` : écrit par Orvaen Terminal, ne pas modifier
[automount]
enabled = true
options = "metadata,umask=000,fmask=000"
`

// writeWSLConfScript écrit /etc/wsl.conf (pris en compte au prochain démarrage de la machine).
const writeWSLConfScript = `cat > /etc/wsl.conf <<'ORVAEN_EOF'
` + wslConf + `ORVAEN_EOF
`

// wslConfCheck réussit si /etc/wsl.conf est à jour.
const wslConfCheck = `grep -qs '` + wslConfMarker + `' /etc/wsl.conf`

// setupScript installe Docker Engine (sans interface) dans la machine Alpine.
const setupScript = `set -e
if ! grep -q '/community' /etc/apk/repositories; then
  sed -n 's#/main$#/community#p' /etc/apk/repositories >> /etc/apk/repositories
fi
apk add --no-cache docker docker-cli-compose docker-cli-buildx
` + writeEngineScript + writeWSLConfScript

// buildxPlugin : le plugin BuildKit de la CLI docker. Sans lui, « docker build » retombe sur l'ancien
// constructeur (obsolète) : contexte envoyé en entier à chaque build, étapes en série, cache moins efficace.
const buildxPlugin = "/usr/libexec/docker/cli-plugins/docker-buildx"

// prepareScript tourne juste avant le démarrage du moteur, en un seul lancement de wsl.exe :
// il ajoute BuildKit aux machines installées sans lui, met à jour le script du moteur et /etc/wsl.conf,
// et copie les certificats du port TCP (dossier Windows ORVAEN_DOCKER_CERTS, traduit en /mnt/c/… par WSLENV).
// Code prepareRestart : wsl.conf vient d'être réécrit, la machine doit redémarrer pour l'appliquer.
const prepareScript = `restart=0
if ! ` + wslConfCheck + `; then
  echo "Mise à jour de la machine Docker : droits des fichiers Windows dans les conteneurs…" >&2
  ` + writeWSLConfScript + `  restart=1
fi
if [ ! -x ` + buildxPlugin + ` ]; then
  echo "Ajout de BuildKit (docker buildx) pour des builds plus rapides, une seule fois…" >&2
  apk add --no-cache -q docker-cli-buildx >/dev/null 2>&1 || echo "avertissement : BuildKit n'a pas pu être installé (réseau ?)." >&2
fi
if ! grep -qs '` + engineMarker + `' /usr/local/bin/orvaen-dockerd; then
  ` + writeEngineScript + `fi
if [ -n "$ORVAEN_DOCKER_CERTS" ] && [ -f "$ORVAEN_DOCKER_CERTS/server-cert.pem" ]; then
  mkdir -p /etc/docker/tls
  for f in ca.pem server-cert.pem server-key.pem; do
    cmp -s "$ORVAEN_DOCKER_CERTS/$f" "/etc/docker/tls/$f" || cp "$ORVAEN_DOCKER_CERTS/$f" "/etc/docker/tls/$f"
  done
  chmod 600 /etc/docker/tls/server-key.pem
fi
[ "$restart" = 1 ] && exit 10
exit 0
`

// prepareRestart : code rendu par prepareScript quand la machine doit redémarrer.
const prepareRestart = 10

// waitReadyScript attend dans la machine que dockerd réponde (un seul wsl.exe au lieu d'un par essai).
// Échoue après 60 s, ou après 10 s si dockerd n'a même pas démarré.
const waitReadyScript = `i=0
while [ $i -lt 300 ]; do
  if test -S /var/run/docker.sock && docker version >/dev/null 2>&1; then exit 0; fi
  if [ $i -ge 50 ] && ! pgrep -x dockerd >/dev/null; then exit 1; fi
  sleep 0.2
  i=$((i+1))
done
exit 1
`

// runningCheck : dockerd tourne et son socket est prêt.
const runningCheck = `pgrep -x dockerd >/dev/null && test -S /var/run/docker.sock`

// cliWrapper vérifie que le moteur tourne (sinon code 222, sans rien lancer), marque l'activité
// (pour l'arrêt automatique) puis lance le vrai client docker.
const cliWrapper = runningCheck + ` || exit 222; touch /run/orvaen-activity 2>/dev/null; exec docker "$@"`
