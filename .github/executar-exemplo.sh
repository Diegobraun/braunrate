#!/usr/bin/env bash
# Roda um cenario de exemplo e aceita duas saidas: 0, ou 3 quando o unico motivo
# da invalidacao for o gerador nao ter sustentado a carga.
#
# O runner do CI e maquina compartilhada. A 200/s, a proporcao de despachos
# atrasados fica em torno de 1%, que e exatamente onde a regra de saturacao
# corta — entao o mesmo cenario alterna entre 0 e 3 sem nada ter mudado no
# produto. Quando isso acontece, exit 3 e a resposta certa da ferramenta: o
# numero mediria o gerador, nao o alvo.
#
# Mascarar seria aceitar qualquer 3. Por isso qualquer outro motivo de
# invalidacao — jornada incompleta, passo sem amostra, tudo falhou, variedade
# colapsada — quebra o build.
set -uo pipefail

cenario=$1
resultado=$(mktemp)

./braunrate execute "$cenario" -quiet -result="$resultado"
codigo=$?

case $codigo in
  0)
    echo "ok: $cenario"
    ;;
  3)
    motivos=$(jq -r '[.sanidade.achados[]?.tipo] | unique | join(" ")' "$resultado")
    if [ "$motivos" = "gerador_saturado" ]; then
      echo "aceito: $cenario saiu 3 porque o gerador nao sustentou a carga nesta maquina"
      jq -r '.avisos[]? | select(.gravidade=="alta") | "  " + .evidencia' "$resultado"
    else
      echo "FALHOU: $cenario saiu invalido por outro motivo: $motivos"
      jq -r '.sanidade.achados[]? | "  - " + .mensagem + "\n    " + .evidencia' "$resultado"
      exit 1
    fi
    ;;
  *)
    echo "FALHOU: $cenario saiu com codigo $codigo"
    exit "$codigo"
    ;;
esac
