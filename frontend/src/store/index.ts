import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { api } from '../api/client'
import { isHubNode, isNodeSelectable, selectActiveNode } from '../utils/topologyStatus'
import type { NodeRecord, TopologySnapshot, UserInfo } from '../types'

interface LiveLog {
  node_id?: string
  source?: string
  level: string
  message: string
  timestamp: string
}

interface LiveMessage {
  type: 'topology' | 'log'
  topology?: TopologySnapshot
  log?: LiveLog
}

export const useAppStore = defineStore('app', () => {
  const savedTheme = localStorage.getItem('opennhrp_theme')
  const isDark = ref<boolean>(savedTheme ? savedTheme === 'dark' : true)
  const activeNodeId = ref<string>('')
  const lastLeaderNodeId = ref<string>('')
  const nodes = ref<NodeRecord[]>([])
  const loading = ref<boolean>(false)
  const topologySnapshot = ref<TopologySnapshot | null>(null)
  const liveLog = ref<LiveLog | null>(null)
  let liveSocket: WebSocket | null = null
  let liveNodeId = ''
  let liveIncludeHA = false
  let reconnectTimer: number | null = null

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
    disconnectLiveUpdates()
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

  const applyNodes = (list: NodeRecord[]) => {
    nodes.value = list

    const hubNodes = list.filter(isHubNode)
    const validNodes = hubNodes.length > 0 ? hubNodes : list
    const selection = selectActiveNode(validNodes, activeNodeId.value, lastLeaderNodeId.value)
    activeNodeId.value = selection.activeNodeId
    lastLeaderNodeId.value = selection.leaderNodeId
  }

  const fetchNodes = async () => {
    if (!token.value) return
    try {
      const list = await api.listNodes()
      applyNodes(list)
    } catch (e) {
      console.error('Failed to fetch nodes', e)
    }
  }

  const disconnectLiveUpdates = () => {
    if (reconnectTimer) window.clearTimeout(reconnectTimer)
    reconnectTimer = null
    liveNodeId = ''
    if (liveSocket) {
      liveSocket.onclose = null
      liveSocket.close()
      liveSocket = null
    }
  }

  const connectLiveUpdates = (includeHA = liveIncludeHA) => {
    if (!token.value || typeof window === 'undefined') return
    const nodeId = activeNodeId.value
    if (liveSocket && liveNodeId === nodeId && liveIncludeHA === includeHA && liveSocket.readyState < WebSocket.CLOSING) return
    liveIncludeHA = includeHA
    disconnectLiveUpdates()
    liveNodeId = nodeId
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const socket = new WebSocket(
      `${protocol}//${window.location.host}/api/topology/ws?node_id=${encodeURIComponent(nodeId)}&token=${encodeURIComponent(token.value)}&include_ha=${includeHA ? '1' : '0'}`
    )
    liveSocket = socket
    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data) as LiveMessage
        if (message.type === 'log' && message.log) {
          liveLog.value = message.log
          return
        }
        const snapshot = message.topology
        if (!snapshot || (snapshot.node_id && snapshot.node_id !== activeNodeId.value)) return
        topologySnapshot.value = snapshot
        applyNodes(snapshot.nodes || [])
      } catch (error) {
        console.error('Parse live update error', error)
      }
    }
    socket.onclose = () => {
      if (liveSocket !== socket) return
      liveSocket = null
      if (token.value) reconnectTimer = window.setTimeout(connectLiveUpdates, 3000)
    }
  }

  watch(activeNodeId, () => {
    if (liveSocket) connectLiveUpdates(liveIncludeHA)
  })

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
      } else if (n.role === 'follower') {
        roleText = 'Follower 服务备节点'
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
    topologySnapshot,
    liveLog,
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
    connectLiveUpdates,
    disconnectLiveUpdates,
  }
})
