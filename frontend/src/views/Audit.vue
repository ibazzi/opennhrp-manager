<template>
  <div class="page-container audit-page">
    <div class="page-header">
      <div>
        <h2>审计日志</h2>
        <span class="sub-title">配置操作与审计日志</span>
      </div>
      <n-button secondary :loading="loading" @click="loadLogs">刷新</n-button>
    </div>

    <n-card title="配置操作与审计日志 (Audit Logs)" class="audit-card">
      <div class="audit-table-scroll">
        <n-table class="audit-table" :bordered="false" :single-line="true" size="small" style="min-width: 1000px;">
          <thead>
            <tr>
              <th style="width: 170px;">操作时间</th>
              <th style="width: 160px;">节点 ID</th>
              <th class="action-column" style="width: 220px;">操作类型</th>
              <th style="width: 100px;">执行人</th>
              <th class="allow-wrap">备注 / 详情</th>
              <th style="width: 90px;">结果</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && auditLogs.length === 0">
              <td colspan="6" class="text-center text-muted">正在加载审计日志...</td>
            </tr>
            <tr v-else-if="auditLogs.length === 0">
              <td colspan="6" class="text-center text-muted">暂无历史操作记录</td>
            </tr>
            <tr v-for="log in auditLogs" :key="log.id">
              <td>{{ new Date(log.created_at).toLocaleString() }}</td>
              <td><code>{{ log.node_id }}</code></td>
              <td><n-tag class="action-tag" size="tiny" type="info">{{ log.action }}</n-tag></td>
              <td>{{ log.operator }}</td>
              <td class="allow-wrap">{{ log.detail || '-' }}</td>
              <td>
                <n-tag size="tiny" :type="log.success ? 'success' : 'error'">
                  {{ log.success ? 'SUCCESS' : 'FAILED' }}
                </n-tag>
              </td>
            </tr>
          </tbody>
        </n-table>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NCard, NTable, NTag } from 'naive-ui'
import { api } from '../api/client'
import type { AuditLog } from '../types'

const auditLogs = ref<AuditLog[]>([])
const loading = ref(false)

const loadLogs = async () => {
  loading.value = true
  try {
    const result = await api.getAuditLogs(50)
    auditLogs.value = result.items
  } catch (error) {
    console.error('Failed to load audit logs', error)
  } finally {
    loading.value = false
  }
}

onMounted(loadLogs)
</script>

<style scoped>
.page-container {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  height: calc(100vh - 56px);
  min-height: 0;
  overflow: hidden;
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0;
  font-size: 20px;
  color: var(--text-title);
}

.sub-title {
  font-size: 13px;
  color: var(--text-muted);
}

.audit-card {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  flex-direction: column;
}

.audit-card :deep(.n-card-content) {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  flex-direction: column;
}

.audit-table-scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}

.audit-table :deep(th.action-column) {
  min-width: 220px;
  width: 220px !important;
}

.audit-table :deep(.action-tag) {
  box-sizing: border-box;
  height: auto;
  max-width: 100%;
  line-height: 1.4;
  white-space: normal;
  overflow-wrap: anywhere;
}

.audit-table :deep(.action-tag .n-tag__content) {
  white-space: normal;
  overflow-wrap: anywhere;
}

.text-center {
  text-align: center;
}

.text-muted {
  color: var(--text-muted);
}

@media (max-width: 768px) {
  .page-header {
    align-items: stretch;
    flex-direction: column;
    gap: 12px;
  }

  .page-header .n-button {
    width: 100%;
  }

  .page-container {
    padding: 10px 12px;
  }
}
</style>
