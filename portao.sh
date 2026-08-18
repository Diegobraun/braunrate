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
    echo "FAILED: $*"
    falhou=1
  fi
}

verificar_formatacao() {
  local arquivos
  arquivos=$(gofmt -l $(go list -f '{{.Dir}}' ./...))
  if [ -n "$arquivos" ]; then
    echo "files not gofmt-clean:"
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
# Compara com o arquivo da arvore, e nao com o do HEAD: contra o HEAD, regenerar
# o exemplo e rodar o portao antes de commitar reprovava sempre.
verificar_exemplo_publicado() {
  local recem
  recem=$(mktemp)
  ./braunrate report docs/exemplo-resultado.json -html="$recem" >/dev/null || return 1
  if ! diff -q "$recem" docs/exemplo-relatorio.html >/dev/null; then
    echo "docs/exemplo-relatorio.html is out of date; regenerate it with:"
    echo "  go run ./cmd/braunrate report docs/exemplo-resultado.json -html=docs/exemplo-relatorio.html"
    rm -f "$recem"
    return 1
  fi
  rm -f "$recem"
}

# Quem ja tem broker no ambiente manda nele: no CI os servicos vem do workflow, e
# subir outro por cima daria conflito de porta.
subir_brokers() {
  if [ -n "${BRAUNRATE_KAFKA:-}" ] && [ -n "${BRAUNRATE_AMQP:-}" ]; then
    echo "brokers came from the environment; not starting any"
    return 0
  fi
  command -v docker >/dev/null || { echo "no docker: cannot start a broker"; return 1; }

  docker rm -f portao-kafka portao-rabbit portao-mqtt >/dev/null 2>&1
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
  docker run -d --name portao-mqtt -p 1883:1883 eclipse-mosquitto:2 \
    sh -c 'printf "listener 1883 0.0.0.0\nallow_anonymous true\n" > /mosquitto/config/mosquitto.conf && exec mosquitto -c /mosquitto/config/mosquitto.conf' >/dev/null || return 1

  export BRAUNRATE_KAFKA=127.0.0.1:9092
  export BRAUNRATE_AMQP=amqp://guest:guest@127.0.0.1:5672/
  export BRAUNRATE_MQTT=tcp://127.0.0.1:1883
  trap 'docker rm -f portao-kafka portao-rabbit portao-mqtt >/dev/null 2>&1' EXIT
  echo "waiting for the brokers to answer"
  sleep 20
}

subir_broker_autenticado() {
  if [ -n "${BRAUNRATE_KAFKA_TLS:-}" ]; then
    echo "the authenticated broker came from the environment; not starting any"
    return 0
  fi
  command -v docker >/dev/null || { echo "no docker: cannot start a broker"; return 1; }
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

passo "formatting" verificar_formatacao
passo "vet" go vet ./...
passo "lint" verificar_lint
passo "build" go build ./...

# -race e o portao, e nao um extra: as duas ultimas corridas de verdade do
# projeto — a do servidor e a da semente — so aparecem com ele ligado.
passo "tests with the race detector" go test ./... -count=1 -race
passo "measurement self-check" go test ./internal/selfcheck/... -count=1
passo "the documentation site builds" go run ./cmd/site -out site

if [ $rapido -eq 1 ]; then
  echo
  echo "--rapido: the broker and published-example steps did not run. This is not the gate."
  exit $falhou
fi

passo "real brokers come up" subir_brokers
# BRAUNRATE_EXIGE_BROKER nomeia o que tem de existir. Sem ele, broker que nao
# sobe vira teste pulado, e teste pulado vira portao verde sem nada medido.
BRAUNRATE_EXIGE_BROKER=BRAUNRATE_KAFKA,BRAUNRATE_AMQP \
  passo "messaging tests against real brokers" \
  go test ./internal/messaging/... ./internal/testsupport/... -count=1 -timeout 5m

passo "broker that demands a real credential" subir_broker_autenticado
BRAUNRATE_EXIGE_BROKER=BRAUNRATE_KAFKA_TLS \
  passo "tests against the authenticated broker" \
  go test ./internal/messaging/... -count=1 -timeout 5m

passo "binary for the examples" go build -o braunrate ./cmd/braunrate
passo "the published example is up to date" verificar_exemplo_publicado
passo "every published example runs" rodar_exemplos
rm -f braunrate

echo
if [ $falhou -eq 0 ]; then
  echo "gate green"
else
  echo "gate red"
fi
exit $falhou
