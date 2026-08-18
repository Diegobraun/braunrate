#!/usr/bin/env bash
# Roda TODOS os exemplos de examples/ contra o alvo embutido. Exemplo novo entra
# no laco sozinho; exemplo que nao roda quebra o build.
#
# Exemplo publicado e a primeira coisa que alguem copia. Ja aconteceu tres vezes
# de um deles estar quebrado sem ninguem ver: ci.yaml com 100% de 401 desde a
# Fase 1, http-basico sem autenticacao e batendo numa rota que o alvo nao tinha,
# e cadeia-assincrona com um SLO que o alvo documentado nunca conseguia atingir.
# As tres apareceram por acidente.
#
# Quem depende de infraestrutura declara com 'requires:' no proprio arquivo. Sem a
# infraestrutura, o laco pula com aviso visivel — nunca em silencio.
set -uo pipefail

falhou=0
pulados=()

disponivel() {
  case "$1" in
    kafka) [ -n "${BRAUNRATE_KAFKA:-}" ] ;;
    amqp) [ -n "${BRAUNRATE_AMQP:-}" ] ;;
    mqtt) [ -n "${BRAUNRATE_MQTT:-}" ] ;;
    credential) [ -n "${BRAUNRATE_CREDENCIAL:-}" ] ;;
    *) return 1 ;;
  esac
}

for cenario in examples/*.yaml; do
  faltando=""
  for requisito in $(grep -oE '^requires: *\[[^]]*\]' "$cenario" | tr -d '[]' | sed 's/^requires: *//' | tr ',' ' '); do
    disponivel "$requisito" || faltando="$faltando $requisito"
  done
  if [ -n "$faltando" ]; then
    echo "PULADO: $cenario declara 'requires:$faltando' e esta maquina nao tem"
    pulados+=("$cenario")
    continue
  fi

  resultado=$(mktemp)
  ./braunrate execute "$cenario" -quiet -result="$resultado" > /dev/null
  codigo=$?

  case $codigo in
    0)
      echo "ok: $cenario"
      ;;
    3)
      motivos=$(jq -r '[.sanity.findings[]?.kind] | unique | join(" ")' "$resultado")
      if [ "$motivos" = "generatorSaturated" ]; then
        # O runner e maquina compartilhada e a regra de saturacao corta em 1% de
        # despachos atrasados. Ali exit 3 e a resposta certa da ferramenta.
        echo "aceito: $cenario saiu 3 porque o gerador nao sustentou a carga nesta maquina"
        jq -r '.warnings[]? | select(.severity=="high") | "  " + .evidence' "$resultado"
      else
        echo "FALHOU: $cenario saiu invalido por outro motivo: $motivos"
        jq -r '.sanity.findings[]? | "  - " + .message + "\n    " + .evidence' "$resultado"
        falhou=1
      fi
      ;;
    *)
      echo "FALHOU: $cenario saiu com codigo $codigo"
      ./braunrate execute "$cenario" -quiet 2>&1 | tail -25
      falhou=1
      ;;
  esac
done

if [ ${#pulados[@]} -gt 0 ]; then
  echo
  echo "${#pulados[@]} exemplo(s) pulado(s) por falta de infraestrutura declarada: ${pulados[*]}"
fi

exit $falhou
