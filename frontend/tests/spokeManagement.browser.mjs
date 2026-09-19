// With Vite on 127.0.0.1:5188 and Chrome --remote-debugging-port=9224,
// run: node tests/spokeManagement.browser.mjs (Node 22+). All API data is mocked.
import assert from 'node:assert/strict'

const origin = 'http://127.0.0.1:5188'
const page = await (await fetch('http://127.0.0.1:9224/json/new?about:blank', { method: 'PUT' })).json()
const ws = new WebSocket(page.webSocketDebuggerUrl)
await new Promise(resolve => ws.addEventListener('open', resolve, { once: true }))
let sequence = 0, managedRequests = 0
const pending = new Map()
const send = (method, params = {}) => new Promise((resolve, reject) => {
  pending.set(++sequence, { resolve, reject })
  ws.send(JSON.stringify({ id: sequence, method, params }))
})
const user = { id: 1, username: 'test-admin', role: 'admin' }
const spoke = { protocol_address: '10.164.0.252/24', nbma_address: '192.0.2.2', interface: 'tun0', type: 'dynamic', registration_mode: 'ha', flags: 'up', expires_in_sec: 600, managed_node_id: 'ctyun', managed_status: 'online' }
const devices = [
  { id: 'ctyun', name: '天翼云', protocol_address: '10.164.0.252', status: 'online', core_available: true, peer_count: 3, ws_rtt_ms: 20 },
  { id: 'offline', name: '离线设备', status: 'offline', core_available: false, peer_count: 0, ws_rtt_ms: 0 },
]
ws.addEventListener('message', async event => {
  const message = JSON.parse(event.data)
  if (message.id) {
    const callback = pending.get(message.id)
    pending.delete(message.id)
    message.error ? callback.reject(message.error) : callback.resolve(message.result)
  } else if (message.method === 'Fetch.requestPaused') {
    const { requestId, request } = message.params
    const path = new URL(request.url).pathname
    let body = []
    if (path === '/api/auth/me') body = user
    else if (path === '/api/nodes') body = [{ id: 'hub', name: 'hub', type: 'hub', status: 'online', role: 'leader', service_avail: true }]
    else if (path === '/api/spokes') body = [spoke]
    else if (path === '/api/managed-spokes') { body = devices; managedRequests++ }
    else if (path === '/api/config/interfaces') body = [{ name: 'tun0', protocol_address: '10.164.0.252/24' }]
    else if (path === '/api/config/file') body = { content: 'interface tun0' }
    else if (path.endsWith('/ha')) body = { candidates: [
      { member: 'hub', state: 'ready', ready: true, active: true, authenticated: true, selected_address: '192.0.2.1', term: 1, leader: 'hub', srtt_ms: 12.5, quality_rtt_ms: 20.1, loss_pct: 1.67, quality_samples: 60, quality_failures: 1, last_quality_reply_age_ms: 150, quality_valid: true, loss_score: 56.666667, latency_score: 27.245455, priority_score: 10, score: 94 },
      { member: 'unknown', state: 'probing', ready: false, srtt_ms: 0, quality_rtt_ms: null, loss_pct: 100, quality_samples: 0, quality_failures: 0, last_quality_reply_age_ms: null, quality_valid: false, loss_score: 0, latency_score: 0, priority_score: 10, score: 0 },
    ], active_member: 'hub', selection_mode: 'auto' }
    await send('Fetch.fulfillRequest', { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body: Buffer.from(JSON.stringify(body)).toString('base64') })
  }
})
const evaluate = async expression => {
  const result = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  assert.equal(result.exceptionDetails, undefined, JSON.stringify(result.exceptionDetails))
  return result.result.value
}
const until = async expression => {
  for (let i = 0; i < 100; i++) {
    if (await evaluate(expression)) return
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  assert.fail(`Timed out: ${expression}; ${await evaluate('location.href + "\\n" + document.body.innerText')}`)
}
const tab = name => evaluate(`[...document.querySelectorAll('.n-tabs-tab')].find(e => e.textContent.trim() === '${name}').click()`)
try {
  await send('Page.enable')
  await send('Fetch.enable', { patterns: [{ urlPattern: `${origin}/api/*` }] })
  await send('Page.addScriptToEvaluateOnNewDocument', { source: `
    localStorage.setItem('opennhrp_token', 'local-browser-test');
    localStorage.setItem('opennhrp_user', ${JSON.stringify(JSON.stringify(user))});
    window.logConnections = 0;
    const NativeWebSocket = window.WebSocket;
    window.WebSocket = class {
      constructor(url, protocols) {
        if (!url.includes('/api/logs/ws')) return new NativeWebSocket(url, protocols);
        window.logConnections++;
      }
      close() { window.logConnections--; queueMicrotask(() => this.onclose?.()); }
    };
  ` })
  for (const width of [1280, 375]) {
    await send('Emulation.setDeviceMetricsOverride', { width, height: 900, deviceScaleFactor: 1, mobile: width < 769 })
    await send('Page.navigate', { url: `${origin}/spokes` })
    await until(`document.body.textContent.includes('10.164.0.252/24')`)
    assert.equal(await evaluate(`document.querySelectorAll('.n-tabs-tab').length`), 2)
    if (width === 1280) {
      assert.equal(await evaluate(`[...document.querySelectorAll('.n-menu-item-content-header')].filter(e => e.textContent.trim() === 'Spoke 管理').length`), 1)
      assert.equal(await evaluate(`document.body.textContent.includes('Spoke 设备管理') || document.body.textContent.includes('Spoke 客户端管理')`), false)
    }
    assert.equal(managedRequests, 0, 'inactive device tab should not fetch')
    await evaluate(`[...document.querySelectorAll('button')].find(e => e.textContent.trim() === '设备管理').click()`)
    await until(`location.search.includes('node=ctyun') && !!document.querySelector('.terminal-container')`)
    assert.equal(await evaluate(`document.querySelector('tbody tr.selected')?.textContent.includes('天翼云')`), true)
    assert.equal(await evaluate(`document.body.textContent.includes('离线设备')`), true)
    await until(`document.body.textContent.includes('评分 RTT') && document.body.textContent.includes('20.1 ms')`)
    assert.equal(await evaluate(`document.body.textContent.includes('超时估计 SRTT：12.5 ms')`), true)
    assert.equal(await evaluate(`document.body.textContent.includes('1 / 60 次失败')`), true)
    assert.equal(await evaluate(`document.body.textContent.includes('56.67 / 27.25 / 10.00')`), true)
    const unknown = await evaluate(`[...document.querySelectorAll('tr')].find(e => e.textContent.includes('unknown')).textContent`)
    assert.ok(unknown.includes('测量不足或已过期'))
    assert.ok(!unknown.includes('100.0%'), 'unknown quality must not be displayed as measured failure')
    assert.ok(unknown.includes('—'))
    await tab('接入记录')
    await until(`!document.querySelector('.terminal-container') && !location.search.includes('tab=managed')`)
    const requests = managedRequests
    await new Promise(resolve => setTimeout(resolve, 3300))
    assert.equal(managedRequests, requests, 'leaving tab must stop polling')
    assert.equal(await evaluate('window.logConnections'), 0, 'leaving tab must close logs without reconnecting')
    await evaluate('history.back()')
    await until(`!!document.querySelector('.terminal-container')`)
    await evaluate('history.forward()')
    await until(`!document.querySelector('.terminal-container')`)
    await tab('纳管设备')
    await until(`location.search.includes('tab=managed') && document.body.textContent.includes('离线设备')`)
    await send('Page.reload')
    await until(`document.body.textContent.includes('离线设备')`)
    await send('Page.navigate', { url: `${origin}/managed-spokes?node=ctyun` })
    await until(`location.pathname === '/spokes' && location.search.includes('node=ctyun') && !!document.querySelector('.terminal-container')`)
    await tab('接入记录')
    await until(`!document.querySelector('.terminal-container')`)
    console.log(`PASS ${width}px: tabs, deep link, offline device, history, reload, redirect and cleanup`)
    managedRequests = 0
  }
} finally {
  await send('Page.close')
  ws.close()
}
