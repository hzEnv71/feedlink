<template>
  <div class="ops-page">
    <section class="card ops-head">
      <div>
        <h2>运维面板</h2>
        <p>MQ 熔断/降级/重试实时指标</p>
      </div>
      <div class="actions">
        <el-switch v-model="autoRefresh" active-text="自动刷新" />
        <el-button :loading="loading" @click="loadMetrics">立即刷新</el-button>
      </div>
    </section>

    <section class="grid">
      <div class="card metric" v-for="item in metricCards" :key="item.key">
        <div class="label">{{ item.label }}</div>
        <div class="value">{{ item.value }}</div>
      </div>
    </section>

    <section class="card raw-wrap">
      <div class="raw-title">原始 JSON</div>
      <pre>{{ JSON.stringify(metrics, null, 2) }}</pre>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { opsApi } from '../api'

const loading = ref(false)
const metrics = ref({})
const autoRefresh = ref(true)
let timer = null

const metricCards = computed(() => [
  { key: 'circuit_open', label: '熔断状态(0/1)', value: metrics.value.circuit_open ?? 0 },
  { key: 'circuit_open_count', label: '熔断开启次数', value: metrics.value.circuit_open_count ?? 0 },
  { key: 'circuit_close_count', label: '熔断关闭次数', value: metrics.value.circuit_close_count ?? 0 },
  { key: 'degrade_count', label: '降级次数(outbox only)', value: metrics.value.degrade_count ?? 0 },
  { key: 'publish_fail_count', label: '发布失败次数', value: metrics.value.publish_fail_count ?? 0 },
  { key: 'retry_count', label: '重试次数', value: metrics.value.retry_count ?? 0 },
  { key: 'dlq_count', label: '死信次数', value: metrics.value.dlq_count ?? 0 },
  { key: 'dispatch_error_count', label: '分发错误次数', value: metrics.value.dispatch_error_count ?? 0 },
])

onMounted(async () => {
  await loadMetrics()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
})

watch(autoRefresh, (v) => {
  if (v) startAutoRefresh()
  else stopAutoRefresh()
})

function startAutoRefresh() {
  stopAutoRefresh()
  if (!autoRefresh.value) return
  timer = setInterval(loadMetrics, 5000)
}

function stopAutoRefresh() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

async function loadMetrics() {
  loading.value = true
  try {
    const res = await opsApi.getMQMetrics()
    metrics.value = res.data || {}
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.ops-page { min-width: 0; }
.ops-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  border-radius: 14px;
}
.ops-head h2 { margin: 0; }
.ops-head p { margin: 4px 0 0; color: #95a0b5; font-size: 13px; }
.actions { display: inline-flex; align-items: center; gap: 10px; }
.grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}
.metric { border-radius: 12px; }
.label { color: #8f99ad; font-size: 12px; }
.value { margin-top: 6px; font-size: 24px; font-weight: 700; }
.raw-wrap { margin-top: 12px; border-radius: 12px; }
.raw-title { font-weight: 600; margin-bottom: 6px; }
pre {
  margin: 0;
  background: #f6f8fb;
  border-radius: 8px;
  padding: 10px;
  overflow: auto;
}
@media (max-width: 1100px) {
  .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
