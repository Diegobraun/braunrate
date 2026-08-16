# Instalacao

Tres caminhos. O primeiro nao exige Go instalado, e e o unico que interessa para
quem so quer rodar teste.

## 1. Baixar o binario da release

Em [releases](https://github.com/Diegobraun/braunrate/releases), pegue o arquivo
do seu sistema, confira o checksum e descompacte:

```bash
# Linux/macOS. Troque VERSAO, SISTEMA (linux, darwin) e ARQUITETURA (amd64, arm64).
curl -fsSLO https://github.com/Diegobraun/braunrate/releases/download/vVERSAO/braunrate_VERSAO_SISTEMA_ARQUITETURA.tar.gz
curl -fsSLO https://github.com/Diegobraun/braunrate/releases/download/vVERSAO/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
tar xzf braunrate_VERSAO_SISTEMA_ARQUITETURA.tar.gz
./braunrate version
```

No Windows, baixe o `.zip`, descompacte e rode `braunrate.exe version`.

O arquivo traz o binario, o `README.md`, a `LICENSE` e a pasta `examples/`
inteira — baixar a ferramenta e nao ter cenario para rodar deixa a pessoa onde
ela estava.

**Primeira execucao no macOS:** o sistema bloqueia binario baixado que nao passou
por notarizacao, com a mensagem de que nao foi possivel verificar o
desenvolvedor. Libere uma vez:

```bash
xattr -d com.apple.quarantine ./braunrate
```

**Primeira execucao no Windows:** o SmartScreen avisa que o publicador e
desconhecido. E "Mais informacoes" e entao "Executar assim mesmo". Os dois avisos
tem a mesma causa, que esta em *o que fica de fora*, no fim desta pagina.

## 2. `go install`

Para quem ja tem Go:

```bash
go install github.com/Diegobraun/braunrate/cmd/braunrate@latest
```

O binario sai sem versao injetada — `braunrate version` responde `dev`, e o
documento de resultado guarda `dev`. E honesto: nao e o artefato da release.

## 3. Compilar do fonte

Para quem vai mexer no codigo:

```bash
git clone https://github.com/Diegobraun/braunrate
cd braunrate
go build -o braunrate ./cmd/braunrate
```

## Plataformas

A release nasce em rascunho, que nao aparece na pagina de releases e nao tem URL
de download. So depois que o binario e baixado desse rascunho, conferido pelo
checksum e executado e que ela vira publica. Conferencia reprovada descarta o
rascunho, e nao ha janela em que alguem baixe artefato quebrado.

Seis alvos publicados. Tres deles sao baixados e executados antes da promocao; os
outros tres saem compilados e empacotados pela mesma configuracao, sem ninguem
rodar:

| Plataforma | Publicada | Executada na publicacao |
|---|---|---|
| linux/amd64 | sim | sim |
| linux/arm64 | sim | **nao** |
| darwin/arm64 (Apple Silicon) | sim | sim |
| darwin/amd64 (Mac Intel) | sim | **nao** |
| windows/amd64 | sim | sim |
| windows/arm64 | sim | **nao** |

A conferencia nao e "o binario existe": ela roda `version` e exige que bata com a
tag, valida um cenario publicado e executa outro, exigindo que a versao apareca
dentro do documento de resultado.

## O que fica de fora, e por que

- **Assinatura de codigo e notarizacao** (macOS e Windows). Exigem certificado
  pago e conta de desenvolvedor em nome de alguem. Enquanto nao existirem, os
  dois avisos de primeira execucao acima continuam aparecendo, e a instrucao para
  contorna-los fica escrita em vez de escondida.
- **Homebrew, Scoop e winget.** Cada um e um repositorio a manter e uma cadencia
  de release a cumprir. Um tap que fica para tras e pior do que nao existir,
  porque instala versao velha em silencio.
- **Instalador grafico, `.msi`, `.pkg`, `.deb`.** O binario nao instala nada, nao
  escreve em registro e nao precisa de servico: um instalador so acrescentaria
  passo entre baixar e rodar.
- **Auto-atualizacao.** Ferramenta de medicao que se atualiza sozinha troca a
  versao entre duas execucoes que alguem ia comparar. A comparacao entre
  execucoes de versoes diferentes ja sai sem veredito; atualizar em silencio
  faria esse "sem veredito" aparecer sem motivo visivel.
