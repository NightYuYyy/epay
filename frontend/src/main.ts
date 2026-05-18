import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import './style.css'

const app = createApp(App)

// Swallow three harmless Naive UI cleanup races that fire during quick
// route changes. They originate from inside Naive UI's DataTable / Dropdown
// portal teardown when a parent VNode is already gone — no user-visible
// effect, but they pollute the console.
//
// Surfacing them once in dev mode is enough; in production we keep the
// console clean. Anything else still bubbles to console.error.
const NAIVE_RACES = [
  /e\.forEach is not a function/,                              // DataTable + Dropdown options watcher
  /Cannot destructure property 'type' of 'e' as it is null/,   // VNode flush after unmount
  /Cannot read properties of null \(reading 'parentNode'\)/,   // Portal node already detached
  /Cannot read properties of null \(reading 'nextSibling'\)/,  // Same family — DOM sibling lookup post-unmount
  /Cannot read properties of null \(reading 'previousSibling'\)/,
  /Cannot read properties of null \(reading 'children'\)/,
]

const seen = new Set<string>()
app.config.errorHandler = (err) => {
  const msg = err instanceof Error ? err.message : String(err)
  if (NAIVE_RACES.some((rx) => rx.test(msg))) {
    if (import.meta.env.DEV && !seen.has(msg)) {
      seen.add(msg)
      // eslint-disable-next-line no-console
      console.debug('[suppressed naive-ui cleanup race]', msg)
    }
    return
  }
  // eslint-disable-next-line no-console
  console.error(err)
}

// Also catch unhandled promise rejections — Vue 3 only routes synchronous
// errors through errorHandler.
window.addEventListener('unhandledrejection', (event) => {
  const msg = (event.reason && event.reason.message) || String(event.reason)
  if (NAIVE_RACES.some((rx) => rx.test(msg))) {
    event.preventDefault()
  }
})

app.use(createPinia())
app.use(router)
app.mount('#app')
