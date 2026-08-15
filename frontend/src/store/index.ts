import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client'
import { isHubNode, isNodeSelectable, selectActiveNode } from '../utils/topologyStatus'
import type { NodeRecord, UserInfo } from '../types'

export const useAppStore = defineStore('app', () => {
  const savedTheme = localStorage.getItem('opennhrp_theme')
  const isDark = ref<boolean>(savedTheme ? savedTheme === 'dark' : true)
  const activeNodeId = ref<string>('')
  const lastLeaderNodeId = ref<string>('')
  const nodes = ref<NodeRecord[]>([])
  const loading = ref<boolean>(false)

  // Auth State
  const token = ref<string>(localStorage.getItem('opennhrp_token') || '')
  const currentUser = ref<UserInfo | null>(
    localStorage.getItem('opennhrp_user') ? JSON.parse(localStorage.getItem('opennhrp_user')!) : null
  )

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => currentUser.value?.role === 'admin')

  const syncThemeClass = () => {
    if (typeof document !== 'undefined') {
      if (isDark.value) {
        document.documentElement.classList.add('dark')
        document.documentElement.classList.remove('light')
      } else {
        document.documentElement.classList.add('light')
        document.documentElement.classList.remove('dark')
      }
    }
  }

  // Initial sync
  syncThemeClass()

  const toggleTheme = () => {
    isDark.value = !isDark.value
    localStorage.setItem('opennhrp_theme', isDark.value ? 'dark' : 'light')
    syncThemeClass()
  }

  const setAuth = (tok: string, user: UserInfo) => {
    token.value = tok
    currentUser.value = user
    localStorage.setItem('opennhrp_token', tok)
    localStorage.setItem('opennhrp_user', JSON.stringify(user))
  }

  const clearAuth = () => {
    token.value = ''
    currentUser.value = null
    localStorage.removeItem('opennhrp_token')
    localStorage.removeItem('opennhrp_user')
  }

  const login = async (username: string, pass: string) => {
    const res = await api.login({ username, password: pass })
    setAuth(res.token, res.user)
    await fetchNodes()
    return res
  }

  const logout = async () => {
    try {
      await api.logout()
    } catch (e) {}
    clearAuth()
    if (typeof window !== 'undefined') {
      window.location.href = '/login'
    }
  }

  const checkAuth = async () => {
    if (!token.value) return null
    try {
      const user = await api.getMe()
      currentUser.value = user
      localStorage.setItem('opennhrp_user', JSON.stringify(user))
      return user
    } catch (e) {
      clearAuth()
      return null
    }
  }

  const fetchNodes = async () => {
    if (!token.value) return
    try {
      const list = await api.listNodes()
      nodes.value = list

      const hubNodes = list.filter(isHubNode)
      const validNodes = hubNodes.length > 0 ? hubNodes : list

      const selection = selectActiveNode(validNodes, activeNodeId.value, lastLeaderNodeId.value)
      activeNodeId.value = selection.activeNodeId
      lastLeaderNodeId.value = selection.leaderNodeId
    } catch (e) {
      console.error('Failed to fetch nodes', e)
    }
  }

  const nodeOptions = computed(() => {
    if (nodes.value.length === 0) {
      return [{ label: '加载节点中...', value: '' }]
    }
    const hubNodes = nodes.value.filter(isHubNode)
    const list = hubNodes.length > 0 ? hubNodes : nodes.value

    return list.map((n) => {
      let roleText = n.role || ''
      if (n.role === 'leader') {
        roleText = 'Leader 主节点'
      } else if (n.role === 'standby') {
        roleText = 'Standby 备节点'
      } else if (n.role === 'learner') {
        roleText = 'Learner 同步中'
      } else if (n.role === 'isolated') {
        roleText = 'Isolated 隔离态'
      } else if (n.role === 'witness' || n.id === 'local') {
        roleText = 'Witness 见证中心'
      }
      const displayName = n.name || n.id
      const statusText = n.status === 'online' ? '在线' : n.status === 'degraded' ? '降级' : 'Agent 离线'
      const label = `${displayName} (${roleText} · ${statusText})`
      return {
        label,
        value: n.id,
        disabled: !isNodeSelectable(n),
      }
    })
  })

  return {
    isDark,
    activeNodeId,
    nodes,
    nodeOptions,
    loading,
    token,
    currentUser,
    isLoggedIn,
    isAdmin,
    toggleTheme,
    setAuth,
    clearAuth,
    login,
    logout,
    checkAuth,
    fetchNodes,
  }
})
