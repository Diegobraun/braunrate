#!/usr/bin/env bash
# O portao do projeto, na maquina e no CI. Existe porque os dois tinham listas
# diferentes: 'go test ./...' local passava enquanto o CI reprovava, e ficou
# vermelho oito commits sem ninguem ver. Duas listas divergem sozinhas; uma nao.
#
# Aqui o CI chama este arquivo. Mudar o portao e mudar este arquivo, e a pessoa
# que roda antes de empurrar roda exatamente o que vai decidir.
#
#   ./portao.sh            sobe os brokers em docker e roda tudo
#   ./portao.sh --rapido   pula o que precisa de broker (nao e o portao)
set -uo pipefail

falhou=0
rapido=0
[ "${1:-}" = "--rapido" ] && rapido=1

passo() {
  echo
  echo "=== $1"
  shift
  if ! "$@"; then
    echo "FALHOU: $*"
    falhou=1
  fi
}

verificar_formatacao() {
  local arquivos
  arquivos=$(gofmt -l $(go list -f '{{.Dir}}' ./...))
  if [ -n "$arquivos" ]; then
    echo "arquivos fora do gofmt:"
    echo "$arquivos"
    return 1
  fi
}

verificar_lint() {
  local binario
  binario="$(go env GOPATH)/bin/golangci-lint"
  if [ ! -x "$binario" ]; then
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 || return 1
  fi
  "$binario" run ./... --timeout 5m
}

# O exemplo publicado e a primeira coisa que alguem copia, e ja esteve
# desatualizado sem ninguem ver.
verificar_exemplo_publicado() {
  ./braunrate report docs/exemplo-resultado.json -html=docs/exemplo-relatorio.html || return 1
  if ! git diff --quiet -- docs/exemplo-relatorio.html; then
    echo "docs/exemplo-relatorio.html esta desatualizado; regenere com:"
    echo "  go run ./cmd/braunrate report docs/exemplo-resultado.json -html=docs/exemplo-relatorio.html"
    git --no-pager diff --stat -- docs/exemplo-relatorio.html
    return 1
  fi
}

# Quem ja tem broker no ambiente manda nele: no CI os servicos vem do workflow, e
# subir outro por cima daria conflito de porta.
subir_brokers() {
  if [ -n "${BRAUNRATE_KAFKA:-}" ] && [ -n "${BRAUNRATE_AMQP:-}" ]; then
    echo "brokers vieram do ambiente; nao subo nada"
    return 0
  fi
  command -v docker >/dev/null || { echo "sem docker: nao ha como subir broker"; return 1; }

  docker rm -f portao-kafka portao-rabbit >/dev/null 2>&1
  docker run -d --name portao-kafka -p 9092:9092 \
    -e KAFKA_NODE_ID=1 -e KAFKA_PROCESS_ROLES=broker,controller \
    -e KAFKA_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093 \
    -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://127.0.0.1:9092 \
    -e KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER \
    -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
    -e KAFKA_CONTROLLER_QUORUM_VOTERS=1@localhost:9093 \
    -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
    -e KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR=1 \
    -e KAFKA_TRANSACTION_STATE_LOG_MIN_ISR=1 \
    -e KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0 \
    -e KAFKA_AUTO_CREATE_TOPICS_ENABLE=true \
    apache/kafka:3.8.0 >/dev/null || return 1
  docker run -d --name portao-rabbit -p 5672:5672 rabbitmq:3.13-alpine >/dev/null || return 1

  export BRAUNRATE_KAFKA=127.0.0.1:9092
  export BRAUNRATE_AMQP=amqp://guest:guest@127.0.0.1:5672/
  trap 'docker rm -f portao-kafka portao-rabbit >/dev/null 2>&1' EXIT
  echo "esperando os brokers responderem"
  sleep 20
}

subir_broker_autenticado() {
  if [ -n "${BRAUNRATE_KAFKA_TLS:-}" ]; then
    echo "broker autenticado veio do ambiente; nao subo nada"
    return 0
  fi
  command -v docker >/dev/null || { echo "sem docker: nao ha como subir broker"; return 1; }
  chmod +x .github/broker-autenticado.sh
  .github/broker-autenticado.sh /tmp/certificados-ci || return 1
  export BRAUNRATE_KAFKA_TLS=localhost:9095
  export BRAUNRATE_KAFKA_USER=ana
  export BRAUNRATE_KAFKA_PASSWORD=segredo-do-ci
  export BRAUNRATE_KAFKA_CA=/tmp/certificados-ci/ca.pem
}

rodar_exemplos() {
  ./braunrate target -address=127.0.0.1:8080 -latency=5ms \
    -kafka="$BRAUNRATE_KAFKA" -input=orders-chain -output=orders-processed &
  local alvo=$!
  sleep 3
  chmod +x .github/executar-exemplos.sh
  .github/executar-exemplos.sh
  local codigo=$?
  kill $alvo 2>/dev/null
  return $codigo
}

passo "formatacao" verificar_formatacao
passo "vet" go vet ./...
passo "lint" verificar_lint
passo "build" go build ./...

# -race e o portao, e nao um extra: as duas ultimas corridas de verdade do
# projeto — a do servidor e a da semente — so aparecem com ele ligado.
passo "testes com detector de corrida" go test ./... -count=1 -race
passo "auto-validacao da medicao" go test ./internal/selfcheck/... -count=1
passo "o site de documentacao gera" go run ./cmd/site -out site

if [ $rapido -eq 1 ]; then
  echo
  echo "--rapido: os passos de broker e de exemplo publicado nao rodaram. Isto nao e o portao."
  exit $falhou
fi

passo "brokers de verdade sobem" subir_brokers
# BRAUNRATE_EXIGE_BROKER nomeia o que tem de existir. Sem ele, broker que nao
# sobe vira teste pulado, e teste pulado vira portao verde sem nada medido.
BRAUNRATE_EXIGE_BROKER=BRAUNRATE_KAFKA,BRAUNRATE_AMQP \
  passo "testes de mensageria contra brokers reais" \
  go test ./internal/messaging/... ./internal/testsupport/... -count=1 -timeout 5m

passo "broker que exige credencial de verdade" subir_broker_autenticado
BRAUNRATE_EXIGE_BROKER=BRAUNRATE_KAFKA_TLS \
  passo "testes contra o broker autenticado" \
  go test ./internal/messaging/... -count=1 -timeout 5m

passo "binario para os exemplos" go build -o braunrate ./cmd/braunrate
passo "exemplo publicado esta atualizado" verificar_exemplo_publicado
passo "todos os exemplos publicados rodam" rodar_exemplos
rm -f braunrate

echo
if [ $falhou -eq 0 ]; then
  echo "portao verde"
else
  echo "portao vermelho"
fi
exit $falhou
