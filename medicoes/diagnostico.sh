#!/bin/bash
# Diagnostico do colapso em taxa alta: quem satura primeiro, gerador ou alvo.
set -u
RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
PORTA=8472
TAXA="${1:-40000}"

"$RAIZ/prototipos/alvo/alvo" -porta=$PORTA -latencia=5ms >/dev/null 2>&1 &
PID_ALVO=$!
sleep 1

(
  while kill -0 $PID_ALVO 2>/dev/null; do
    ps -o pcpu=,rss= -p $PID_ALVO | awk '{printf "alvo cpu=%s%% rss=%dMB\n", $1, $2/1024}'
    sleep 2
  done
) &
PID_AMOSTRA=$!

"$RAIZ/prototipos/go/braunrate-proto-go" \
  -alvo=http://127.0.0.1:$PORTA/pedido -taxa=$TAXA -duracao=15s -aquecimento=5s -espera-antes=1s \
  2>&1 | grep -E "heartbeat|PRONTO" &
PID_GERADOR=$!

wait $PID_GERADOR
echo "atendidas pelo alvo: $(curl -s http://127.0.0.1:$PORTA/atendidas)"
kill $PID_AMOSTRA 2>/dev/null
kill $PID_ALVO 2>/dev/null
wait 2>/dev/null
