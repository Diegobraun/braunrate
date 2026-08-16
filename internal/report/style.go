package report

// The comparison page and the run report share one stylesheet: two pages of the
// same tool that look like two tools make the reader wonder which one is right.
const pageStyle = `{{define "style"}}
<style>
:root {
  --background: #ffffff; --text: #14181f; --soft: #5b6472; --border: #e2e6ec;
  --passed: #0f7a3d; --failed: #b3261e; --warning: #8a5a00; --neutral: #2a5c9a;
  --card-background: #f7f9fb;
}
@media (prefers-color-scheme: dark) {
  :root { --background: #0f1319; --text: #e8ecf2; --soft: #98a2b3; --border: #232a35;
          --passed: #4ad07f; --failed: #ff6b5e; --warning: #f0b429; --neutral: #6aa6ff;
          --card-background: #161b23; }
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--background); color: var(--text);
  font: 16px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
main { max-width: 960px; margin: 0 auto; padding: 40px 24px 72px; }
header { border-bottom: 1px solid var(--border); padding-bottom: 20px; margin-bottom: 28px; }
.scenario { font-size: 14px; color: var(--soft); text-transform: uppercase; letter-spacing: .08em; }
h1 { font-size: 27px; line-height: 1.3; margin: 12px 0 8px; font-weight: 650; }
h1.passed { color: var(--passed); }
h1.failed, h1.invalid { color: var(--failed); }
h1.neutral { color: var(--neutral); }
.subtitle { color: var(--soft); font-size: 16px; margin: 0; }
h2 { font-size: 15px; text-transform: uppercase; letter-spacing: .07em; color: var(--soft);
  margin: 36px 0 12px; font-weight: 600; }
table { width: 100%; border-collapse: collapse; font-variant-numeric: tabular-nums; }
th, td { text-align: right; padding: 9px 10px; border-bottom: 1px solid var(--border); font-size: 15px; }
th:first-child, td:first-child { text-align: left; }
th { font-size: 13px; color: var(--soft); font-weight: 600; }
td.error { color: var(--failed); font-weight: 600; }
.mark { display: inline-block; min-width: 18px; font-size: 12px; color: var(--soft); }
.numbers { display: flex; flex-wrap: wrap; gap: 12px; margin: 0; padding: 0; list-style: none; }
.numbers li { flex: 1 1 150px; background: var(--card-background); border: 1px solid var(--border);
  border-radius: 10px; padding: 14px 16px; }
.numbers .value { font-size: 23px; font-weight: 620; font-variant-numeric: tabular-nums; }
.numbers .label { font-size: 13px; color: var(--soft); }
.reading { background: var(--card-background); border: 1px solid var(--border); border-left: 3px solid var(--neutral);
  border-radius: 8px; padding: 14px 16px; margin: 14px 0; }
.note { color: var(--soft); font-size: 14px; margin: 10px 0 0; }
ul.sentences { list-style: none; padding: 0; margin: 0; }
ul.sentences li { padding: 7px 0; border-bottom: 1px solid var(--border); font-size: 15px; }
ul.sentences li:last-child { border-bottom: none; }
.warning { border-radius: 8px; padding: 13px 16px; margin: 10px 0; border: 1px solid var(--border); }
.warning .label { font-size: 12px; text-transform: uppercase; letter-spacing: .08em; font-weight: 700; }
.warning.high { border-color: var(--failed); } .warning.high .label { color: var(--failed); }
.warning.medium { border-color: var(--warning); } .warning.medium .label { color: var(--warning); }
.warning .evidence { color: var(--soft); font-size: 14px; }
.slo li { display: flex; gap: 10px; align-items: baseline; }
.slo .ok { color: var(--passed); font-weight: 700; }
.slo .no { color: var(--failed); font-weight: 700; }
.slo .none { color: var(--soft); font-weight: 700; }
svg { width: 100%; height: auto; }
svg .grid { stroke: var(--border); stroke-width: 1; }
svg .axis { fill: var(--soft); font-size: 12px; }
svg .p50 { fill: none; stroke: var(--neutral); stroke-width: 2; }
svg .p99 { fill: none; stroke: var(--warning); stroke-width: 2; }
svg .error { stroke: var(--failed); stroke-width: 1; opacity: .35; }
.legend { display: flex; gap: 18px; font-size: 13px; color: var(--soft); margin-top: 6px; }
.legend .sample { display: inline-block; width: 14px; height: 3px; vertical-align: middle; margin-right: 6px; }
footer { margin-top: 44px; padding-top: 18px; border-top: 1px solid var(--border);
  color: var(--soft); font-size: 13px; }
</style>
{{end}}`
