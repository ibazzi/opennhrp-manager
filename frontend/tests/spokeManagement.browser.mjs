// With Vite on 127.0.0.1:5188 and Chrome --remote-debugging-port=9224,
// run: node tests/spokeManagement.browser.mjs (Node 22+). All API data is mocked.
import assert from 'node:assert/strict'

const origin = 'http://127.0.0.1:5188'
const page = await (await fetch('http://127.0.0.1:9224/json/new?about:blank', { method: 'PUT' })).json()
const ws = new WebSocket(page.webSocketDebuggerUrl)
await new Promise(resolve => ws.addEventListener('open', resolve, { once: true }))
let sequence = 0
const spokeRequests = new Set()
const operationRequests = []
const pending = new Map()
const send = (method, params = {}) => new Promise((resolve, reject) => {
  pending.set(++sequence, { resolve, reject })
  ws.send(JSON.stringify({ id: sequence, method, params }))
})
const user = { id: 1, username: 'test-admin', role: 'admin' }
const nodes = [
  { id: 'hub-a', name: '北京 Hub', type: 'hub', status: 'online', role: 'leader', service_avail: true },
  { id: 'hub-b', name: '上海 Hub', type: 'hub', status: 'online', role: 'follower', service_avail: true },
  { id: 'hub-fail', name: '故障 Hub', type: 'hub', status: 'offline', role: 'standby', service_avail: false },
  { id: 'ctyun', name: '天翼云', type: 'spoke', status: 'online', role: 'spoke', service_avail: true },
  { id: 'offline', name: '离线设备', type: 'spoke', status: 'offline', role: 'spoke', service_avail: false },
]
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
    return
  }
  if (message.method !== 'Fetch.requestPaused') return
  const { requestId, request } = message.params
  const url = new URL(request.url)
  const path = url.pathname
  if ((request.method === 'POST' || request.method === 'PATCH') && (path.startsWith('/api/spokes/') || path.startsWith('/api/managed-spokes/'))) operationRequests.push(`${request.method} ${path}?${url.searchParams}`)
  let body = []
  let responseCode = 200
  if (path === '/api/auth/me') body = user
  else if (path === '/api/config/nodes') body = nodes
  else if (path === '/api/spokes') {
    const hub = url.searchParams.get('node_id')
    spokeRequests.add(hub)
    if (hub === 'hub-fail') {
      responseCode = 503
      body = { error: 'offline' }
    } else {
      body = [{ protocol_address: '10.164.0.252/24', nbma_address: hub === 'hub-a' ? '192.0.2.2' : '198.51.100.2', interface: 'tun0', type: 'dynamic', registration_mode: hub === 'hub-a' ? 'ha' : 'legacy', flags: 'up', expires_in_sec: 600, managed_node_id: 'ctyun', managed_status: 'online' }]
    }
  } else if (path === '/api/managed-spokes') body = devices
  else if (request.method === 'PATCH' && path === '/api/managed-spokes/ctyun') body = { success: true }
  else if (path === '/api/config/interfaces') body = [{ name: 'tun0', protocol_address: '10.164.0.252/24' }]
  else if (path === '/api/config/file') body = { content: 'interface tun0' }
  else if (path === '/api/audit-logs') body = { items: [], total: 0 }
  else if (path.endsWith('/ha')) body = { candidates: [
    { member: 'hub-a', state: 'ready', ready: true, active: true, authenticated: true, selected_address: '192.0.2.1', term: 1, leader: 'hub-a', srtt_ms: 12.5, quality_rtt_ms: 20.1, loss_pct: 1.67, quality_samples: 60, quality_failures: 1, last_quality_reply_age_ms: 150, quality_valid: true, loss_score: 56.666667, latency_score: 27.245455, priority_score: 10, score: 94 },
    { member: 'unknown', state: 'probing', ready: false, authenticated: false, selected_address: '', term: 1, srtt_ms: 0, quality_rtt_ms: null, loss_pct: 100, quality_samples: 0, quality_failures: 0, last_quality_reply_age_ms: null, quality_valid: false, loss_score: 0, latency_score: 0, priority_score: 10, score: 0 },
  ], active_member: 'hub-a', selection_mode: 'auto' }
  await send('Fetch.fulfillRequest', { requestId, responseCode, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body: Buffer.from(JSON.stringify(body)).toString('base64') })
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

try {
  await send('Page.enable')
  await send('Fetch.enable', { patterns: [{ urlPattern: `${origin}/api/*` }] })
  await send('Page.addScriptToEvaluateOnNewDocument', { source: `
    localStorage.setItem('opennhrp_token', 'local-browser-test');
    localStorage.setItem('opennhrp_user', ${JSON.stringify(JSON.stringify(user))});
  ` })
  for (const width of [1280, 375]) {
    spokeRequests.clear()
    await send('Emulation.setDeviceMetricsOverride', { width, height: 900, deviceScaleFactor: 1, mobile: width < 769 })
    await send('Page.navigate', { url: `${origin}/spokes` })
    await until(`document.body.textContent.includes('离线设备') && document.body.textContent.includes('故障 Hub')`)
    assert.deepEqual([...spokeRequests].sort(), ['hub-a', 'hub-b', 'hub-fail'])
    assert.equal(await evaluate(`document.querySelectorAll('.n-tabs-tab').length`), 0)
    assert.equal(await evaluate(`[...document.querySelectorAll('tbody tr')].filter(e => e.textContent.includes('10.164.0.252/24')).length`), 2)
    assert.equal(await evaluate(`[...document.querySelectorAll('thead th')].map(e => e.textContent.trim()).join('|')`), 'Protocol IP|所属 Hub|Spoke / 备注|接入信息|运行状态|Agent 遥测|最后心跳|操作')
    assert.equal(await evaluate(`[...document.querySelectorAll('tbody tr')].some(e => e.textContent.includes('北京 Hub') && e.textContent.includes('天翼云'))`), true)
    assert.equal(await evaluate(`document.querySelector('tbody .access-tags').textContent.trim()`), 'dynamicHA')
    assert.equal(await evaluate(`document.querySelector('tbody .access-tags').children.length`), 2)
    assert.equal(await evaluate(`[...document.querySelectorAll('tbody .access-tags')].some(e => e.textContent.trim() === 'dynamiclegacy')`), true)
    assert.equal(await evaluate(`getComputedStyle(document.querySelector('tbody .access-tags')).flexWrap`), 'nowrap')
    assert.equal(await evaluate(`new Set([...document.querySelector('tbody .access-tags').children].map(e => e.getBoundingClientRect().top)).size`), 1)
    assert.equal(await evaluate(`[...document.querySelectorAll('tbody tr')].find(e => e.textContent.includes('北京 Hub')).querySelectorAll('td:nth-child(1) .n-tag, td:nth-child(3) .n-tag, td:nth-child(1) code, td:nth-child(3) code').length`), 0)
    assert.equal(await evaluate(`document.body.textContent.includes('OpenNHRP 可用') || document.body.textContent.includes('OpenNHRP 不可用')`), false)
    assert.equal(await evaluate(`document.documentElement.scrollHeight <= innerHeight && document.body.scrollHeight <= innerHeight`), true)
    await evaluate(`[...document.querySelectorAll('tbody tr')].find(e => e.textContent.includes('北京 Hub') && e.textContent.includes('天翼云')).click()`)
    await until(`location.search.includes('node=ctyun') && !!document.querySelector('.manage-modal')`)
    await until(`document.body.textContent.includes('评分 RTT') && document.body.textContent.includes('20.1 ms')`)
    await new Promise(resolve => setTimeout(resolve, 250))
    assert.equal(await evaluate(`document.body.textContent.includes('opennhrp.conf') || document.body.textContent.includes('节点实时日志')`), false)
    assert.equal(await evaluate(`document.querySelector('.manage-modal').getBoundingClientRect().width <= 1401`), true)
    if (width === 1280) assert.equal(await evaluate(`document.querySelector('.manage-modal').getBoundingClientRect().width > 1200`), true)
    assert.equal(await evaluate(`document.querySelectorAll('.manage-modal table').length`), 3)
    assert.equal(await evaluate(`new Set([...document.querySelectorAll('.manage-modal table')].map(e => Math.round(e.getBoundingClientRect().top))).size`), 3)
    assert.equal(await evaluate(`document.querySelector('.manage-modal').scrollWidth <= document.querySelector('.manage-modal').clientWidth`), true)
    assert.equal(await evaluate(`document.querySelectorAll('.manage-modal .n-scrollbar').length`), 0)
    assert.equal(await evaluate(`document.body.textContent.includes('1 / 60 次失败')`), true)
    const unknown = await evaluate(`[...document.querySelectorAll('tr')].find(e => e.textContent.includes('unknown')).textContent`)
    assert.ok(unknown.includes('测量不足或已过期') && !unknown.includes('100.0%'))
    await evaluate(`[...document.querySelectorAll('.n-base-close')].find(e => e.offsetParent !== null).click()`)
    await until(`!location.search.includes('node=') && !document.querySelector('.manage-modal')`)
    await evaluate('history.back()')
    await until(`location.search.includes('node=ctyun') && !!document.querySelector('.manage-modal')`)
    await evaluate('history.forward()')
    await until(`!document.querySelector('.manage-modal')`)
    await send('Page.navigate', { url: `${origin}/managed-spokes?node=ctyun&tab=managed` })
    await until(`location.pathname === '/spokes' && location.search.includes('node=ctyun') && !location.search.includes('tab=') && !!document.querySelector('.manage-modal')`)
    console.log(`PASS ${width}px: all Hubs, merged rows, failure isolation, dialog, history and redirect`)
  }
  await send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false })
  await send('Page.navigate', { url: `${origin}/config` })
  await until(`document.body.textContent.includes('OpenNHRP 配置文件编辑') && document.body.textContent.includes('北京 Hub（hub-a / Hub）')`)
  assert.equal(await evaluate(`document.body.textContent.includes('OpenNHRP 进程操作')`), false)
  assert.equal(await evaluate(`document.body.textContent.includes('保存配置文件 (Save)')`), true)
  assert.equal(await evaluate(`[...document.querySelectorAll('.editor-actions button')].map(e => e.textContent.trim()).join('|')`), '添加静态映射|保存 Map|清除重定向缓存|热重载配置 (Reload)|保存配置文件 (Save)')
  await evaluate(`document.querySelector('.n-base-selection').click()`)
  await until(`document.body.textContent.includes('天翼云 (Spoke · 在线)')`)
  await evaluate(`[...document.querySelectorAll('.n-base-select-option')].find(e => e.textContent.includes('天翼云 (Spoke')).click()`)
  await until(`document.body.textContent.includes('天翼云（ctyun / Spoke）')`)
  await evaluate(`[...document.querySelectorAll('button')].find(e => e.textContent.trim() === '保存 Map').click()`)
  await until(`[...document.querySelectorAll('.n-popover')].some(e => e.offsetParent !== null && e.textContent.includes('持久化当前 Map'))`)
  await evaluate(`[...document.querySelectorAll('.n-popover')].find(e => e.offsetParent !== null && e.textContent.includes('持久化当前 Map')).querySelector('.n-button--primary-type').click()`)
  await new Promise(resolve => setTimeout(resolve, 200))
  assert.ok(operationRequests.some(value => value.includes('/api/spokes/map/save?node_id=ctyun') && value.includes('interface=tun0')))
  await evaluate(`[...document.querySelectorAll('button')].find(e => e.textContent.trim() === '清除重定向缓存').click()`)
  await until(`[...document.querySelectorAll('.n-popover')].some(e => e.offsetParent !== null && e.textContent.includes('全部重定向与限流缓存'))`)
  await evaluate(`[...document.querySelectorAll('.n-popover')].find(e => e.offsetParent !== null && e.textContent.includes('全部重定向与限流缓存')).querySelector('.n-button--primary-type').click()`)
  await evaluate(`[...document.querySelectorAll('button')].find(e => e.textContent.trim() === '添加静态映射').click()`)
  await until(`[...document.querySelectorAll('.n-card-header__main')].some(e => e.textContent.includes('添加静态 NHRP 映射'))`)
  await evaluate(`(() => {
    const set = (label, value) => {
      const item = [...document.querySelectorAll('.n-form-item')].find(e => e.textContent.includes(label) && e.querySelector('input:not([disabled])'));
      const input = item.querySelector('input:not([disabled])');
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, value);
      input.dispatchEvent(new Event('input', { bubbles: true }));
    };
    set('Protocol IP', '10.164.0.99');
    set('NBMA 地址', '192.0.2.99');
  })()`)
  await evaluate(`[...document.querySelectorAll('button')].find(e => e.offsetParent !== null && e.textContent.trim() === '添加').click()`)
  await new Promise(resolve => setTimeout(resolve, 200))
  assert.ok(operationRequests.some(value => value.includes('/api/spokes/redirect/purge?node_id=ctyun')))
  assert.ok(operationRequests.some(value => value.includes('/api/spokes/map?node_id=ctyun')))
  console.log('PASS config: Hub/Spoke selector and all selected-node process operations')
} finally {
  await send('Page.close')
  ws.close()
}
