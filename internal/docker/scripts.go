package docker

// engineScript tourne dans la machine Alpine : il lance dockerd puis l'arrête après
// ORVAEN_DOCKER_IDLE secondes sans commande docker ni conteneur actif (600 par défaut, 0 = jamais).
// Quand dockerd s'arrête, plus rien ne tourne et WSL éteint la machine virtuelle : zéro RAM utilisée.
const engineScript = `#!/bin/sh
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

rm -f /var/run/docker.pid
dockerd >/var/log/docker.log 2>&1 &
pid=$!
touch "$ACTIVITY"

while kill -0 "$pid" 2>/dev/null; do
  sleep 30
  if [ -n "$(docker ps -q 2>/dev/null)" ]; then touch "$ACTIVITY"; fi
  last=$(stat -c %Y "$ACTIVITY" 2>/dev/null || echo 0)
  if [ "$IDLE_LIMIT" -gt 0 ] && [ $(( $(date +%s) - last )) -ge "$IDLE_LIMIT" ]; then
    kill "$pid"
    wait "$pid"
    exit 0
  fi
done
`

// setupScript installe Docker Engine (sans interface) dans la machine Alpine.
const setupScript = `set -e
if ! grep -q '/community' /etc/apk/repositories; then
  sed -n 's#/main$#/community#p' /etc/apk/repositories >> /etc/apk/repositories
fi
apk add --no-cache docker docker-cli-compose
cat > /usr/local/bin/orvaen-dockerd <<'ORVAEN_EOF'
` + engineScript + `ORVAEN_EOF
chmod +x /usr/local/bin/orvaen-dockerd
`

// runningCheck : dockerd tourne et son socket est prêt.
const runningCheck = `pgrep -x dockerd >/dev/null && test -S /var/run/docker.sock`

// cliWrapper marque l'activité (pour l'arrêt automatique) puis lance le vrai client docker.
const cliWrapper = `touch /run/orvaen-activity 2>/dev/null; exec docker "$@"`
