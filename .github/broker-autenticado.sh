#!/usr/bin/env bash
# Sobe um Kafka que exige credencial de verdade: SASL/SCRAM-SHA-512 sobre TLS,
# com CA propria. Teste que so roda contra broker local sem senha nao decide
# nada sobre um broker de homologacao.
#
# O listener PLAINTEXT em 9097 existe so para criar o usuario SCRAM: em KRaft a
# credencial e criada pelo kafka-configs, e ele precisa de um caminho de
# administracao. Ele escuta so em 127.0.0.1 e nao e o listener que o teste usa.
set -euo pipefail

diretorio=${1:-$(mktemp -d)}
nome=${BROKER_NOME:-braunrate-kafka-auth}
senha_keystore=${SENHA_KEYSTORE:-braunrate-ci}
usuario=${BRAUNRATE_KAFKA_USER:-ana}
senha=${BRAUNRATE_KAFKA_PASSWORD:-segredo-do-ci}

mkdir -p "$diretorio"
cd "$diretorio"

openssl req -new -x509 -days 3650 -nodes -newkey rsa:2048 \
  -keyout ca.key -out ca.pem -subj "/CN=braunrate-ca" 2>/dev/null

openssl req -new -nodes -newkey rsa:2048 -keyout broker.key -out broker.csr \
  -subj "/CN=localhost" 2>/dev/null
printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\n' > extensoes.cnf
openssl x509 -req -in broker.csr -CA ca.pem -CAkey ca.key -CAcreateserial \
  -out broker.pem -days 3650 -extfile extensoes.cnf 2>/dev/null

openssl pkcs12 -export -in broker.pem -inkey broker.key -certfile ca.pem \
  -name broker -out broker.p12 -password "pass:$senha_keystore" 2>/dev/null

# A imagem espera as senhas em arquivo dentro de /etc/kafka/secrets, e exige
# KAFKA_OPTS com java.security.auth.login.config mesmo quando o SCRAM nao usa
# JAAS: em KRaft a credencial vem do log de metadados.
printf '%s' "$senha_keystore" > keystore_creds
printf '%s' "$senha_keystore" > key_creds
cat > kafka_jaas.conf <<'JAAS'
KafkaServer {
  org.apache.kafka.common.security.scram.ScramLoginModule required;
};
JAAS
chmod 644 broker.p12 ca.pem keystore_creds key_creds kafka_jaas.conf

docker rm -f "$nome" >/dev/null 2>&1 || true
docker run -d --name "$nome" \
  -p 9095:9095 -p 9097:9097 \
  -v "$diretorio":/etc/kafka/secrets \
  -e KAFKA_NODE_ID=1 \
  -e KAFKA_PROCESS_ROLES=broker,controller \
  -e KAFKA_LISTENERS='PLAINTEXT://:9097,CONTROLLER://:9096,SASL_SSL://:9095' \
  -e KAFKA_ADVERTISED_LISTENERS='PLAINTEXT://127.0.0.1:9097,SASL_SSL://localhost:9095' \
  -e KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER \
  -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP='CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT,SASL_SSL:SASL_SSL' \
  -e KAFKA_CONTROLLER_QUORUM_VOTERS='1@localhost:9096' \
  -e KAFKA_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
  -e KAFKA_SASL_ENABLED_MECHANISMS=SCRAM-SHA-512 \
  -e KAFKA_SSL_KEYSTORE_TYPE=PKCS12 \
  -e KAFKA_SSL_KEYSTORE_FILENAME=broker.p12 \
  -e KAFKA_SSL_KEYSTORE_CREDENTIALS=keystore_creds \
  -e KAFKA_SSL_KEY_CREDENTIALS=key_creds \
  -e KAFKA_SSL_CLIENT_AUTH=none \
  -e KAFKA_OPTS='-Djava.security.auth.login.config=/etc/kafka/secrets/kafka_jaas.conf' \
  -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
  -e KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR=1 \
  -e KAFKA_TRANSACTION_STATE_LOG_MIN_ISR=1 \
  -e KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0 \
  -e KAFKA_AUTO_CREATE_TOPICS_ENABLE=true \
  apache/kafka:3.8.0 >/dev/null

pronto=0
for _ in $(seq 1 40); do
  if docker exec "$nome" /opt/kafka/bin/kafka-broker-api-versions.sh \
      --bootstrap-server 127.0.0.1:9097 >/dev/null 2>&1; then
    pronto=1
    break
  fi
  sleep 2
done
if [ "$pronto" -ne 1 ]; then
  echo "o broker autenticado nao subiu em 80s" >&2
  docker logs "$nome" 2>&1 | tail -30 >&2
  exit 1
fi

docker exec "$nome" /opt/kafka/bin/kafka-configs.sh \
  --bootstrap-server 127.0.0.1:9097 --alter \
  --add-config "SCRAM-SHA-512=[password=$senha]" \
  --entity-type users --entity-name "$usuario" >/dev/null

echo "broker autenticado em localhost:9095 (SCRAM-SHA-512 sobre TLS), usuario $usuario"
echo "$diretorio/ca.pem"
