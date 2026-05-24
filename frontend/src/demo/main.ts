import QRCode from 'qrcode'
import {
  buildEasyPayDemoPayload,
  pickQrValue,
  type EasyPayCreateResponse,
  type EasyPayDemoForm,
} from './easypayDemo'

function input(id: string): HTMLInputElement | HTMLSelectElement {
  const el = document.getElementById(id)
  if (el instanceof HTMLInputElement || el instanceof HTMLSelectElement) {
    return el
  }
  throw new Error(`missing input: ${id}`)
}

function element<T extends HTMLElement>(id: string, ctor: new (...args: any[]) => T): T {
  const el = document.getElementById(id)
  if (el instanceof ctor) return el
  throw new Error(`missing element: ${id}`)
}

const formEl = element('payment-form', HTMLFormElement)
const submitEl = element('submit-button', HTMLButtonElement)
const resultEl = element('result', HTMLElement)
const statusEl = element('status', HTMLElement)
const responseEl = element('raw-response', HTMLElement)
const qrCanvas = element('qr-code', HTMLCanvasElement)
const qrTextEl = element('qr-text', HTMLElement)
const tradeNoEl = element('trade-no', HTMLElement)
const copyQrEl = element('copy-qr', HTMLButtonElement)
const resetOrderEl = element('reset-order', HTMLButtonElement)
const syncButtonEl = element('sync-button', HTMLButtonElement)
const platformNotifyUrlEl = element('platform_notify_url', HTMLElement)
const callbackStatusEl = element('callback-status', HTMLElement)
const callbackEventsEl = element('callback-events', HTMLElement)

let latestQrValue = ''
let loading = false
let pollTimer: number | undefined
let pollAttempts = 0
let currentOutTradeNo = ''

function makeOrderNo(): string {
  return `DEMO${Date.now()}`
}

function setStatus(text: string, type: 'info' | 'success' | 'error' = 'info') {
  statusEl.textContent = text
  statusEl.dataset.type = type
  statusEl.hidden = text === ''
}

function timeoutSignal(ms: number): AbortSignal {
  if (typeof AbortSignal.timeout === 'function') return AbortSignal.timeout(ms)
  const controller = new AbortController()
  window.setTimeout(() => controller.abort(), ms)
  return controller.signal
}

function readForm(): EasyPayDemoForm {
  return {
    pid: input('pid').value,
    pkey: input('pkey').value,
    outTradeNo: input('out_trade_no').value,
    name: input('name').value,
    money: input('money').value === '' ? null : Number(input('money').value),
    notifyUrl: input('notify_url').value,
    returnUrl: input('return_url').value,
    device: input('device').value,
    method: input('method').value,
    param: input('param').value,
    sitename: input('sitename').value,
  }
}

function validateForm(form: EasyPayDemoForm): string | null {
  if (form.pid.trim() === '') return '请填写商户 PID'
  if (form.pkey.trim() === '') return '请填写商户 PKEY'
  if (form.outTradeNo.trim() === '') return '请填写商户订单号'
  if (!/^[a-zA-Z0-9._\-|]+$/.test(form.outTradeNo.trim())) return '订单号只能包含字母、数字、点、下划线、短横线和竖线'
  if (form.name.trim() === '') return '请填写商品名称'
  if (form.money === null || !Number.isFinite(form.money) || form.money < 0.01) return '订单金额必须大于 0.01'
  if (form.notifyUrl.trim() === '') return '请填写异步通知地址 notify_url'
  if (form.returnUrl.trim() === '') return '请填写同步跳转地址 return_url'
  return null
}

function clearResult() {
  latestQrValue = ''
  resultEl.hidden = true
  responseEl.textContent = ''
  qrTextEl.textContent = '-'
  tradeNoEl.textContent = '-'
  qrCanvas.getContext('2d')?.clearRect(0, 0, qrCanvas.width, qrCanvas.height)
  qrCanvas.hidden = false
  setStatus('', 'info')
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
  callbackStatusEl.textContent = '等待支付成功后的 Epay 转发回调。'
  callbackEventsEl.textContent = ''
  syncButtonEl.hidden = true
}

async function renderResult(data: EasyPayCreateResponse) {
  responseEl.textContent = JSON.stringify(data, null, 2)
  resultEl.hidden = false

  if (data.code !== 1) {
    latestQrValue = ''
    tradeNoEl.textContent = '-'
    qrTextEl.textContent = data.msg || '生成支付信息失败'
    qrCanvas.hidden = true
    setStatus(data.msg || '生成支付信息失败', 'error')
    return
  }

  latestQrValue = pickQrValue(data)
  tradeNoEl.textContent = data.trade_no || '-'
  qrTextEl.textContent = latestQrValue || '-'

  if (latestQrValue === '') {
    qrCanvas.hidden = true
    setStatus('响应中没有 qrcode / payurl，无法生成二维码', 'error')
    return
  }

  qrCanvas.hidden = false
  await QRCode.toCanvas(qrCanvas, latestQrValue, {
    width: 248,
    margin: 2,
    errorCorrectionLevel: 'M',
    color: { dark: '#0d253dff', light: '#ffffffff' },
  })
  setStatus('支付信息生成成功，请扫码支付', 'success')
}

function renderCallbackEvents(events: Array<Record<string, any>>) {
  callbackEventsEl.textContent = ''
  if (events.length === 0) {
    callbackStatusEl.textContent = '还没有收到商户回调；支付成功后 Epay 会 GET notify_url。'
    return
  }

  callbackStatusEl.textContent = `已收到 ${events.length} 条商户回调。`
  for (const event of events) {
    const item = document.createElement('div')
    item.className = 'callback-event'
    item.textContent = JSON.stringify(event, null, 2)
    callbackEventsEl.appendChild(item)
  }
}

async function fetchCallbackEvents(outTradeNo: string) {
  const resp = await fetch(`/demo/notify-events?out_trade_no=${encodeURIComponent(outTradeNo)}`, {
    signal: timeoutSignal(5_000),
  })
  const text = await resp.text()
  if (!resp.ok) {
    throw new Error(text || `/demo/notify-events HTTP ${resp.status}`)
  }
  const data = JSON.parse(text) as { data?: { events?: Array<Record<string, any>> } }
  const events = data.data?.events || []
  renderCallbackEvents(events)
  if (events.length > 0 && pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

async function syncOrder(outTradeNo: string) {
  syncButtonEl.disabled = true
  syncButtonEl.textContent = '同步中...'
  try {
    const resp = await fetch(`/demo/sync?out_trade_no=${encodeURIComponent(outTradeNo)}`, { signal: timeoutSignal(30_000) })
    const text = await resp.text()
    if (!resp.ok) throw new Error(text)
    const data = JSON.parse(text) as { code: number; msg?: string; data?: { status: string } }
    if (data.code === 0) {
      callbackStatusEl.textContent = `订单状态：${data.data?.status || '-'}。正在拉取回调记录...`
      await fetchCallbackEvents(outTradeNo)
    } else {
      callbackStatusEl.textContent = `同步失败：${data.msg || 'unknown'}`
    }
  } catch (err) {
    callbackStatusEl.textContent = err instanceof Error ? err.message : '同步请求失败'
  } finally {
    syncButtonEl.disabled = false
    syncButtonEl.textContent = '手动同步支付状态'
  }
}

function startCallbackPolling(outTradeNo: string) {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
  }
  pollAttempts = 0
  callbackStatusEl.textContent = '二维码已生成，正在等待 Epay 转发商户回调...'
  void fetchCallbackEvents(outTradeNo).catch(() => undefined)
  pollTimer = window.setInterval(() => {
    pollAttempts += 1
    if (pollAttempts > 100) {
      window.clearInterval(pollTimer)
      pollTimer = undefined
      callbackStatusEl.textContent = '已停止轮询；如已支付但未看到回调，请检查支付宝是否通知到 Epay /api/alipay/notify。'
      return
    }
    void fetchCallbackEvents(outTradeNo).catch((err) => {
      callbackStatusEl.textContent = err instanceof Error ? err.message : '查询回调记录失败'
    })
  }, 3_000)
}

async function readJsonResponse(resp: Response): Promise<EasyPayCreateResponse> {
  const text = await resp.text()
  if (!resp.ok) {
    const detail = text.trim().slice(0, 160)
    throw new Error(detail === '' ? `/mapi.php 请求失败：HTTP ${resp.status}` : `/mapi.php 请求失败：HTTP ${resp.status} ${detail}`)
  }
  try {
    return JSON.parse(text) as EasyPayCreateResponse
  } catch {
    throw new Error('/mapi.php 返回了非 JSON 响应')
  }
}

async function createPayment() {
  if (loading) return

  const form = readForm()
  const invalid = validateForm(form)
  if (invalid) {
    setStatus(invalid, 'error')
    return
  }

  loading = true
  submitEl.disabled = true
  submitEl.textContent = '生成中...'
  clearResult()
  setStatus('正在调用 /mapi.php 生成支付信息...', 'info')
  currentOutTradeNo = form.outTradeNo.trim()


  try {
    const resp = await fetch('/mapi.php', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
      body: buildEasyPayDemoPayload(form),
      signal: timeoutSignal(15_000),
    })
    const data = await readJsonResponse(resp)
    await renderResult(data)
    if (data.code === 1) {
      syncButtonEl.hidden = false
      startCallbackPolling(form.outTradeNo)
    }
  } catch (err) {
    const msg = err instanceof DOMException && (err.name === 'AbortError' || err.name === 'TimeoutError') ? '请求超时，请稍后重试' : err instanceof Error ? err.message : '生成支付信息失败'
    setStatus(msg, 'error')
  } finally {
    loading = false
    submitEl.disabled = false
    submitEl.textContent = '生成二维码支付信息'
  }
}

formEl.addEventListener('submit', (event) => {
  event.preventDefault()
  void createPayment()
})

function clearResultWhenIdle() {
  if (!loading) clearResult()
}

formEl.addEventListener('input', clearResultWhenIdle)
formEl.addEventListener('change', clearResultWhenIdle)

resetOrderEl.addEventListener('click', () => {
  input('out_trade_no').value = makeOrderNo()
  clearResult()
})

copyQrEl.addEventListener('click', () => {
  if (latestQrValue === '') {
    setStatus('二维码内容为空', 'error')
    return
  }
  if (!navigator.clipboard?.writeText) {
    setStatus('当前环境不支持剪贴板，请手动复制', 'error')
    return
  }
  navigator.clipboard.writeText(latestQrValue)
    .then(() => setStatus('二维码内容已复制', 'success'))
    .catch(() => setStatus('复制失败，请手动复制', 'error'))
})

syncButtonEl.addEventListener('click', () => {
  void syncOrder(currentOutTradeNo)
})

input('out_trade_no').value = makeOrderNo()
input('notify_url').value = `${window.location.origin}/demo/notify`
input('return_url').value = `${window.location.origin}/demo.html`
platformNotifyUrlEl.textContent = `${window.location.origin}/api/alipay/notify`
clearResult()
