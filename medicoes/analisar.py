#!/usr/bin/env python3
"""Le os JSON brutos da bateria e imprime as tabelas em markdown."""
import json
import sys
from pathlib import Path

BRUTO = Path(__file__).resolve().parent / "bruto"


def carregar(experimento):
    caminho = BRUTO / f"resumo-{experimento}.json"
    return json.loads(caminho.read_text()) if caminho.exists() else []


def valor(agregado, campo):
    return agregado[campo]["media"], agregado[campo]["margem"]


def com_margem(agregado, campo, casas=1):
    media, margem = valor(agregado, campo)
    return f"{media:,.{casas}f} ± {margem:,.{casas}f}".replace(",", ".")


def sustentou(agregado):
    taxa = agregado["taxa_efetiva"]["media"]
    alvo = agregado["taxa_alvo"]
    atrasados = agregado["despachos_atrasados"]["media"]
    medidas = max(1.0, taxa * 15)
    desvio = agregado["desvio_p99_us"]["media"]
    return taxa >= 0.99 * alvo and atrasados / medidas < 0.01 and desvio < 10_000


def inclinacao(pontos):
    """Custo marginal de CPU por requisicao: regressao linear de cpu total x taxa."""
    n = len(pontos)
    if n < 2:
        return 0.0
    media_x = sum(x for x, _ in pontos) / n
    media_y = sum(y for _, y in pontos) / n
    numerador = sum((x - media_x) * (y - media_y) for x, y in pontos)
    denominador = sum((x - media_x) ** 2 for x, _ in pontos)
    return numerador / denominador if denominador else 0.0


def tabela_taxa(dados):
    linhas = ["| Taxa alvo | Prototipo | Taxa efetiva | Desvio p50 (us) | Desvio p99 (us) | Desvio max (us) | Despachos atrasados | Erros | Sustentou |",
              "|---|---|---|---|---|---|---|---|---|"]
    for agregado in dados:
        linhas.append("| {} /s | {} | {} | {} | {} | {} | {} | {} | {} |".format(
            f"{agregado['taxa_alvo']:,}".replace(",", "."),
            agregado["prototipo"],
            com_margem(agregado, "taxa_efetiva"),
            com_margem(agregado, "desvio_p50_us", 0),
            com_margem(agregado, "desvio_p99_us", 0),
            com_margem(agregado, "desvio_max_us", 0),
            com_margem(agregado, "despachos_atrasados", 0),
            com_margem(agregado, "erros", 0),
            "sim" if sustentou(agregado) else "**nao**"))
    return "\n".join(linhas)


def tabela_recursos(dados):
    linhas = ["| Taxa alvo | Prototipo | RSS repouso (MB) | RSS sob carga (MB) | CPU (% de 1 nucleo) | Latencia corrigida p99 (us) | Latencia de servico p99 (us) |",
              "|---|---|---|---|---|---|---|"]
    for agregado in dados:
        linhas.append("| {} /s | {} | {} | {} | {} | {} | {} |".format(
            f"{agregado['taxa_alvo']:,}".replace(",", "."),
            agregado["prototipo"],
            com_margem(agregado, "rss_repouso_mb"),
            com_margem(agregado, "rss_sob_carga_mb"),
            com_margem(agregado, "cpu_percentual_de_um_nucleo"),
            com_margem(agregado, "latencia_corrigida_p99_us", 0),
            com_margem(agregado, "latencia_de_servico_p99_us", 0)))
    return "\n".join(linhas)


def tabela_concorrencia(dados):
    linhas = ["| Taxa alvo | Prototipo | Pico de requisicoes em andamento | Taxa efetiva | Desvio p99 (us) | Erros | RSS sob carga (MB) |",
              "|---|---|---|---|---|---|---|"]
    for agregado in dados:
        linhas.append("| {} /s | {} | {} | {} | {} | {} | {} |".format(
            f"{agregado['taxa_alvo']:,}".replace(",", "."),
            agregado["prototipo"],
            com_margem(agregado, "pico_em_andamento", 0),
            com_margem(agregado, "taxa_efetiva"),
            com_margem(agregado, "desvio_p99_us", 0),
            com_margem(agregado, "erros", 0),
            com_margem(agregado, "rss_sob_carga_mb")))
    return "\n".join(linhas)


def custo_marginal(dados):
    saida = []
    for prototipo in ("java", "go"):
        pontos = []
        for agregado in dados:
            if agregado["prototipo"] != prototipo or not sustentou(agregado):
                continue
            taxa = agregado["taxa_efetiva"]["media"]
            cpu_ns_por_segundo = agregado["cpu_percentual_de_um_nucleo"]["media"] / 100 * 1e9
            pontos.append((taxa, cpu_ns_por_segundo))
        marginal = inclinacao(pontos)
        base = (sum(y for _, y in pontos) / len(pontos) - marginal * sum(x for x, _ in pontos) / len(pontos)) if pontos else 0
        saida.append((prototipo, len(pontos), marginal, base / 1e9))
    return saida


def main():
    taxa = carregar("taxa")
    concorrencia = carregar("concorrencia")
    startup = json.loads((BRUTO / "resumo-startup.json").read_text()) if (BRUTO / "resumo-startup.json").exists() else {}

    if taxa:
        print("### Taxa de chegada e precisao do agendamento\n")
        print(tabela_taxa(taxa))
        print("\n### Recursos do gerador\n")
        print(tabela_recursos(taxa))
        print("\n### Custo marginal de CPU por requisicao\n")
        print("| Prototipo | Pontos usados | Custo marginal (us de CPU/req) | Piso do agendador (nucleos) |")
        print("|---|---|---|---|")
        for prototipo, pontos, marginal, piso in custo_marginal(taxa):
            print(f"| {prototipo} | {pontos} | {marginal/1000:.1f} | {piso:.2f} |")
    if concorrencia:
        print("\n### Concorrencia (alvo com 1 s de latencia)\n")
        print(tabela_concorrencia(concorrencia))
    if startup:
        print("\n### Startup\n")
        print("| Prototipo | Startup (ms) | Amostras |")
        print("|---|---|---|")
        for prototipo, dados in startup.items():
            print(f"| {prototipo} | {dados['media']:.1f} ± {dados['margem']:.1f} | {dados['n']} |")


if __name__ == "__main__":
    main()
