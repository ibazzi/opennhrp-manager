<template>
  <div class="terminal-container">
    <div class="terminal-header">
      <div class="terminal-dots">
        <span class="dot red"></span>
        <span class="dot yellow"></span>
        <span class="dot green"></span>
        <span class="terminal-title" :title="title || 'Live System Logs'">{{ title || 'Live System Logs' }}</span>
      </div>
      <div class="terminal-actions">
        <n-button
          size="tiny"
          :type="isWrap ? 'primary' : 'default'"
          secondary
          @click="toggleWrap"
        >
          {{ isWrap ? '已换行' : '不换行' }}
        </n-button>
        <n-button size="tiny" secondary @click="clearLogs">清屏</n-button>
        <n-button size="tiny" :type="isPaused ? 'warning' : 'default'" secondary @click="isPaused = !isPaused">
          {{ isPaused ? '恢复' : '暂停' }}
        </n-button>
      </div>
    </div>
    <div class="terminal-body-wrapper">
      <n-scrollbar ref="scrollbarRef" class="terminal-scrollbar" x-scrollable>
        <div class="terminal-content" :class="{ 'is-nowrap': !isWrap }">
          <div v-if="logs.length === 0" class="empty-tip">
            等待日志流接入 (WebSocket)...
          </div>
          <div
            v-for="(log, index) in logs"
            :key="index"
            class="log-line"
            :class="[log.level.toLowerCase(), { 'wrap-mode': isWrap, 'nowrap-mode': !isWrap }]"
          >
            <span class="log-meta">
              <span class="log-time">[{{ log.time }}]</span>
              <span class="log-source" v-if="log.source">[{{ log.source }}]</span>
              <span class="log-level" :class="log.level.toLowerCase()">{{ log.level }}</span>
            </span>
            <span class="log-message">{{ log.message }}</span>
          </div>
        </div>
      </n-scrollbar>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { NButton, NScrollbar } from 'naive-ui'

const props = defineProps<{
  title?: string
  nodeId?: string
}>()

interface LogItem {
  time: string
  source?: string
  level: string
  message: string
}

const logs = ref<LogItem[]>([])
const isPaused = ref(false)
const isWrap = ref(false)
const scrollbarRef = ref<any>(null)
let ws: WebSocket | null = null

const toggleWrap = () => {
  isWrap.value = !isWrap.value
  try {
    localStorage.setItem('terminal_log_wrap', isWrap.value ? 'true' : 'false')
  } catch (e) {}
}

const clearLogs = () => {
  logs.value = []
}

const connectWS = () => {
  const token = localStorage.getItem('opennhrp_token')
  if (!token) return

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}/api/logs/ws?token=${encodeURIComponent(token)}`

  ws = new WebSocket(wsUrl)

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      if (props.nodeId && data.node_id !== props.nodeId) return
      const item: LogItem = {
        time: new Date(data.timestamp || Date.now()).toLocaleTimeString(),
        source: [data.node_id, data.source].filter(Boolean).join('/') || 'system',
        level: data.level || 'INFO',
        message: data.message,
      }

      logs.value.push(item)
      if (logs.value.length > 500) {
        logs.value.shift()
      }

      if (!isPaused.value) {
        nextTick(() => {
          scrollbarRef.value?.scrollTo({ position: 'bottom', silent: true })
        })
      }
    } catch (e) {
      console.error('Parse log error', e)
    }
  }

  ws.onclose = () => {
    const currentToken = localStorage.getItem('opennhrp_token')
    if (currentToken) {
      setTimeout(connectWS, 3000)
    }
  }
}

onMounted(() => {
  const savedWrap = localStorage.getItem('terminal_log_wrap')
  if (savedWrap !== null) {
    isWrap.value = savedWrap === 'true'
  }
  connectWS()
})

onUnmounted(() => {
  if (ws) {
    ws.close()
    ws = null
  }
})
</script>

<style scoped>
.terminal-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 260px;
  background: var(--bg-card, #09090b);
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.1));
  border-radius: 8px;
  overflow: hidden;
  font-family: 'Fira Code', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  box-shadow: var(--card-shadow);
}

.terminal-header {
  background: var(--bg-card-secondary, #18181b);
  padding: 8px 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--border-color, rgba(255, 255, 255, 0.06));
}

.terminal-dots {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex: 1;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dot.red { background: #ef4444; }
.dot.yellow { background: #f59e0b; }
.dot.green { background: #10b981; }

.terminal-title {
  margin-left: 6px;
  font-size: 12px;
  color: var(--text-muted, #a1a1aa);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.terminal-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.terminal-body-wrapper {
  flex: 1;
  height: 0;
  min-height: 200px;
  overflow: hidden;
  background: var(--bg-body, #09090b);
}

.terminal-scrollbar {
  height: 100%;
  max-height: 100%;
}

.terminal-content {
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.6;
}

.terminal-content.is-nowrap {
  width: max-content;
  min-width: 100%;
}

.empty-tip {
  color: var(--text-muted, #52525b);
  text-align: center;
  padding: 40px;
}

/* Nowrap mode: clean single-line horizontal scroll */
.log-line.nowrap-mode {
  display: flex;
  align-items: baseline;
  gap: 8px;
  white-space: pre;
  margin-bottom: 4px;
}

.log-line.nowrap-mode .log-meta {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  flex-shrink: 0;
}

.log-line.nowrap-mode .log-message {
  white-space: pre;
  flex-shrink: 0;
}

/* Wrap mode: meta is inline, message flows smoothly using full width */
.log-line.wrap-mode {
  display: block;
  margin-bottom: 6px;
  word-break: break-word;
  overflow-wrap: break-word;
}

.log-line.wrap-mode .log-meta {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  margin-right: 8px;
  vertical-align: baseline;
}

.log-line.wrap-mode .log-message {
  display: inline;
  word-break: break-word;
  overflow-wrap: break-word;
}

.log-time {
  color: #71717a;
  font-size: 11.5px;
}

.log-source {
  color: #38bdf8;
  font-weight: 500;
  font-size: 11.5px;
}

.log-level {
  font-weight: 600;
  font-size: 11px;
  padding: 1px 5px;
  border-radius: 3px;
}

.log-level.info { color: #10b981; background: rgba(16, 185, 129, 0.1); }
.log-level.warn { color: #f59e0b; background: rgba(245, 158, 11, 0.1); }
.log-level.error { color: #ef4444; background: rgba(239, 68, 68, 0.15); }
.log-level.debug { color: #a855f7; background: rgba(168, 85, 247, 0.1); }

.log-message {
  color: var(--text-title, #f4f4f5);
}

@media (max-width: 640px) {
  .terminal-header {
    padding: 6px 10px;
    gap: 6px;
  }
  .terminal-title {
    font-size: 11px;
  }
  .terminal-actions {
    gap: 4px;
  }
  .terminal-actions :deep(.n-button) {
    padding: 0 5px;
    font-size: 11px;
  }
  .terminal-content {
    padding: 8px 10px;
    font-size: 11.5px;
    line-height: 1.5;
  }
}
</style>
