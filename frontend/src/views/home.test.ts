import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('public homepage', () => {
  it('makes user registration discoverable from /', () => {
    const router = readFileSync(new URL('../router/index.ts', import.meta.url), 'utf8')
    const home = readFileSync(new URL('./Home.vue', import.meta.url), 'utf8')

    expect(router).toContain("path: '/'")
    expect(router).toContain("name: 'Home'")
    expect(router).not.toContain("redirect: '/admin/dashboard'")
    expect(home).toContain('to="/user/register"')
    expect(home).toContain('用户注册')
    expect(home).toContain('to="/user/login"')
    expect(home).toContain('to="/admin/login"')
  })
})
