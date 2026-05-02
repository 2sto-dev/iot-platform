#!/usr/bin/env bash
# Instalare EMQX 5.8.6 pe emqx-1 (Debian 12/13).
# Rulează de pe mașina locală:
#   SSH_KEY=~/.ssh/id_ed25519_new bash infra/emqx-install.sh

set -euo pipefail

SSH_KEY="${SSH_KEY:-~/.ssh/id_ed25519_new}"
EMQX_HOST="c@172.16.0.103"
EMQX_VERSION="5.8.6"
DEB_URL="https://packages.emqx.com/emqx-ce/v${EMQX_VERSION}/emqx-${EMQX_VERSION}-debian12-amd64.deb"

echo "=== EMQX $EMQX_VERSION install on $EMQX_HOST ==="

ssh -i "$SSH_KEY" "$EMQX_HOST" "
  set -e
  cd /tmp
  echo '[..] Download EMQX deb...'
  wget -q -O emqx.deb '$DEB_URL'
  ls -lh emqx.deb
  echo '[..] Install dependencies...'
  sudo apt-get install -y -q libssl3 libodbc2 2>/dev/null || true
  echo '[..] Install EMQX...'
  sudo dpkg -i emqx.deb
  echo '[OK] Installed:'
  sudo emqx --version 2>/dev/null | head -1 || echo 'run: sudo emqx --version'
  sudo systemctl enable emqx
  echo '[OK] emqx service enabled'
"

echo ""
echo "Pasul următor: configurează cu HOOK_SECRET=... bash infra/emqx-http-acl.sh"
