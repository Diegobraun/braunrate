'use strict'

// The screen reads and writes the .yaml files of -dir, and nothing else. There
// is no field that lives only here: whatever the interface knows how to do, it
// does by editing the file, and the equivalent terminal command stays visible
// the whole time.

const state = { scenarios: [], runs: [], directory: '.', following: null }

const content = document.getElementById('content')
const scenarioList = document.getElementById('scenarios')
const runList = document.getElementById('runs')
const command = document.getElementById('command')

function escape (text) {
  return String(text ?? '').replace(/[&<>"']/g, function (character) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[character]
  })
}

function showCommand (text) {
  command.textContent = text
}

document.getElementById('explanations').addEventListener('change', function () {
  document.body.classList.toggle('without-explanations', !this.checked)
})

// No stored choice means "follow the OS"; the button offers the other of the two.
function nowDark () {
  const chosen = document.documentElement.dataset.theme
  return chosen ? chosen === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches
}
function labelTheme () {
  document.getElementById('theme').textContent = nowDark() ? 'light' : 'dark'
}
labelTheme()
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', labelTheme)
document.getElementById('theme').addEventListener('click', function () {
  const next = nowDark() ? 'light' : 'dark'
  document.documentElement.dataset.theme = next
  try { localStorage.setItem('theme', next) } catch (failure) {}
  labelTheme()
})

document.getElementById('copy').addEventListener('click', async function () {
  try {
    await navigator.clipboard.writeText(command.textContent)
    this.textContent = 'copied'
    setTimeout(() => { this.textContent = 'copy' }, 1500)
  } catch (failure) {
    this.textContent = 'copy it by hand'
  }
})

async function ask (path, options) {
  const response = await fetch(path, options)
  const text = await response.text()
  let body = text
  if ((response.headers.get('content-type') || '').includes('json')) {
    try { body = JSON.parse(text) } catch (failure) { body = { message: text } }
  }
  return { ok: response.ok, status: response.status, body }
}

function errorMessage (response) {
  if (response.body && response.body.message) return response.body.message
  if (typeof response.body === 'string' && response.body) return response.body
  return 'I could not talk to braunrate; is it still up?'
}

// The sidebar is the state of the directory, so it is redrawn whenever
// something changes on disk or a run starts.
async function reloadSides () {
  const scenarios = await ask('/scenarios')
  if (scenarios.ok) state.scenarios = scenarios.body.scenarios || []
  const runs = await ask('/runs')
  if (runs.ok) state.runs = runs.body.runs || []
  drawSides()
}

function drawSides () {
  const current = decodeURIComponent((location.hash.split('/')[2] || ''))
  if (state.scenarios.length === 0) {
    scenarioList.innerHTML = '<li class="empty">none in this folder</li>'
  } else {
    scenarioList.innerHTML = state.scenarios.map(function (scenario) {
      const mark = scenario.name === current ? ' aria-current="page"' : ''
      return `<li><a class="item" href="#/scenario/${encodeURIComponent(scenario.name)}"${mark}>${escape(scenario.name)}</a></li>`
    }).join('')
  }

  if (state.runs.length === 0) {
    runList.innerHTML = '<li class="empty">none yet</li>'
  } else {
    runList.innerHTML = state.runs.slice(0, 8).map(function (run) {
      const verdict = run.verdict || run.status
      const kind = verdict === 'passed' ? 'pass'
        : (run.status === 'running' || verdict === 'in progress') ? 'run'
          : /fail|invalid/.test(verdict) ? 'fail' : 'idle'
      return `<li><a class="item" href="#/run/${run.id}"><span class="rdot ${kind}"></span>${escape(run.id)} · ${escape(run.scenario)}</a></li>`
    }).join('')
  }
}

// ---------------------------------------------------------------- screens

function scenariosScreen () {
  showCommand(`braunrate ui -dir ${state.directory}`)
  if (state.scenarios.length === 0) {
    content.innerHTML = `
      <div class="empty-screen">
        <h1>No scenario in this folder yet</h1>
        <p class="caption">The interface edits the <code>.yaml</code> files of <code>${escape(state.directory)}</code>.
          Three ways to get the first one:</p>
        <div class="options">
          <a class="option" href="#/new"><b>Start from scratch</b>
            <span>a short form that writes a commented scenario</span></a>
          <a class="option" href="#/import"><b>Import</b>
            <span>a "Copy as cURL" from the network panel, or a JMeter .jmx plan</span></a>
          <a class="option" href="#/examples"><b>Browse examples</b>
            <span>published scenarios to read and copy from</span></a>
          <a class="option" href="#/demo"><b>See the demonstration</b>
            <span>runs a complete example, nothing to configure</span></a>
        </div>
        <div id="target-panel"></div>
      </div>`
    drawTarget()
    return
  }
  content.innerHTML = `
    <h1>Scenarios</h1>
    <p class="caption">${state.scenarios.length} file(s) in <code>${escape(state.directory)}</code>.
      Opening a scenario opens the file itself: what you save here is what the terminal reads.</p>
    <table>
      <tr><th>file</th><th>path</th></tr>
      ${state.scenarios.map(function (scenario) {
        return `<tr><td><a href="#/scenario/${encodeURIComponent(scenario.name)}">${escape(scenario.name)}</a></td>
          <td><code>${escape(scenario.path)}</code></td></tr>`
      }).join('')}
    </table>
    <div id="target-panel"></div>`
  drawTarget()
}

async function drawTarget () {
  const panel = document.getElementById('target-panel')
  if (!panel) return
  const current = await ask('/target')
  if (!current.ok) { panel.innerHTML = ''; return }
  render(current.body)
  function render (target) {
    if (target.running) {
      panel.innerHTML = `<div class="notice"><h3>Practice target running</h3>
        <p>Point a scenario's <code>target</code> at <code>${escape(target.address)}</code> — it answers in about 10ms.</p>
        <button class="button" id="target-stop">Stop it</button></div>`
      document.getElementById('target-stop').addEventListener('click', async function () {
        this.disabled = true
        render((await ask('/target', { method: 'DELETE' })).body)
      })
    } else {
      panel.innerHTML = `<div class="notice"><h3>No service to test against?</h3>
        <p>Start a built-in practice target — a small HTTP server to point a scenario at.</p>
        <button class="button" id="target-start">Start a practice target</button></div>`
      document.getElementById('target-start').addEventListener('click', async function () {
        this.disabled = true
        render((await ask('/target', { method: 'POST' })).body)
      })
    }
  }
}

function demoScreen () {
  showCommand('braunrate demo')
  content.innerHTML = `
    <h1>The demonstration runs in the terminal</h1>
    <p class="caption">It starts a fake target, writes the scenario it is about to run and explains
      every number as it appears. There is no way to do that in here without hiding half of what it
      teaches, so the command is this one:</p>
    <pre>braunrate demo</pre>
    <p>And, to watch the tool catch a real problem:</p>
    <pre>braunrate demo --with-failure</pre>
    <p><a class="button" href="#/">back</a></p>`
}

function importScreen () {
  showCommand('braunrate import jmx plan.jmx -output scenario.yaml')
  content.innerHTML = `
    <h1>Import a scenario</h1>
    <p class="caption">Either format becomes a <code>.yaml</code> in <code>${escape(state.directory)}</code>.
      The import is a draft: read the warnings, then it is the file that is the truth, edited right here.</p>
    <div class="tabs">
      <button type="button" class="tab active" data-src="curl">Paste a cURL</button>
      <button type="button" class="tab" data-src="jmx">Upload a .jmx</button>
    </div>
    <div class="panel" id="src-curl">
      <textarea id="curl" spellcheck="false" placeholder='curl https://your-api/orders -H "Authorization: Bearer ..."'></textarea>
      <p class="teaches">Copy the request in the browser's network panel with "Copy as cURL". The token
        becomes <code>\${TOKEN}</code>, read from the environment, and never written to the file.</p>
    </div>
    <div class="panel" id="src-jmx" hidden>
      <label><span>JMeter plan</span><input type="file" id="jmx" accept=".jmx,application/xml,text/xml"></label>
      <p class="teaches">The common subset of a JMeter plan translates; whatever is left out comes back as a
        warning instead of a silent gap. The busiest domain becomes the target.</p>
    </div>
    <div class="bar"><button type="button" class="primary" id="convert">Convert</button>
      <a class="button" href="#/">cancel</a>
      <span class="right" id="import-status"></span></div>
    <div id="warnings"></div>
    <div id="generated" hidden>
      <h2>Generated scenario</h2>
      <textarea id="yaml" spellcheck="false" aria-label="generated scenario in YAML"></textarea>
      <div class="bar">
        <input id="filename" value="imported.yaml" style="width:auto;min-width:220px" aria-label="file name">
        <button type="button" class="primary" id="save-imported">Save scenario</button></div>
    </div>`

  let source = 'curl'
  const tabs = content.querySelectorAll('.tab')
  tabs.forEach(tab => tab.addEventListener('click', function () {
    source = tab.dataset.src
    tabs.forEach(other => other.classList.toggle('active', other === tab))
    document.getElementById('src-curl').hidden = source !== 'curl'
    document.getElementById('src-jmx').hidden = source !== 'jmx'
  }))

  const importStatus = document.getElementById('import-status')
  const warnings = document.getElementById('warnings')
  const generated = document.getElementById('generated')

  document.getElementById('convert').addEventListener('click', async function () {
    let payload = ''
    let suggested = 'imported.yaml'
    if (source === 'curl') {
      payload = document.getElementById('curl').value.trim()
      if (!payload) { importStatus.textContent = 'paste a cURL first'; return }
    } else {
      const file = document.getElementById('jmx').files[0]
      if (!file) { importStatus.textContent = 'choose a .jmx first'; return }
      payload = await file.text()
      suggested = file.name.replace(/\.jmx$/i, '') + '.yaml'
    }
    this.disabled = true
    importStatus.textContent = 'converting…'
    const answer = await ask(`/import/${source}`, { method: 'POST', body: payload })
    this.disabled = false
    importStatus.textContent = ''
    if (!answer.ok) {
      warnings.innerHTML = `<div class="notice error"><h3>I could not import it</h3><pre>${escape(errorMessage(answer))}</pre></div>`
      generated.hidden = true
      return
    }
    const list = answer.body.warnings || []
    warnings.innerHTML = list.length
      ? `<div class="notice"><h3>${list.length} thing(s) to check before running</h3>
          <ul>${list.map(one => `<li>${escape(one)}</li>`).join('')}</ul></div>`
      : ''
    document.getElementById('yaml').value = answer.body.yaml
    document.getElementById('filename').value = suggested
    generated.hidden = false
  })

  document.getElementById('save-imported').addEventListener('click', async function () {
    let file = document.getElementById('filename').value.trim()
    if (!file) { importStatus.textContent = 'name the file first'; return }
    if (!file.endsWith('.yaml') && !file.endsWith('.yml')) file += '.yaml'
    const answer = await ask(`/scenarios/${encodeURIComponent(file)}/text`,
      { method: 'PUT', body: document.getElementById('yaml').value })
    if (!answer.ok) {
      warnings.innerHTML = `<div class="notice error"><h3>I did not save it</h3><pre>${escape(errorMessage(answer))}</pre></div>`
      return
    }
    await reloadSides()
    location.hash = `#/scenario/${encodeURIComponent(file)}`
  })
}

const schemaLine = '# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json'

const scenarioTemplateHTTP = ({ name, target, method, path, rate, duration, p95, errors }) =>
`${schemaLine}
name: ${name}
target: ${target}

# rate: how many requests per second braunrate fires. It fires at that pace
# whether the target is fast or slow, which is what real users do.
load:
  profiles:
    - steady: { rate: ${rate}/s, duration: ${duration} }

scenario:
  - http: ${method} ${path}
    name: ${name.toLowerCase()}
    expect: { status: 200 }

# acceptance criterion: if it goes over, the run exits with code 1 and your CI fails.
slo:
  - global: { p95: < ${p95}, errors: < ${errors} }
`

const scenarioTemplateKafka = ({ name, brokers, topic, rate, duration, p95, errors }) =>
`${schemaLine}
name: ${name}
target: ${brokers}
requires: [kafka]

load:
  profiles:
    - steady: { rate: ${rate}/s, duration: ${duration} }

scenario:
  - kafka:
      topic: ${topic}
      key: "1"
      value: { id: "1" }
    name: ${name.toLowerCase()}

# Broker with authentication? Uncomment and give the values in the session field
# on the scenario page — they resolve from the environment, never from the file.
# messaging:
#   kafka:
#     brokers: [${brokers}]
#     auth: { type: scramSha512, user: "\${KAFKA_USER}", password: "\${KAFKA_PASSWORD}" }

slo:
  - global: { p95: < ${p95}, errors: < ${errors} }
`

const scenarioTemplateAMQP = ({ name, address, queue, rate, duration, p95, errors }) =>
`${schemaLine}
name: ${name}
target: ${address}
requires: [amqp]

load:
  profiles:
    - steady: { rate: ${rate}/s, duration: ${duration} }

scenario:
  - amqp:
      queue: ${queue}
      body: { id: "1" }
    name: ${name.toLowerCase()}

# messaging:
#   amqp:
#     addresses: [${address}]
#     auth: { user: "\${AMQP_USER}", password: "\${AMQP_PASSWORD}" }

slo:
  - global: { p95: < ${p95}, errors: < ${errors} }
`

const scenarioTemplateGraphQL = ({ name, target, rate, duration, p95, errors }) =>
`${schemaLine}
name: ${name}
target: ${target}

load:
  profiles:
    - steady: { rate: ${rate}/s, duration: ${duration} }

scenario:
  - graphql:
      query: |
        query {
          orders { id status }
        }
    name: ${name.toLowerCase()}
    expect: { status: 200 }

slo:
  - global: { p95: < ${p95}, errors: < ${errors} }
`

const templates = { http: scenarioTemplateHTTP, kafka: scenarioTemplateKafka, amqp: scenarioTemplateAMQP, graphql: scenarioTemplateGraphQL }

function newScreen () {
  showCommand('braunrate new scenario.yaml')
  content.innerHTML = `
    <h1>Start from scratch</h1>
    <p class="caption">The form writes a commented <code>.yaml</code> file. From then on the file is
      the truth: everything it does not cover is edited in the text, right here.</p>
    <form class="form" id="form">
      <label><span>File name</span><input name="file" value="scenario.yaml" required>
        <div class="help">Goes into <code>${escape(state.directory)}</code>.</div></label>
      <label><span>Scenario name</span><input name="name" value="Order lookup" required>
        <div class="help">Shows up at the top of the report.</div></label>
      <label><span>Protocol</span>
        <select name="kind" id="kind"><option value="http">HTTP</option><option value="graphql">GraphQL</option><option value="kafka">Kafka</option><option value="amqp">RabbitMQ (AMQP)</option></select></label>

      <div id="g-target">
        <label><span>Target</span><input name="target" value="http://127.0.0.1:8080">
          <div class="help">The endpoint to measure. With none at hand, the built-in target gives you one.</div></label>
      </div>
      <div id="g-http">
        <div class="pair">
          <label><span>Method</span>
            <select name="method"><option>GET</option><option>POST</option><option>PUT</option><option>PATCH</option><option>DELETE</option></select></label>
          <label><span>Path</span><input name="path" value="/orders/1">
            <div class="teaches">A fixed path measures the target's cache, not the target. After saving, swap
              <code>/orders/1</code> for <code>/orders/${'$'}{orders.id}</code> and declare where it comes from.</div></label>
        </div>
      </div>
      <div id="g-kafka" hidden>
        <label><span>Brokers</span><input name="brokers" value="127.0.0.1:9092">
          <div class="help">host:port of the broker. A broker with auth is added on the scenario page.</div></label>
        <label><span>Topic</span><input name="topic" value="orders"></label>
      </div>
      <div id="g-amqp" hidden>
        <label><span>Address</span><input name="address" value="127.0.0.1:5672">
          <div class="help">host:port of RabbitMQ.</div></label>
        <label><span>Queue</span><input name="queue" value="orders"></label>
      </div>

      <div class="pair">
        <label><span>Rate (per second)</span><input name="rate" value="100" required>
          <div class="teaches">The pace the generator fires at, fast target or slow — the way real users
            behave. A tool that waits for the previous response eases off exactly when the system struggles.</div></label>
        <label><span>Duration</span><input name="duration" value="30s" required></label>
      </div>
      <div class="pair">
        <label><span>95% of the responses within</span><input name="p95" value="500ms" required>
          <div class="teaches">"95% within 500ms" means 5% waited longer. The average stays out of the
            report because it hides exactly those.</div></label>
        <label><span>Maximum error rate (%)</span><input name="errors" value="0.1" required>
          <div class="teaches">These two are the acceptance criterion: if either goes over, braunrate exits
            with code 1 and the pipeline fails the build.</div></label>
      </div>
      <div class="bar"><button class="primary" type="submit">Save scenario</button>
        <a class="button" href="#/">cancel</a></div>
      <div id="result"></div>
    </form>`

  const kind = document.getElementById('kind')
  function showGroups () {
    document.getElementById('g-target').hidden = kind.value !== 'http' && kind.value !== 'graphql'
    ;['http', 'kafka', 'amqp'].forEach(one => { document.getElementById('g-' + one).hidden = kind.value !== one })
  }
  kind.addEventListener('change', showGroups)
  showGroups()

  document.getElementById('form').addEventListener('submit', async function (event) {
    event.preventDefault()
    const fields = Object.fromEntries(new FormData(this).entries())
    let file = fields.file.trim()
    if (!file.endsWith('.yaml') && !file.endsWith('.yml')) file += '.yaml'

    const body = (templates[fields.kind] || scenarioTemplateHTTP)(fields)
    const response = await ask(`/scenarios/${encodeURIComponent(file)}/text`, { method: 'PUT', body })
    const result = document.getElementById('result')
    if (!response.ok) {
      result.innerHTML = `<div class="notice error"><h3>I did not save it</h3><p>${escape(errorMessage(response))}</p></div>`
      return
    }
    await reloadSides()
    location.hash = `#/scenario/${encodeURIComponent(file)}`
  })
}

function cssId (text) { return text.replace(/[^a-z0-9]/gi, '-') }
function techLabel (name) { return ({ http: 'REST', graphql: 'GraphQL', kafka: 'Kafka', amqp: 'RabbitMQ' })[name] || name }

async function examplesScreen () {
  showCommand('braunrate new scenario.yaml   # starting from an example')
  content.innerHTML = `
    <h1>Examples</h1>
    <p class="caption">Published scenarios to learn from. "Use this" copies one into
      <code>${escape(state.directory)}</code> as a file you can edit and run.</p>
    <div id="ex-list" class="loading">loading…</div>`
  const list = document.getElementById('ex-list')
  const response = await ask('/examples')
  if (!response.ok) {
    list.className = ''
    list.innerHTML = `<div class="notice error"><h3>I could not read the examples</h3><pre>${escape(errorMessage(response))}</pre></div>`
    return
  }
  list.className = 'examples'
  list.innerHTML = (response.body.examples || []).map(function (example) {
    const id = cssId(example.file)
    const tags = (example.tech || []).map(one => `<span class="tag ${escape(one)}">${escape(techLabel(one))}</span>`).join('')
    const badges = (example.requires || []).map(one => `<span class="badge req">${escape(one)}</span>`).join('')
    return `<div class="example">
      <div class="ex-head">
        <div><b>${escape(example.name || example.file)}</b> ${tags}${badges}
          <div class="ex-sub"><code>${escape(example.file)}</code> · ${example.steps} step(s)</div></div>
        <div class="ex-acts">
          <button class="button" data-view-file="${escape(example.file)}">view</button>
          <input class="ex-name" data-name-for="${escape(example.file)}" value="${escape(example.file)}" aria-label="save as" hidden>
          <button class="button primary" data-use="${escape(example.file)}">Use this</button></div>
      </div>
      <pre class="ex-src" id="src-${id}" hidden></pre></div>`
  }).join('')

  list.querySelectorAll('[data-view-file]').forEach(button => button.addEventListener('click', async function () {
    const file = button.dataset.viewFile
    const pre = document.getElementById('src-' + cssId(file))
    if (pre.textContent) { pre.hidden = !pre.hidden; return }
    const raw = await ask('/examples/' + encodeURIComponent(file))
    pre.textContent = raw.ok ? raw.body : errorMessage(raw)
    pre.hidden = false
  }))
  list.querySelectorAll('[data-use]').forEach(button => button.addEventListener('click', async function () {
    const file = button.dataset.use
    const field = list.querySelector(`[data-name-for="${file.replace(/"/g, '\\"')}"]`)
    if (field && field.hidden) {
      field.hidden = false
      field.focus()
      field.select()
      button.textContent = 'Save as this'
      return
    }
    let name = (field && field.value.trim()) || file
    if (!name.endsWith('.yaml') && !name.endsWith('.yml')) name += '.yaml'
    const raw = await ask('/examples/' + encodeURIComponent(file))
    if (!raw.ok) return
    const saved = await ask('/scenarios/' + encodeURIComponent(name) + '/text', { method: 'PUT', body: raw.body })
    if (!saved.ok) { button.textContent = 'could not copy'; return }
    await reloadSides()
    location.hash = '#/scenario/' + encodeURIComponent(name)
  }))
}

function editorScreen (name) {
  showCommand(`braunrate validate ${name}`)
  content.innerHTML = `
    <p class="crumbs">Scenarios / <b>${escape(name)}</b></p>
    <h1 class="mono">${escape(name)}</h1>
    <p class="caption">The file itself, comments and all — edits from outside show up on reload.</p>
    <div class="bar">
      <button id="debug">Debug one iteration</button>
      <button id="execute" class="primary">Run with load</button>
      <button id="save">Save</button>
      <span class="right" id="status">loading…</span>
    </div>
    <p class="teaches">Debug fires one request to watch the scenario reach its end — do this first.
      Run with load fires the real test at the declared rate. Both save the file before they run;
      Save on its own only writes it.</p>
    <div id="migrate"></div>
    <div id="verdict"></div>
    <div id="credentials"></div>
    <textarea id="text" spellcheck="false" aria-label="scenario in YAML"></textarea>
    <div id="output"></div>`

  const text = document.getElementById('text')
  const status = document.getElementById('status')
  const verdict = document.getElementById('verdict')
  const credentials = document.getElementById('credentials')
  const output = document.getElementById('output')

  ask(`/scenarios/${encodeURIComponent(name)}/text`).then(function (response) {
    if (!response.ok) {
      status.textContent = ''
      verdict.innerHTML = `<div class="notice error"><h3>I could not open it</h3><p>${escape(errorMessage(response))}</p></div>`
      return
    }
    text.value = response.body
    status.textContent = 'saved'
    offerMigrate()
    validate()
  })

  let delay = null
  text.addEventListener('input', function () {
    status.textContent = 'not saved'
    offerMigrate()
    clearTimeout(delay)
    delay = setTimeout(validate, 700)
  })

  function offerMigrate () {
    const box = document.getElementById('migrate')
    if (!/^(nome|alvo|cenario|carga|autenticacao|variaveis|requer):/m.test(text.value)) { box.innerHTML = ''; return }
    if (box.dataset.on) return
    box.dataset.on = '1'
    box.innerHTML = `<div class="notice needs"><h3>This looks like the old Portuguese format</h3>
      <p>The keys moved to English. Convert here, then review and Save.</p>
      <button id="do-migrate" class="button">Convert to English</button></div>`
    document.getElementById('do-migrate').addEventListener('click', async function () {
      this.disabled = true
      this.textContent = 'converting…'
      const answer = await ask('/migrate', { method: 'POST', body: text.value })
      if (!answer.ok) { this.disabled = false; this.textContent = 'Convert to English'; return }
      text.value = answer.body.yaml
      box.innerHTML = ''
      delete box.dataset.on
      status.textContent = 'not saved'
      validate()
    })
  }

  // Validation checks the draft on the screen, not the file on disk, and by the
  // same reading path the terminal uses: the editor never approves what the
  // command would refuse.
  async function validate () {
    const response = await ask(`/scenarios/${encodeURIComponent(name)}/validate`,
      { method: 'POST', body: text.value })
    if (response.ok) {
      const lines = (response.body.lines || []).map(escape).join('\n')
      verdict.innerHTML = `<div class="notice ok"><h3>Valid scenario</h3><pre>${lines}</pre></div>`
      drawCredentials(response.body.needs || [])
      return
    }
    const position = response.body.line ? ` (line ${response.body.line}, column ${response.body.column})` : ''
    verdict.innerHTML = `<div class="notice error"><h3>The scenario does not hold${escape(position)}</h3>
      <pre>${escape(errorMessage(response))}</pre></div>`
  }

  // A credencial nunca vai para o arquivo — o arquivo mantém ${TOKEN}. Aqui a
  // pessoa dá o valor da sessão, que o servidor guarda em memória e some no
  // reinício. Sem isto, quem só usa a tela não tem como preencher o ${TOKEN}, e o
  // único atalho aparente — colar o segredo no YAML — é recusado. Ver ADR 0021.
  function drawCredentials (needs) {
    if (!needs.length) {
      credentials.innerHTML = ''
      return
    }
    credentials.innerHTML = `
      <div class="notice needs">
        <h3>This scenario needs a value the file does not carry</h3>
        <p>The file keeps <code>\${NAME}</code>; give the value here. It is held in memory
           until the server restarts, and never written to the file.</p>
        ${needs.map(function (variableName) {
          return `<label class="credential">
            <span>${escape(variableName)}</span>
            <input type="password" autocomplete="off" data-name="${escape(variableName)}"
              aria-label="value for ${escape(variableName)}">
            <button type="button" class="apply" data-name="${escape(variableName)}">Use for this session</button>
          </label>`
        }).join('')}
      </div>`
    credentials.querySelectorAll('button.apply').forEach(function (button) {
      button.addEventListener('click', async function () {
        const field = credentials.querySelector(`input[data-name="${button.dataset.name}"]`)
        if (!field.value) return
        const response = await ask('/environment',
          { method: 'PUT', body: JSON.stringify({ [button.dataset.name]: field.value }) })
        field.value = ''
        if (response.ok) validate()
      })
    })
  }

  async function save () {
    status.textContent = 'saving…'
    const response = await ask(`/scenarios/${encodeURIComponent(name)}/text`,
      { method: 'PUT', body: text.value })
    if (!response.ok) {
      status.textContent = 'did not save'
      verdict.innerHTML = `<div class="notice error"><h3>I did not save it</h3><p>${escape(errorMessage(response))}</p></div>`
      return false
    }
    status.textContent = 'saved'
    return true
  }

  document.getElementById('save').addEventListener('click', save)

  document.getElementById('debug').addEventListener('click', async function () {
    showCommand(`braunrate debug ${name}`)
    if (!await save()) return
    output.innerHTML = '<p class="loading">running one iteration…</p>'
    const response = await ask(`/scenarios/${encodeURIComponent(name)}/debug`, { method: 'POST' })
    if (!response.ok) {
      output.innerHTML = `<div class="notice error"><h3>The debug run stopped</h3><pre>${escape(errorMessage(response))}</pre></div>`
      return
    }
    const kind = response.body.complete ? 'ok' : 'error'
    const title = response.body.complete
      ? 'Iteration complete: it can be run with load'
      : 'The iteration did not reach the end; load only means something after it passes'
    output.innerHTML = `<h2>Debug</h2><div class="notice ${kind}"><h3>${title}</h3></div>
      <pre>${escape(response.body.text)}</pre>`
  })

  document.getElementById('execute').addEventListener('click', async function () {
    showCommand(`braunrate execute ${name}`)
    if (!await save()) return
    output.innerHTML = '<p class="loading">starting…</p>'
    const response = await ask(`/scenarios/${encodeURIComponent(name)}/runs`, { method: 'POST' })
    if (!response.ok) {
      output.innerHTML = `<div class="notice error"><h3>I did not start</h3><pre>${escape(errorMessage(response))}</pre></div>`
      return
    }
    await reloadSides()
    location.hash = `#/run/${response.body.id}`
  })
}

function runsScreen () {
  showCommand('braunrate compare before.json after.json')
  if (state.runs.length === 0) {
    content.innerHTML = `
      <div class="empty-screen">
        <h1>No run yet</h1>
        <p class="caption">Runs live in the memory of this process and disappear when it restarts —
          there is no database, because a database would be a second truth beside the files.</p>
        <p><a class="button" href="#/">pick a scenario</a></p>
      </div>`
    return
  }
  content.innerHTML = `
    <h1>Runs</h1>
    <p class="caption">Tick two to compare them. Comparing needs both finished.</p>
    <div class="bar"><button id="compare" disabled>Compare the two ticked</button>
      <span class="right" id="ticked">none ticked</span></div>
    <table>
      <tr><th></th><th>id</th><th>scenario</th><th>verdict</th><th>when</th></tr>
      ${state.runs.map(function (run) {
        return `<tr class="selectable" data-id="${escape(run.id)}">
          <td><input type="checkbox" style="width:auto" data-tick="${escape(run.id)}"></td>
          <td><a href="#/run/${escape(run.id)}">${escape(run.id)}</a></td>
          <td>${escape(run.scenario)}</td>
          <td>${escape(run.verdict || run.status)}</td>
          <td>${escape(new Date(run.started_at).toLocaleString())}</td></tr>`
      }).join('')}
    </table>
    <div id="comparison"></div>`

  const button = document.getElementById('compare')
  const ticked = document.getElementById('ticked')
  function selected () {
    return Array.from(document.querySelectorAll('[data-tick]:checked')).map(box => box.dataset.tick)
  }
  content.addEventListener('change', function () {
    const chosen = selected()
    button.disabled = chosen.length !== 2
    ticked.textContent = chosen.length === 0 ? 'none ticked' : chosen.join(' and ')
  })
  button.addEventListener('click', function () {
    const [first, second] = selected()
    location.hash = `#/comparison/${first}/${second}`
  })
}

async function runScreen (id) {
  const run = state.runs.find(one => one.id === id)
  showCommand(`braunrate execute ${run ? run.scenario : '<scenario>.yaml'} -html report.html`)
  const running = run && run.status === 'running'
  content.innerHTML = `
    <p class="crumbs">Runs / <b>${run ? escape(run.scenario) : escape(id)}</b></p>
    <h1>Run ${escape(id)}</h1>
    <p class="caption" id="from-which-scenario">${run ? escape(run.scenario) : 'loading…'}</p>
    <div class="bar">
      <span id="note">${running ? 'in progress — closing this page interrupts nothing' : ''}</span>
      <button type="button" id="cancel" class="button danger"${running ? '' : ' hidden'}>Cancel the run</button>
      <span class="right actions" id="actions"${running ? ' hidden' : ''}>
        <a class="button" id="download" download="${escape(id)}-report.html"
           href="/runs/${encodeURIComponent(id)}/report">Download</a>
        <a class="button" download="${escape(id)}.csv" href="/runs/${encodeURIComponent(id)}/csv">CSV</a>
        <button type="button" class="button" id="save-report">Save to folder</button>
      </span>
    </div>
    <p class="teaches" id="save-note" hidden></p>
    <div class="tabs">
      <button type="button" class="tab" data-panel="log">Log</button>
      <button type="button" class="tab" data-panel="report">Report</button>
    </div>
    <div class="panel" id="panel-log"><pre id="lines">${running ? 'waiting for the first line…' : ''}</pre></div>
    <div class="panel" id="panel-report"><div id="report"></div></div>`

  const report = document.getElementById('report')
  const tabs = content.querySelectorAll('.tab')
  function activate (which) {
    tabs.forEach(tab => tab.classList.toggle('active', tab.dataset.panel === which))
    document.getElementById('panel-log').hidden = which !== 'log'
    document.getElementById('panel-report').hidden = which !== 'report'
  }
  tabs.forEach(tab => tab.addEventListener('click', () => activate(tab.dataset.panel)))
  activate(running ? 'log' : 'report')

  document.getElementById('save-report').addEventListener('click', async function () {
    this.disabled = true
    const label = this.textContent
    this.textContent = 'saving…'
    const answer = await ask(`/runs/${encodeURIComponent(id)}/save`, { method: 'POST' })
    this.disabled = false
    this.textContent = label
    const note = document.getElementById('save-note')
    note.hidden = false
    note.classList.toggle('failed-text', !answer.ok)
    note.textContent = answer.ok ? `saved to ${answer.body.path}` : errorMessage(answer)
  })

  // follow replays past lines before closing, so a finished run fills its Log too.
  const replayed = follow(id, document.getElementById('lines'))

  if (running) {
    // Apontar a carga para o ambiente errado e o erro mais caro deste tipo de
    // teste, e ate aqui a unica saida derrubava o servidor junto.
    document.getElementById('cancel').addEventListener('click', async function () {
      this.disabled = true
      this.textContent = 'canceling…'
      const answer = await ask(`/runs/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (!answer.ok) {
        this.textContent = 'Cancel the run'
        this.disabled = false
        document.getElementById('note').textContent = errorMessage(answer)
      }
    })
    await replayed
    await reloadSides()
    const finished = state.runs.find(one => one.id === id)
    // An ended run cannot be canceled; the report exists now, so its actions appear.
    document.getElementById('cancel').remove()
    document.getElementById('actions').hidden = false
    document.getElementById('note').textContent = finished && finished.verdict
      ? `finished: ${finished.verdict}`
      : 'finished'
    activate('report')
  }

  const response = await ask(`/runs/${id}`)
  if (!response.ok) {
    report.innerHTML = `<div class="notice error"><h3>No report</h3><pre>${escape(errorMessage(response))}</pre></div>`
    return
  }
  const document_ = response.body
  const fromScenario = document_.run && document_.run.scenario
  document.getElementById('from-which-scenario').textContent = fromScenario || (run ? run.scenario : '')
  if (document_.status === 'running') {
    report.innerHTML = '<p class="loading">still running…</p>'
    return
  }
  // An invalid run gets no number in the foreground: the reason comes first, and
  // no green appears above it.
  const sanity = document_.sanity || {}
  if (sanity.checked && !sanity.valid) {
    report.innerHTML = `<div class="notice error"><h3>Invalid result</h3>
      <p>${escape(sanity.sentence)}</p>
      <div class="teaches">No number of this run counts as an answer: it neither approves nor fails, and
        the exit code is 3. Fix what is below and run it again.</div></div>` +
      (sanity.findings || []).map(finding =>
        `<div class="notice error"><p>${escape(finding.message)}</p><p><small>${escape(finding.evidence)}</small></p></div>`).join('')
  }
  report.innerHTML += `<h2>Report</h2><iframe src="/runs/${encodeURIComponent(id)}/report" title="report"></iframe>`
}

// The stream is the same text the terminal prints, one line per update.
async function follow (id, destination) {
  const response = await fetch(`/runs/${encodeURIComponent(id)}/stream`)
  if (!response.ok || !response.body) {
    destination.textContent = 'I could not follow this run'
    return
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let accumulated = ''
  destination.textContent = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    accumulated += decoder.decode(value, { stream: true })
    destination.textContent = accumulated
    destination.scrollTop = destination.scrollHeight
  }
}

function comparisonScreen (before, after) {
  showCommand(`braunrate compare ${before}.json ${after}.json -html comparison.html`)
  content.innerHTML = `
    <h1>${escape(before)} against ${escape(after)}</h1>
    <p class="caption">Two runs give no confidence interval: a change below 5% is noise, and a caveat
      that explains the difference by itself takes the verdict away.</p>
    <iframe src="/runs/${encodeURIComponent(before)}/comparison/${encodeURIComponent(after)}/report" title="comparison"></iframe>
    <p><a class="button" href="#/runs">back</a></p>`
}

// ---------------------------------------------------------------- routing

async function draw () {
  const parts = location.hash.replace(/^#\/?/, '').split('/').map(decodeURIComponent)
  drawSides()
  switch (parts[0]) {
    case 'new': return newScreen()
    case 'import': return importScreen()
    case 'examples': return examplesScreen()
    case 'demo': return demoScreen()
    case 'runs': return runsScreen()
    case 'scenario': return editorScreen(parts[1])
    case 'run': return runScreen(parts[1])
    case 'comparison': return comparisonScreen(parts[1], parts[2])
    default: return scenariosScreen()
  }
}

window.addEventListener('hashchange', draw)

async function start () {
  const health = await ask('/health')
  if (!health.ok) {
    content.innerHTML = `<div class="notice error"><h3>braunrate did not answer</h3>
      <p>The interface is served by the command itself; if it stopped, start it again with
      <code>braunrate ui</code>.</p></div>`
    return
  }
  document.getElementById('version').textContent = health.body.version
  state.directory = health.body.directory || '.'
  await reloadSides()
  await draw()
}

start()
