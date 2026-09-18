<template>
  <div class="page-container">
    <h2>Spoke 管理</h2>
    <n-tabs :value="activeTab" type="line" @update:value="selectTab">
      <n-tab-pane name="records" tab="接入记录" display-directive="if">
        <Spokes />
      </n-tab-pane>
      <n-tab-pane name="managed" tab="纳管设备" display-directive="if">
        <ManagedSpokes :key="typeof route.query.node === 'string' ? route.query.node : ''" />
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NTabs, NTabPane } from 'naive-ui'
import Spokes from './Spokes.vue'
import ManagedSpokes from './ManagedSpokes.vue'

const route = useRoute()
const router = useRouter()
const activeTab = computed(() => route.query.tab === 'managed' ? 'managed' : 'records')
const selectTab = (tab: string) => router.push({
  path: '/spokes',
  query: { ...route.query, tab: tab === 'managed' ? 'managed' : undefined, node: undefined },
})
</script>

<style scoped>
.page-container { padding: 24px; }
h2 { margin: 0 0 16px; font-size: 20px; color: var(--text-title); }
@media (max-width: 768px) {
  .page-container { padding: 12px; }
}
</style>
