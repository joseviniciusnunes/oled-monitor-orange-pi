#!/usr/bin/env bash
#
# Copia o código deste repositório para o Orange Pi via scp.
#
# Uso:
#   ./deploy.sh            # copia para o destino padrão
#   ./deploy.sh <destino>  # copia para um diretório específico na placa
#
# Configuração (ajuste se necessário):
set -euo pipefail

HOST="homelab@192.168.1.99"
REMOTE_DIR="${1:-/home/homelab/oled}"

# Diretório raiz do repositório (onde este script está)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Copiando código para ${HOST}:${REMOTE_DIR}"

# Cria o diretório remoto, se não existir
ssh "${HOST}" "mkdir -p '${REMOTE_DIR}'"

# Copia todos os arquivos do projeto, exceto .git
rsync -av --delete \
  --exclude '.git' \
  --exclude 'deploy.sh' \
  "${SCRIPT_DIR}/" \
  "${HOST}:${REMOTE_DIR}/"

echo "==> Código copiado com sucesso para ${HOST}:${REMOTE_DIR}"
echo "==> Para construir e rodar na placa:"
echo "    ssh ${HOST}"
echo "    cd ${REMOTE_DIR}"
echo "    docker compose up -d --build"