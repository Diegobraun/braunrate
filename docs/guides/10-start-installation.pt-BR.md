---
translated_from: 10-start-installation.en.md
source_hash: 97479367547d
---
# Instalação

Três caminhos. O primeiro não exige Go instalado, e é o único que interessa a
quem só quer rodar teste.

## 1. Baixar o binário da release

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

O arquivo traz o binário, o `README.md`, a `LICENSE` e a pasta `examples/`
inteira: baixar a ferramenta e não ter cenário para rodar deixa a pessoa onde ela
estava.

### Primeira execução no macOS

O sistema bloqueia binário baixado que não passou por notarização, com a mensagem
de que não foi possível verificar o desenvolvedor. Libere uma vez:

```bash
xattr -d com.apple.quarantine ./braunrate
```

### Primeira execução no Windows

O SmartScreen avisa que o publicador é desconhecido. Clique em **Mais
informações** e depois em **Executar assim mesmo**.

> **Nota** Os dois avisos têm a mesma causa: não há assinatura de código. O
> motivo está em [O que fica de fora](#o-que-fica-de-fora-e-por-que).

## 2. `go install`

Para quem já tem Go:

```bash
go install github.com/Diegobraun/braunrate/cmd/braunrate@latest
```

O binário sai sem versão injetada: `braunrate version` responde `dev`, e o
documento de resultado guarda `dev`. É honesto, porque não é o artefato da
release.

## 3. Compilar do fonte

Para quem vai mexer no código:

```bash
git clone https://github.com/Diegobraun/braunrate
cd braunrate
go build -o braunrate ./cmd/braunrate
```

## Plataformas

A release nasce em rascunho, que não aparece na página de releases e não tem URL
de download. Ela só vira pública depois que o binário é baixado desse rascunho,
conferido pelo checksum e executado. Conferência reprovada descarta o rascunho, e
não existe janela em que alguém baixe artefato quebrado.

São seis alvos publicados. Três deles são baixados e executados antes da
promoção; os outros três saem compilados e empacotados pela mesma configuração,
sem ninguém rodar:

| Plataforma | Publicada | Executada na publicação |
|---|---|---|
| linux/amd64 | sim | sim |
| linux/arm64 | sim | **não** |
| darwin/arm64 (Apple Silicon) | sim | sim |
| darwin/amd64 (Mac Intel) | sim | **não** |
| windows/amd64 | sim | sim |
| windows/arm64 | sim | **não** |

A conferência não é "o binário existe": ela roda `version` e exige que bata com a
tag, valida um cenário publicado e executa outro, exigindo que a versão apareça
dentro do documento de resultado.

## O que fica de fora, e por quê

- **Assinatura de código e notarização** (macOS e Windows). Exigem certificado
  pago e conta de desenvolvedor em nome de alguém. Enquanto não existirem, os
  dois avisos de primeira execução continuam aparecendo, e a instrução para
  contorná-los fica escrita em vez de escondida.
- **Homebrew, Scoop e winget.** Cada um é um repositório a manter e uma cadência
  de release a cumprir. Um tap que fica para trás é pior do que não existir,
  porque instala versão velha em silêncio.
- **Instalador gráfico, `.msi`, `.pkg`, `.deb`.** O binário não instala nada, não
  escreve em registro e não precisa de serviço: um instalador só acrescentaria um
  passo entre baixar e rodar.
- **Autoatualização.** Ferramenta de medição que se atualiza sozinha troca a
  versão entre duas execuções que alguém ia comparar. A comparação entre
  execuções de versões diferentes já sai sem veredito; atualizar em silêncio
  faria esse "sem veredito" aparecer sem motivo visível.
