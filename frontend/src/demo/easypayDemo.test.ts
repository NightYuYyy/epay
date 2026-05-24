import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import viteConfig from '../../vite.config'
import { describe, expect, it } from 'vitest'
import {
  buildEasyPayDemoPayload,
  buildSignContent,
  md5Hex,
  pickQrValue,
  signEasyPayMD5,
} from './easypayDemo'

describe('standalone EasyPay demo page', () => {
  it('is a standalone HTML entry instead of a merchant portal route', () => {
    const html = readFileSync(new URL('../../demo.html', import.meta.url), 'utf8')
    const router = readFileSync(new URL('../router/index.ts', import.meta.url), 'utf8')
    const userLayout = readFileSync(new URL('../layouts/UserLayout.vue', import.meta.url), 'utf8')
    const demoMain = readFileSync(new URL('./main.ts', import.meta.url), 'utf8')

    expect(html).toContain('<form id="payment-form"')
    expect(html).toContain('id="qr-code"')
    expect(html).toContain('/src/demo/main.ts')
    expect(html).toContain('id="platform_notify_url"')
    expect(html).toContain('id="callback-events"')
    expect(html).toContain('/demo/notify')
    expect(html).toContain('/api/alipay/notify')
    expect(html).not.toContain('id="app"')
    expect(router).not.toContain('MerchantPaymentDemo')
    expect(router).not.toContain('/views/merchant/PaymentDemo.vue')
    expect(userLayout).not.toContain('/user/demo')
    expect(userLayout).not.toContain('支付 Demo')
    const proxy = (viteConfig as any).server?.proxy
    expect(proxy && typeof proxy).toBe('object')
    const proxyKeys = Object.keys(proxy)
    expect(proxyKeys).not.toContain('/demo')
    expect(proxyKeys).toContain('/demo/')
    expect(demoMain).not.toContain("params.get('pkey')")
    expect(demoMain).toContain("params.delete('pkey')")
    expect(demoMain).toContain('window.history.replaceState')
  })
})

describe('EasyPay standalone demo signing', () => {
  it('canonicalizes parameters using the EasyPay MD5 rules', () => {
    const content = buildSignContent({
      pid: '1001',
      type: 'alipay',
      out_trade_no: 'X1',
      money: '100.00',
      empty: '',
      blank: '  ',
      zero: '0',
      sign: 'ignored',
      sign_type: 'MD5',
    })

    expect(content).toBe('money=100.00&out_trade_no=X1&pid=1001&type=alipay&zero=0')
  })

  it('matches the documented EasyPay MD5 fixed vector and Node crypto for UTF-8', () => {
    expect(signEasyPayMD5({
      pid: '1001',
      out_trade_no: '20250101',
      name: 'hi',
      money: '100.00',
      sign_type: 'MD5',
    }, 'abc123')).toBe('7d485d80eaf0b05747315811496959cb')

    const input = 'name=测试商品&money=12.34abc123'
    expect(md5Hex(input)).toBe(createHash('md5').update(input, 'utf8').digest('hex'))
  })

  it('builds a signed alipay payload and omits blank optional fields', () => {
    const payload = buildEasyPayDemoPayload({
      pid: '1001',
      pkey: ' abc123 ',
      outTradeNo: 'DEMO-1',
      name: '测试订单',
      money: 12.3,
      notifyUrl: 'https://merchant.example/notify',
      returnUrl: 'https://merchant.example/return',
      device: 'pc',
      method: '',
      param: 'demo',
      sitename: '',
    })

    expect(Object.fromEntries(payload)).toMatchObject({
      pid: '1001',
      type: 'alipay',
      out_trade_no: 'DEMO-1',
      name: '测试订单',
      money: '12.30',
      notify_url: 'https://merchant.example/notify',
      return_url: 'https://merchant.example/return',
      device: 'pc',
      param: 'demo',
      sign_type: 'MD5',
    })
    expect(payload.has('method')).toBe(false)

    const raw = Object.fromEntries(payload) as Record<string, string>
    expect(raw.sign).toMatch(/^[0-9a-f]{32}$/)
    expect(raw.sign).toBe(signEasyPayMD5(raw, 'abc123'))
  })

  it('rejects null or sub-cent amounts before signing', () => {
    const base = {
      pid: '1001',
      pkey: 'abc123',
      outTradeNo: 'DEMO-1',
      name: '测试订单',
      notifyUrl: 'https://merchant.example/notify',
      returnUrl: 'https://merchant.example/return',
      device: 'pc',
      method: '',
      param: '',
      sitename: '',
    }

    expect(() => buildEasyPayDemoPayload({ ...base, money: 0.004 })).toThrow('订单金额必须大于 0.01')
    expect(() => buildEasyPayDemoPayload({ ...base, money: null })).toThrow('订单金额必须大于 0.01')
  })

  it('prefers qrcode over payurl and rejects empty responses', () => {
    expect(pickQrValue({ qrcode: ' https://qr.example ', payurl: 'https://pay.example' })).toBe('https://qr.example')
    expect(pickQrValue({ qrcode: '', payurl: 'https://pay.example' })).toBe('https://pay.example')
    expect(pickQrValue({ qrcode: ' ', payurl: ' ' })).toBe('')
  })
})
