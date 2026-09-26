// Run with Vite on 127.0.0.1:5188 and Chrome --remote-debugging-port=9224.
import assert from 'node:assert/strict'

const origin = 'http://127.0.0.1:5188'
const page = await (await fetch('http://127.0.0.1:9224/json/new?about:blank', { method: 'PUT' })).json()
const ws = new WebSocket(page.webSocketDebuggerUrl)
await new Promise(resolve => ws.addEventListener('open', resolve, { once: true }))
let id = 0
const pending = new Map()
const send = (method, params = {}) => new Promise((resolve, reject) => {
  pending.set(++id, { resolve, reject })
  ws.send(JSON.stringify({ id, method, params }))
})
const user = { id: 1, username: 'test-admin', role: 'admin' }
const nodes = [{ id: 'hub-a', name: 'Hub A', type: 'hub', status: 'online', role: 'leader', service_avail: true }]
const logs = Array.from({ length: 50 }, (_, id) => ({ id, created_at: '2026-09-26T00:00:00Z', node_id: 'hub-a', action: 'reload', operator: 'test-admin', detail: `操作 ${id}`, success: true }))
const arbitrations = [{ id: 1, term: 197, primary_node_id: 'hub-a', backup_node_id: 'hub-b', decision: 'witness_lease_command_timeout', reason: 'holder command failed: context deadline exceeded', recorded_at: '2026-09-26T00:00:00Z' }]

ws.addEventListener('message', async event => {
  const msg = JSON.parse(event.data)
  if (msg.id) {
    const call = pending.get(msg.id)
    pending.delete(msg.id)
    msg.error ? call.reject(msg.error) : call.resolve(msg.result)
    return
  }
  if (msg.method !== 'Fetch.requestPaused') return
  const { requestId, request } = msg.params
  const path = new URL(request.url).pathname
  const body = path === '/api/auth/me' ? user
    : path === '/api/config/nodes' ? nodes
    : path === '/api/config/interfaces' ? [{ name: 'tun0', type: 'gre' }]
    : path === '/api/config/file' ? { content: 'interface tun0' }
    : path === '/api/config/audit-logs' ? { items: logs, total: logs.length }
    : path === '/api/witness/arbitrations' ? arbitrations
    : {}
  await send('Fetch.fulfillRequest', { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body: Buffer.from(JSON.stringify(body)).toString('base64') })
})

const evaluate = async expression => {
  const result = await send('Runtime.evaluate', { expression, returnByValue: true })
  assert.equal(result.exceptionDetails, undefined, JSON.stringify(result.exceptionDetails))
  return result.result.value
}
const until = async expression => {
  for (let i = 0; i < 100; i++) {
    if (await evaluate(expression)) return
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  assert.fail(`Timed out: ${expression}`)
}
const tableIssues = () => evaluate(`(() => {
  const issues = [];
  for (const table of document.querySelectorAll('.n-table, .n-data-table table')) {
    for (const th of table.querySelectorAll('th')) {
      if (getComputedStyle(th).whiteSpace !== 'nowrap' || th.scrollWidth > th.clientWidth + 2) issues.push(th.textContent.trim());
    }
    if (table.getBoundingClientRect().width <= innerWidth) continue;
    let parent = table.parentElement;
    while (parent && parent !== document.body) {
      if (parent.scrollWidth > parent.clientWidth + 2 && ['auto', 'scroll'].includes(getComputedStyle(parent).overflowX)) break;
      parent = parent.parentElement;
    }
    if (!parent || parent === document.body) issues.push('missing horizontal scroll');
  }
  return issues;
})()`)

try {
  await send('Page.enable')
  await send('Fetch.enable', { patterns: [{ urlPattern: `${origin}/api/*` }] })
  const authScript = await send('Page.addScriptToEvaluateOnNewDocument', { source: `
    localStorage.setItem('opennhrp_token', 'local-browser-test');
    localStorage.setItem('opennhrp_user', ${JSON.stringify(JSON.stringify(user))});
  ` })
  for (const width of [877, 1280, 1754, 375]) {
    await send('Emulation.setDeviceMetricsOverride', { width, height: width === 375 ? 520 : 900, deviceScaleFactor: 1, mobile: width === 375 })
    for (const path of ['/config', '/audit', '/spokes']) {
      await send('Page.navigate', { url: `${origin}${path}` })
      await until(`document.querySelector('.page-container') && document.querySelector('.app-scrollbar .n-scrollbar-container')`)
      if (path === '/audit') await until(`document.querySelectorAll('.audit-table tbody tr').length === 50`)
      if (path === '/spokes') await evaluate(`(() => { const tbody = document.querySelector('.table-scroll tbody'); for (let i = 0; i < 30; i++) tbody.append(tbody.firstElementChild.cloneNode(true)); return true })()`)
      const metrics = await evaluate(`(() => {
        const page = document.querySelector('.page-container');
        const area = document.querySelector('.app-scrollbar .n-scrollbar-container');
        const height = page.getBoundingClientRect().height;
        const overflow = getComputedStyle(page).overflowY;
        area.scrollTop = area.scrollHeight;
        return { height, overflow, scrollTop: area.scrollTop, scrollHeight: area.scrollHeight, clientHeight: area.clientHeight, rootHeight: area.parentElement.getBoundingClientRect().height, maxHeight: getComputedStyle(area.parentElement).maxHeight, reachable: page.getBoundingClientRect().bottom <= area.getBoundingClientRect().bottom + 1 };
      })()`)
      assert.deepEqual(await tableIssues(), [], `${width}px ${path}: tables`)
      if (path === '/config') {
        const title = await evaluate(`(() => { const el = document.querySelector('.editor-card .n-card-header__main'); const rect = el.getBoundingClientRect(); return { width: rect.width, height: rect.height, text: el.textContent.trim() } })()`)
        assert.ok(title.width >= 180 && title.height < 80, `${width}px config title compressed: ${JSON.stringify(title)}`)
      }
      if (width === 375) {
        assert.equal(metrics.overflow, 'visible', `${path}: mobile page must not clip content`)
        assert.ok(metrics.scrollTop > 0 && metrics.reachable, `${path}: mobile page bottom must be reachable: ${JSON.stringify(metrics)}`)
        if (path === '/config') {
          assert.ok(await evaluate(`document.querySelector('.editor-wrapper').getBoundingClientRect().height >= 300`))
        }
        if (path === '/audit') assert.ok(await evaluate(`(() => { const area = document.querySelector('.audit-table-scroll'); area.scrollTop = area.scrollHeight; return area.scrollTop > 0 })()`))
      } else {
        assert.equal(metrics.overflow, 'hidden', `${path}: desktop layout`)
        assert.ok(Math.abs(metrics.height - 844) < 2, `${path}: desktop height ${metrics.height}`)
      }
      console.log(`PASS ${width}px ${path}`)
    }
  }
  for (const width of [877, 1280, 1754, 375]) {
    await send('Emulation.setDeviceMetricsOverride', { width, height: width === 375 ? 520 : 900, deviceScaleFactor: 1, mobile: width === 375 })
    for (const path of ['/', '/ha', '/witness', '/users']) {
      await send('Page.navigate', { url: `${origin}${path}` })
      await until(`document.querySelector('.page-container') && document.querySelector('.app-scrollbar .n-scrollbar-container')`)
      if (path === '/witness') await until(`document.querySelector('.sticky-thead') && document.body.textContent.includes('holder command failed')`)
      if (path === '/ha') await evaluate(`(() => {
        const table = [...document.querySelectorAll('.n-table')].find(el => el.textContent.includes('目标 Member ID'));
        const row = document.createElement('tr');
        row.innerHTML = '<td>ddc297f2b4de...</td><td><strong>orcl-jp1</strong></td><td>90</td><td>已声明</td><td>2026/8/25 14:35:44</td><td>删除</td>';
        table.tBodies[0].append(row);
        return true;
      })()`)
      assert.deepEqual(await tableIssues(), [], `${width}px ${path}: tables`)
      if (path === '/witness') {
        const heading = await evaluate(`(() => { const th = [...document.querySelectorAll('.sticky-thead th')].find(el => el.textContent.trim() === '判定依据与推理'); const range = document.createRange(); range.selectNodeContents(th); return { lines: range.getClientRects().length, width: th.getBoundingClientRect().width } })()`)
        assert.ok(heading.lines === 1 && heading.width >= 220, `${width}px witness heading compressed: ${JSON.stringify(heading)}`)
      }
      if (width === 375) assert.ok(await evaluate(`(() => { const page = document.querySelector('.page-container'); const area = document.querySelector('.app-scrollbar .n-scrollbar-container'); area.scrollTop = area.scrollHeight; return page.getBoundingClientRect().bottom <= area.getBoundingClientRect().bottom + 1 })()`), `${path}: mobile page bottom must be reachable`)
      console.log(`PASS ${width}px ${path}`)
    }
  }
  await send('Page.removeScriptToEvaluateOnNewDocument', { identifier: authScript.identifier })
  await evaluate(`localStorage.removeItem('opennhrp_token'); localStorage.removeItem('opennhrp_user')`)
  await send('Emulation.setDeviceMetricsOverride', { width: 375, height: 320, deviceScaleFactor: 1, mobile: true })
  await send('Page.navigate', { url: `${origin}/login` })
  await until(`document.querySelector('.login-card')`)
  assert.ok(await evaluate(`(() => { const area = document.querySelector('.login-wrapper'); area.scrollTop = area.scrollHeight; return area.scrollTop > 0 && document.querySelector('.login-card').getBoundingClientRect().bottom <= area.getBoundingClientRect().bottom + 1 })()`), 'mobile login form must be reachable')
  console.log('PASS 375px /login')
} finally {
  await send('Page.close')
  ws.close()
}
