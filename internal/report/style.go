package report

// The comparison page and the run report share one stylesheet: two pages of the
// same tool that look like two tools make the reader wonder which one is right.
const pageStyle = `{{define "style"}}
<style>
:root {
  --fundo: #ffffff; --texto: #14181f; --suave: #5b6472; --borda: #e2e6ec;
  --passou: #0f7a3d; --falhou: #b3261e; --atencao: #8a5a00; --neutro: #2a5c9a;
  --fundo-cartao: #f7f9fb;
}
@media (prefers-color-scheme: dark) {
  :root { --fundo: #0f1319; --texto: #e8ecf2; --suave: #98a2b3; --borda: #232a35;
          --passou: #4ad07f; --falhou: #ff6b5e; --atencao: #f0b429; --neutro: #6aa6ff;
          --fundo-cartao: #161b23; }
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--fundo); color: var(--texto);
  font: 16px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
main { max-width: 960px; margin: 0 auto; padding: 40px 24px 72px; }
header { border-bottom: 1px solid var(--borda); padding-bottom: 20px; margin-bottom: 28px; }
.cenario { font-size: 14px; color: var(--suave); text-transform: uppercase; letter-spacing: .08em; }
h1 { font-size: 27px; line-height: 1.3; margin: 12px 0 8px; font-weight: 650; }
h1.passou { color: var(--passou); }
h1.falhou, h1.invalido { color: var(--falhou); }
h1.neutro { color: var(--neutro); }
.subtitulo { color: var(--suave); font-size: 16px; margin: 0; }
h2 { font-size: 15px; text-transform: uppercase; letter-spacing: .07em; color: var(--suave);
  margin: 36px 0 12px; font-weight: 600; }
table { width: 100%; border-collapse: collapse; font-variant-numeric: tabular-nums; }
th, td { text-align: right; padding: 9px 10px; border-bottom: 1px solid var(--borda); font-size: 15px; }
th:first-child, td:first-child { text-align: left; }
th { font-size: 13px; color: var(--suave); font-weight: 600; }
td.erro { color: var(--falhou); font-weight: 600; }
.marca { display: inline-block; min-width: 18px; font-size: 12px; color: var(--suave); }
.numeros { display: flex; flex-wrap: wrap; gap: 12px; margin: 0; padding: 0; list-style: none; }
.numeros li { flex: 1 1 150px; background: var(--fundo-cartao); border: 1px solid var(--borda);
  border-radius: 10px; padding: 14px 16px; }
.numeros .valor { font-size: 23px; font-weight: 620; font-variant-numeric: tabular-nums; }
.numeros .rotulo { font-size: 13px; color: var(--suave); }
.leitura { background: var(--fundo-cartao); border: 1px solid var(--borda); border-left: 3px solid var(--neutro);
  border-radius: 8px; padding: 14px 16px; margin: 14px 0; }
.nota { color: var(--suave); font-size: 14px; margin: 10px 0 0; }
ul.frases { list-style: none; padding: 0; margin: 0; }
ul.frases li { padding: 7px 0; border-bottom: 1px solid var(--borda); font-size: 15px; }
ul.frases li:last-child { border-bottom: none; }
.aviso { border-radius: 8px; padding: 13px 16px; margin: 10px 0; border: 1px solid var(--borda); }
.aviso .rotulo { font-size: 12px; text-transform: uppercase; letter-spacing: .08em; font-weight: 700; }
.aviso.alta { border-color: var(--falhou); } .aviso.alta .rotulo { color: var(--falhou); }
.aviso.media { border-color: var(--atencao); } .aviso.media .rotulo { color: var(--atencao); }
.aviso .evidencia { color: var(--suave); font-size: 14px; }
.slo li { display: flex; gap: 10px; align-items: baseline; }
.slo .ok { color: var(--passou); font-weight: 700; }
.slo .nao { color: var(--falhou); font-weight: 700; }
.slo .sem { color: var(--suave); font-weight: 700; }
svg { width: 100%; height: auto; }
svg .grade { stroke: var(--borda); stroke-width: 1; }
svg .eixo { fill: var(--suave); font-size: 12px; }
svg .p50 { fill: none; stroke: var(--neutro); stroke-width: 2; }
svg .p99 { fill: none; stroke: var(--atencao); stroke-width: 2; }
svg .erro { stroke: var(--falhou); stroke-width: 1; opacity: .35; }
.legenda { display: flex; gap: 18px; font-size: 13px; color: var(--suave); margin-top: 6px; }
.legenda .amostra { display: inline-block; width: 14px; height: 3px; vertical-align: middle; margin-right: 6px; }
footer { margin-top: 44px; padding-top: 18px; border-top: 1px solid var(--borda);
  color: var(--suave); font-size: 13px; }
</style>
{{end}}`
