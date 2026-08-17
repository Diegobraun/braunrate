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
      return `<li><a class="item" href="#/run/${run.id}">${escape(run.id)} · ${escape(run.scenario)}
        <span class="state">${escape(run.verdict || run.status)}</span></a></li>`
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
          <a class="option" href="#/import"><b>Import a cURL</b>
            <span>paste the command copied from the browser's network panel</span></a>
          <a class="option" href="#/demo"><b>See the demonstration</b>
            <span>runs a complete example, nothing to configure</span></a>
        </div>
      </div>`
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
    </table>`
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
  showCommand('braunrate import curl "<command>" -output scenario.yaml')
  content.innerHTML = `
    <h1>Import a cURL</h1>
    <p class="caption">The import happens in the terminal, and the file it writes shows up in the list
      here as soon as it exists. Copy the request in the browser's network panel with
      "Copy as cURL" and run:</p>
    <pre>braunrate import curl "&lt;paste here&gt;" -output scenario.yaml</pre>
    <p>The token becomes <code>\${TOKEN}</code>, read from the environment, and never reaches the repository.</p>
    <p><a class="button" href="#/">back</a></p>`
}

const scenarioTemplate = ({ name, target, method, path, rate, duration, p95, errors }) =>
`# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
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
      <label><span>Target</span><input name="target" value="http://127.0.0.1:8080" required>
        <div class="help">The service to measure. With no service at hand, <code>braunrate target</code> starts one.</div></label>
      <div class="pair">
        <label><span>Method</span>
          <select name="method"><option>GET</option><option>POST</option><option>PUT</option><option>PATCH</option><option>DELETE</option></select></label>
        <label><span>Path</span><input name="path" value="/orders/1" required>
          <div class="teaches">A path with a fixed value measures the target's cache, not the target:
            every request will be identical. After saving, swap <code>/orders/1</code> for
            <code>/orders/${'$'}{orders.id}</code> and declare where <code>${'$'}{orders.id}</code> comes from.</div></label>
      </div>
      <div class="pair">
        <label><span>Rate (requests per second)</span><input name="rate" value="100" required>
          <div class="teaches">Rate is the pace the generator fires at, whether the target is fast or
            slow — the way real users behave. A tool that waits for the previous response eases off the
            system exactly when it is struggling.</div></label>
        <label><span>Duration</span><input name="duration" value="30s" required></label>
      </div>
      <div class="pair">
        <label><span>95% of the responses within</span><input name="p95" value="500ms" required>
          <div class="teaches">"95% within 500ms" means 5% of the people waited longer than that.
            The average stays out of the report because it hides exactly those.</div></label>
        <label><span>Maximum error rate (%)</span><input name="errors" value="0.1" required>
          <div class="teaches">These two limits are the acceptance criterion: if either goes over,
            braunrate exits with code 1 and the pipeline fails the build.</div></label>
      </div>
      <div class="bar"><button class="primary" type="submit">Save scenario</button>
        <a class="button" href="#/">cancel</a></div>
      <div id="result"></div>
    </form>`

  document.getElementById('form').addEventListener('submit', async function (event) {
    event.preventDefault()
    const fields = Object.fromEntries(new FormData(this).entries())
    let file = fields.file.trim()
    if (!file.endsWith('.yaml') && !file.endsWith('.yml')) file += '.yaml'

    const response = await ask(`/scenarios/${encodeURIComponent(file)}/text`,
      { method: 'PUT', body: scenarioTemplate(fields) })
    const result = document.getElementById('result')
    if (!response.ok) {
      result.innerHTML = `<div class="notice error"><h3>I did not save it</h3><p>${escape(errorMessage(response))}</p></div>`
      return
    }
    await reloadSides()
    location.hash = `#/scenario/${encodeURIComponent(file)}`
  })
}

function editorScreen (name) {
  showCommand(`braunrate validate ${name}`)
  content.innerHTML = `
    <h1>${escape(name)}</h1>
    <p class="caption">This is the file, with the comments you wrote. Editing it from outside is seen
      here: reload the page.</p>
    <div class="bar">
      <button id="save">Save</button>
      <button id="debug">Debug one iteration</button>
      <button id="execute" class="primary">Run with load</button>
      <span class="right" id="status">loading…</span>
    </div>
    <div id="verdict"></div>
    <textarea id="text" spellcheck="false" aria-label="scenario in YAML"></textarea>
    <div id="output"></div>`

  const text = document.getElementById('text')
  const status = document.getElementById('status')
  const verdict = document.getElementById('verdict')
  const output = document.getElementById('output')

  ask(`/scenarios/${encodeURIComponent(name)}/text`).then(function (response) {
    if (!response.ok) {
      status.textContent = ''
      verdict.innerHTML = `<div class="notice error"><h3>I could not open it</h3><p>${escape(errorMessage(response))}</p></div>`
      return
    }
    text.value = response.body
    status.textContent = 'saved'
    validate()
  })

  let delay = null
  text.addEventListener('input', function () {
    status.textContent = 'not saved'
    clearTimeout(delay)
    delay = setTimeout(validate, 700)
  })

  // Validation checks the draft on the screen, not the file on disk, and by the
  // same reading path the terminal uses: the editor never approves what the
  // command would refuse.
  async function validate () {
    const response = await ask(`/scenarios/${encodeURIComponent(name)}/validate`,
      { method: 'POST', body: text.value })
    if (response.ok) {
      const lines = (response.body.lines || []).map(escape).join('\n')
      verdict.innerHTML = `<div class="notice ok"><h3>Valid scenario</h3><pre>${lines}</pre></div>`
      return
    }
    const position = response.body.line ? ` (line ${response.body.line}, column ${response.body.column})` : ''
    verdict.innerHTML = `<div class="notice error"><h3>The scenario does not hold${escape(position)}</h3>
      <pre>${escape(errorMessage(response))}</pre></div>`
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
  content.innerHTML = `
    <h1>Run ${escape(id)}</h1>
    <p class="caption" id="from-which-scenario">${run ? escape(run.scenario) : 'loading…'}</p>
    <div id="progress"></div>
    <div id="report"></div>`

  const progress = document.getElementById('progress')
  const report = document.getElementById('report')

  if (run && run.status === 'running') {
    progress.innerHTML = `
      <div class="bar">
        <span class="right">in progress — closing this page interrupts nothing</span>
        <button type="button" id="cancel" class="button danger">Cancel the run</button>
      </div>
      <pre id="lines">waiting for the first line…</pre>`
    // Apontar a carga para o ambiente errado e o erro mais caro deste tipo de
    // teste, e ate aqui a unica saida derrubava o servidor junto.
    document.getElementById('cancel').addEventListener('click', async function () {
      this.disabled = true
      this.textContent = 'canceling…'
      const answer = await ask(`/runs/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (!answer.ok) {
        this.textContent = 'Cancel the run'
        this.disabled = false
        progress.querySelector('.right').textContent = errorMessage(answer)
      }
    })
    await follow(id, document.getElementById('lines'))
    await reloadSides()
    const finished = state.runs.find(one => one.id === id)
    progress.querySelector('.right').textContent = finished && finished.verdict
      ? `finished: ${finished.verdict}`
      : 'finished'
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
