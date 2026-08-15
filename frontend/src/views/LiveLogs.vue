<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>实时日志监控与流转终端</h2>
        <span class="sub-title">实时订阅各 Hub 系统日志中的 OpenNHRP 核心与 HA 协调器日志</span>
      </div>
    </div>
    <TerminalLog title="集群全局日志流终端 (WebSocket Active)" style="height: calc(100vh - 180px);" />
  </div>
</template>

<script setup lang="ts">
import TerminalLog from '../components/TerminalLog.vue'
</script>

<style scoped>
.page-container {
  padding: 20px;
}

.page-header {
  margin-bottom: 16px;
}

.page-header h2 {
  margin: 0 0 4px 0;
  font-size: 22px;
  font-weight: 700;
  color: var(--text-title);
}

.sub-title {
  font-size: 13px;
  color: var(--text-muted);
}
</style>
