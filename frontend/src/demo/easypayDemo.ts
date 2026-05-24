export interface EasyPayDemoForm {
  pid: string
  pkey: string
  outTradeNo: string
  name: string
  money: number | null
  notifyUrl: string
  returnUrl: string
  device: string
  method: string
  param: string
  sitename: string
}

export interface EasyPayCreateResponse {
  code: number
  msg?: string
  trade_no?: string
  payurl?: string
  qrcode?: string
}

const SHIFT_AMOUNTS = [
  7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
  5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
  4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
  6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
]

// MD5 T[i] = floor(2^32 * abs(sin(i + 1))), per RFC 1321 section 3.4.
const TABLE = Array.from({ length: 64 }, (_, i) => Math.floor(Math.abs(Math.sin(i + 1)) * 0x100000000) >>> 0)
const textEncoder = new TextEncoder()

function rotateLeft(value: number, shift: number): number {
  return ((value << shift) | (value >>> (32 - shift))) >>> 0
}

function wordToHex(value: number): string {
  let out = ''
  for (let i = 0; i < 4; i += 1) {
    out += ((value >>> (i * 8)) & 0xff).toString(16).padStart(2, '0')
  }
  return out
}

export function md5Hex(input: string): string {
  const bytes = textEncoder.encode(input)
  const bitLength = bytes.length * 8
  const paddedLength = (((bytes.length + 9) + 63) >> 6) << 6
  const padded = new Uint8Array(paddedLength)
  padded.set(bytes)
  padded[bytes.length] = 0x80

  const view = new DataView(padded.buffer)
  view.setUint32(paddedLength - 8, bitLength >>> 0, true)
  view.setUint32(paddedLength - 4, Math.floor(bitLength / 0x100000000) >>> 0, true)

  let a0 = 0x67452301
  let b0 = 0xefcdab89
  let c0 = 0x98badcfe
  let d0 = 0x10325476

  for (let offset = 0; offset < paddedLength; offset += 64) {
    const words = new Array<number>(16)
    for (let i = 0; i < 16; i += 1) {
      words[i] = view.getUint32(offset + i * 4, true)
    }

    let a = a0
    let b = b0
    let c = c0
    let d = d0

    for (let i = 0; i < 64; i += 1) {
      let f: number
      let g: number
      if (i < 16) {
        f = (b & c) | (~b & d)
        g = i
      } else if (i < 32) {
        f = (d & b) | (~d & c)
        g = (5 * i + 1) % 16
      } else if (i < 48) {
        f = b ^ c ^ d
        g = (3 * i + 5) % 16
      } else {
        f = c ^ (b | ~d)
        g = (7 * i) % 16
      }

      const next = d
      d = c
      c = b
      b = (b + rotateLeft((a + f + TABLE[i] + words[g]) >>> 0, SHIFT_AMOUNTS[i])) >>> 0
      a = next
    }

    a0 = (a0 + a) >>> 0
    b0 = (b0 + b) >>> 0
    c0 = (c0 + c) >>> 0
    d0 = (d0 + d) >>> 0
  }

  return wordToHex(a0) + wordToHex(b0) + wordToHex(c0) + wordToHex(d0)
}

export function buildSignContent(params: Record<string, string>): string {
  return Object.keys(params)
    .filter((key) => key !== 'sign' && key !== 'sign_type' && params[key].trim() !== '')
    .sort()
    .map((key) => `${key}=${params[key]}`)
    .join('&')
}

export function signEasyPayMD5(params: Record<string, string>, pkey: string): string {
  return md5Hex(buildSignContent(params) + pkey)
}

function appendIfPresent(params: URLSearchParams, key: string, value: string) {
  const normalized = value.trim()
  if (normalized !== '') {
    params.set(key, normalized)
  }
}

function formatMoney(value: number | null): string {
  if (value === null || !Number.isFinite(value)) {
    throw new Error('订单金额必须大于 0.01')
  }
  const formatted = value.toFixed(2)
  if (Number(formatted) < 0.01) {
    throw new Error('订单金额必须大于 0.01')
  }
  return formatted
}

export function buildEasyPayDemoPayload(form: EasyPayDemoForm): URLSearchParams {
  const params = new URLSearchParams()
  appendIfPresent(params, 'pid', form.pid)
  params.set('type', 'alipay')
  appendIfPresent(params, 'out_trade_no', form.outTradeNo)
  appendIfPresent(params, 'notify_url', form.notifyUrl)
  appendIfPresent(params, 'return_url', form.returnUrl)
  appendIfPresent(params, 'name', form.name)
  params.set('money', formatMoney(form.money))
  appendIfPresent(params, 'device', form.device)
  appendIfPresent(params, 'method', form.method)
  appendIfPresent(params, 'param', form.param)
  appendIfPresent(params, 'sitename', form.sitename)
  params.set('sign_type', 'MD5')

  const raw = Object.fromEntries(params) as Record<string, string>
  // Trim paste-only whitespace at the demo boundary; protocol signing itself uses the raw pkey argument.
  params.set('sign', signEasyPayMD5(raw, form.pkey.trim()))
  return params
}

export function pickQrValue(response: Pick<EasyPayCreateResponse, 'qrcode' | 'payurl'>): string {
  return (response.qrcode || '').trim() || (response.payurl || '').trim()
}
