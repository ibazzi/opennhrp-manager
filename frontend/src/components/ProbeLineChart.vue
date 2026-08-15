<template>
  <div class="probe-chart-container">
    <div ref="chartRef" class="echarts-box"></div>
    <div v-if="!probes || probes.length === 0" class="empty-overlay">
      <span class="text-muted">暂无历史探针时序数据（所选时间范围内未记录到探测）</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, nextTick } from 'vue'
import * as echarts from 'echarts'
import { useAppStore } from '../store'
import type { ProbeRecord, NodeRecord } from '../types'

const props = withDefaults(
  defineProps<{
    probes?: ProbeRecord[]
    timeHours?: number
    selectedNode?: string
    probeLayer?: 'l3_nbma' | 'l4_port' | 'all'
    metricType?: 'all' | 'rtt' | 'loss'
    nodes?: NodeRecord[]
  }>(),
  {
    probes: () => [],
    timeHours: 24,
    selectedNode: 'all',
    probeLayer: 'l3_nbma',
    metricType: 'all',
    nodes: () => [],
  }
)

const store = useAppStore()
const chartRef = ref<HTMLDivElement | null>(null)
let chartInstance: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null
let resizeTimeout: ReturnType<typeof setTimeout> | null = null

// Palette for smooth aesthetic lines with glowing gradients (matching reference style)
const seriesColors = [
  { line: '#38bdf8', gradientFrom: 'rgba(56, 189, 248, 0.26)', gradientTo: 'rgba(56, 189, 248, 0.01)' }, // Sky Blue
  { line: '#10b981', gradientFrom: 'rgba(16, 185, 129, 0.26)', gradientTo: 'rgba(16, 185, 129, 0.01)' }, // Emerald
  { line: '#c084fc', gradientFrom: 'rgba(192, 132, 252, 0.26)', gradientTo: 'rgba(192, 132, 252, 0.01)' }, // Purple / Lilac
  { line: '#f43f5e', gradientFrom: 'rgba(244, 63, 94, 0.26)', gradientTo: 'rgba(244, 63, 94, 0.01)' }, // Rose Pink
  { line: '#06b6d4', gradientFrom: 'rgba(6, 182, 212, 0.26)', gradientTo: 'rgba(6, 182, 212, 0.01)' }, // Cyan
]

const lossColorSchemes = [
  { line: '#f59e0b', gradientFrom: 'rgba(245, 158, 11, 0.22)', gradientTo: 'rgba(245, 158, 11, 0.01)' }, // Amber
  { line: '#ef4444', gradientFrom: 'rgba(239, 68, 68, 0.22)', gradientTo: 'rgba(239, 68, 68, 0.01)' }, // Rose Red
  { line: '#f97316', gradientFrom: 'rgba(249, 115, 22, 0.22)', gradientTo: 'rgba(249, 115, 22, 0.01)' }, // Orange
  { line: '#ec4899', gradientFrom: 'rgba(236, 72, 153, 0.22)', gradientTo: 'rgba(236, 72, 153, 0.01)' }, // Pink
]

const getNodeName = (nodeId: string) => {
  const n = props.nodes?.find((item) => item.id === nodeId)
  return n?.name || nodeId
}

const getProbeTypeLabel = (type: string) => {
  if (type === 'l3_nbma') return 'L3 Ping'
  if (type === 'l4_port') return 'L4 端口'
  if (type === 'overlay_gre') return 'GRE 隧道'
  if (type === 'agent_telemetry') return 'Agent 遥测'
  return type
}

const renderChart = () => {
  if (!chartRef.value) return

  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
  }

  if (!props.probes || props.probes.length === 0) {
    chartInstance.clear()
    return
  }

  const containerWidth = chartRef.value.clientWidth || 800
  const isMobile = containerWidth < 640

  const isDark = store.isDark
  const textColor = isDark ? '#94a3b8' : '#64748b'
  const titleColor = isDark ? '#f4f4f5' : '#0f172a'
  const splitLineColor = isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)'
  const tooltipBg = isDark ? 'rgba(24, 24, 27, 0.94)' : 'rgba(255, 255, 255, 0.96)'
  const tooltipBorder = isDark ? 'rgba(255, 255, 255, 0.12)' : 'rgba(0, 0, 0, 0.08)'

  const now = Date.now()
  const hours = props.timeHours || 24
  const minTime = now - hours * 3600 * 1000
  const maxTime = now

  // 1. Filter by selected node
  let filtered = props.probes.filter((p) => p && p.target_node_id && p.recorded_at)
  if (props.selectedNode && props.selectedNode !== 'all') {
    filtered = filtered.filter((p) => p.target_node_id === props.selectedNode)
  }

  // 2. Filter by probe layer (l3_nbma / l4_port / all)
  const activeLayer = props.probeLayer || 'l3_nbma'
  if (activeLayer !== 'all') {
    filtered = filtered.filter((p) => p.probe_type === activeLayer)
  }

  // 3. Group by unique key [target_node_id + probe_type]
  const seriesGroups = new Map<string, { nodeId: string; probeType: string; items: ProbeRecord[] }>()

  filtered.forEach((p) => {
    const key = activeLayer === 'all' ? `${p.target_node_id}__${p.probe_type}` : p.target_node_id
    if (!seriesGroups.has(key)) {
      seriesGroups.set(key, {
        nodeId: p.target_node_id,
        probeType: p.probe_type,
        items: [],
      })
    }
    seriesGroups.get(key)!.items.push(p)
  })

  const series: any[] = []
  const showRTT = !props.metricType || props.metricType === 'all' || props.metricType === 'rtt'
  const showLoss = !props.metricType || props.metricType === 'all' || props.metricType === 'loss'

  // Determine dynamic yAxisIndex for loss series
  const lossAxisIndex = showRTT ? 1 : 0

  let colorIdx = 0
  seriesGroups.forEach((group) => {
    const nodeName = getNodeName(group.nodeId)
    const layerSuffix = activeLayer === 'all' ? ` (${getProbeTypeLabel(group.probeType)})` : ''
    const displayName = `${nodeName}${layerSuffix}`

    const colorScheme = seriesColors[colorIdx % seriesColors.length]
    const lossScheme = lossColorSchemes[colorIdx % lossColorSchemes.length]
    colorIdx++

    const points = group.items
      .map((point) => ({ ...point, timeMs: new Date(point.recorded_at).getTime() }))
      .filter((point) => Number.isFinite(point.timeMs))
      .sort((a, b) => a.timeMs - b.timeMs)
    const rttData = points.map((point) => [point.timeMs, point.rtt_ms > 0 ? point.rtt_ms : null])
    const lossData = points.map((point) => [point.timeMs, Number(((!point.success ? 1 : point.loss_rate || 0) * 100).toFixed(1))])

    if (showRTT) {
      series.push({
        name: `${displayName} · 延迟`,
        type: 'line',
        smooth: 0.38,
        sampling: 'lttb',
        connectNulls: false,
        showSymbol: false,
        symbol: 'circle',
        symbolSize: 6,
        yAxisIndex: 0,
        data: rttData,
        itemStyle: {
          color: colorScheme.line,
        },
        lineStyle: {
          width: 2.2,
          type: 'solid',
          color: colorScheme.line,
          shadowColor: isDark ? 'rgba(0,0,0,0.3)' : 'rgba(0,0,0,0.06)',
          shadowBlur: 3,
        },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: colorScheme.gradientFrom },
            { offset: 1, color: colorScheme.gradientTo },
          ]),
        },
      })
    }

    if (showLoss) {
      series.push({
        name: `${displayName} · 丢包率`,
        type: 'line',
        smooth: 0.38,
        sampling: 'lttb',
        connectNulls: true,
        showSymbol: false,
        symbol: 'circle',
        symbolSize: 6,
        yAxisIndex: lossAxisIndex,
        data: lossData,
        itemStyle: {
          color: lossScheme.line,
        },
        lineStyle: {
          width: 2.2,
          type: 'solid',
          color: lossScheme.line,
          shadowColor: isDark ? 'rgba(0,0,0,0.3)' : 'rgba(0,0,0,0.06)',
          shadowBlur: 3,
        },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: lossScheme.gradientFrom },
            { offset: 1, color: lossScheme.gradientTo },
          ]),
        },
      })
    }
  })

  const yAxes: any[] = []
  if (showRTT) {
    yAxes.push({
      type: 'value',
      name: '延迟 (ms)',
      position: 'left',
      nameLocation: 'end',
      nameGap: isMobile ? 8 : 10,
      nameTextStyle: {
        color: textColor,
        fontSize: isMobile ? 10 : 11,
        align: 'left',
        padding: [0, 0, 0, 0],
      },
      axisLabel: {
        color: textColor,
        fontSize: isMobile ? 10 : 11,
        formatter: isMobile ? '{value}' : '{value} ms',
      },
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: {
        lineStyle: {
          color: splitLineColor,
          type: 'dashed',
        },
      },
    })
  }

  if (showLoss) {
    yAxes.push({
      type: 'value',
      name: '丢包率 (%)',
      position: showRTT ? 'right' : 'left',
      nameLocation: 'end',
      nameGap: isMobile ? 8 : 10,
      min: 0,
      max: 100,
      nameTextStyle: {
        color: textColor,
        fontSize: isMobile ? 10 : 11,
        align: 'right',
        padding: [0, 0, 0, 0],
      },
      axisLabel: {
        color: textColor,
        fontSize: isMobile ? 10 : 11,
        formatter: '{value}%',
      },
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: {
        show: !showRTT,
        lineStyle: {
          color: splitLineColor,
          type: 'dashed',
        },
      },
    })
  }

  const option: echarts.EChartsOption = {
    animation: true,
    animationDuration: 300,
    animationEasing: 'cubicOut',
    animationDurationUpdate: 280,
    animationEasingUpdate: 'cubicOut',
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      padding: isMobile ? [8, 10] : [10, 14],
      textStyle: {
        color: titleColor,
        fontSize: isMobile ? 11 : 12,
      },
      axisPointer: {
        type: 'line',
        lineStyle: {
          color: isDark ? 'rgba(255,255,255,0.2)' : 'rgba(0,0,0,0.2)',
          type: 'dashed',
        },
      },
      formatter: (params: any) => {
        if (!Array.isArray(params) || params.length === 0) return ''
        const firstItem = params[0]
        const timeVal = Array.isArray(firstItem?.value) ? firstItem.value[0] : null
        const d = timeVal ? new Date(timeVal) : null
        const dateStr =
          d && !isNaN(d.getTime())
            ? `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
            : ''
        let html = `<div style="font-weight:700;margin-bottom:6px;border-bottom:1px solid ${splitLineColor};padding-bottom:4px;">${dateStr}</div>`
        params.forEach((item: any) => {
          if (item && item.value && Array.isArray(item.value) && item.value.length >= 2) {
            const val = item.value[1]
            const isLoss = item.seriesName && item.seriesName.includes('丢包率')
            if (val !== null && val !== undefined && !isNaN(val)) {
              const unit = isLoss ? '%' : ' ms'
              const numVal = Number(val)
              html += `<div style="display:flex;justify-content:space-between;gap:16px;margin:3px 0;font-size:${isMobile ? 11 : 12}px;">
                <span>${item.marker || ''} ${item.seriesName || ''}</span>
                <strong style="color:${isLoss && numVal > 0 ? '#ef4444' : titleColor};font-family:monospace;">${numVal.toFixed(isLoss ? 0 : 2)}${unit}</strong>
              </div>`
            } else if (!isLoss) {
              html += `<div style="display:flex;justify-content:space-between;gap:16px;margin:3px 0;font-size:${isMobile ? 11 : 12}px;">
                <span>${item.marker || ''} ${item.seriesName || ''}</span>
                <strong style="color:#ef4444;font-family:monospace;">服务断开 (无响应)</strong>
              </div>`
            }
          }
        })
        return html
      },
    },
    legend: {
      type: 'scroll',
      top: isMobile ? 2 : 4,
      left: isMobile ? 6 : 'center',
      right: isMobile ? 6 : 'auto',
      textStyle: {
        color: textColor,
        fontSize: isMobile ? 10 : 11,
      },
      icon: 'roundRect',
      itemWidth: isMobile ? 10 : 12,
      itemHeight: isMobile ? 6 : 8,
      itemGap: isMobile ? 8 : 12,
      pageTextStyle: {
        color: textColor,
      },
      pageIconColor: isDark ? '#38bdf8' : '#0284c7',
      pageIconInactiveColor: isDark ? '#4b5563' : '#cbd5e1',
      pageButtonItemGap: 2,
      pageButtonGap: 4,
      pageButtonPosition: 'end',
    },
    grid: {
      left: isMobile ? 6 : '3%',
      right: isMobile ? (showLoss && showRTT ? 8 : 6) : (showLoss && showRTT ? '4%' : '3%'),
      top: isMobile ? 64 : 52,
      bottom: isMobile ? 48 : 38,
      containLabel: true,
    },
    dataZoom: [
      {
        type: 'inside',
        start: 0,
        end: 100,
      },
      {
        type: 'slider',
        show: true,
        height: isMobile ? 14 : 16,
        bottom: 4,
        borderColor: 'transparent',
        backgroundColor: isDark ? 'rgba(255, 255, 255, 0.04)' : 'rgba(0, 0, 0, 0.03)',
        fillerColor: isDark ? 'rgba(56, 189, 248, 0.22)' : 'rgba(56, 189, 248, 0.15)',
        showDetail: false,
        brushSelect: false,
        handleSize: '100%',
        handleStyle: {
          color: isDark ? '#38bdf8' : '#0284c7',
          borderColor: 'transparent',
        },
        textStyle: {
          color: textColor,
          fontSize: 10,
        },
      },
    ],
    xAxis: {
      type: 'time',
      min: minTime,
      max: maxTime,
      splitNumber: isMobile ? 3 : 5,
      boundaryGap: ['0%', '0%'],
      axisLine: {
        lineStyle: { color: splitLineColor },
      },
      axisTick: { show: false },
      axisLabel: {
        color: textColor,
        fontSize: isMobile ? 9 : 10,
        fontFamily: "'Fira Code', monospace",
        hideOverlap: true,
        showMinLabel: true,
        showMaxLabel: true,
        formatter: (val: number) => {
          const d = new Date(val)
          if (isNaN(d.getTime())) return ''
          const h = String(d.getHours()).padStart(2, '0')
          const m = String(d.getMinutes()).padStart(2, '0')
          if (hours >= 24) {
            const mo = String(d.getMonth() + 1).padStart(2, '0')
            const day = String(d.getDate()).padStart(2, '0')
            return isMobile ? `${mo}/${day}\n${h}:${m}` : `${mo}/${day} ${h}:${m}`
          }
          return `${h}:${m}`
        },
      },
      splitLine: {
        show: false,
      },
    },
    yAxis: yAxes as any,
    series: series as any,
  }

  chartInstance.setOption(option, true)
}

const handleResize = () => {
  if (resizeTimeout) clearTimeout(resizeTimeout)
  resizeTimeout = setTimeout(() => {
    if (chartInstance) {
      chartInstance.resize()
      renderChart()
    }
  }, 100)
}

onMounted(() => {
  nextTick(() => {
    if (chartRef.value && typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(handleResize)
      resizeObserver.observe(chartRef.value)
    }
    renderChart()
  })
})

onUnmounted(() => {
  if (resizeTimeout) {
    clearTimeout(resizeTimeout)
  }
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
})

watch(
  () => [props.probes, props.timeHours, props.selectedNode, props.probeLayer, props.metricType, store.isDark],
  () => {
    renderChart()
  }
)
</script>

<style scoped>
.probe-chart-container {
  width: 100%;
  height: 380px;
  position: relative;
}

.echarts-box {
  width: 100%;
  height: 100%;
}

.empty-overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  background: transparent;
  pointer-events: none;
}

@media (max-width: 768px) {
  .probe-chart-container {
    height: 350px;
  }
}
</style>
