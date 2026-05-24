import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/api/client'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const user = ref(JSON.parse(localStorage.getItem('user') || 'null'))

  function setAuth(t: string, u: any) {
    token.value = t
    user.value = u
    localStorage.setItem('token', t)
    localStorage.setItem('user', JSON.stringify(u))
  }

  async function login(username: string, password: string, isAdmin = true) {
    const url = isAdmin ? '/api/admin/login' : '/api/merchant/login'
	const payload = isAdmin ? { username, password } : { pid: username, password }
	const { data } = await api.post(url, payload)
    if (data.code === 0) {
      setAuth(data.data.token, data.data)
    }
    return data
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  return { token, user, setAuth, login, logout }
})
