<template>
  <div class="page-container">
    <h2>Spoke 管理</h2>
    <Spokes />
  </div>
</template>

<script setup lang="ts">
import Spokes from './Spokes.vue'
</script>

<style scoped>
.page-container { box-sizing: border-box; display: flex; flex-direction: column; height: calc(100vh - 56px); overflow: hidden; padding: 24px; }
h2 { margin: 0 0 16px; font-size: 20px; color: var(--text-title); }
@media (max-width: 768px) {
  .page-container { padding: 12px; }
}
</style>
