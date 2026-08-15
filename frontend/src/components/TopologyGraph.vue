<template>
  <div class="topology-container" ref="containerRef">
    <div class="canvas-wrapper">
      <!-- Overlay legend inside topology graph canvas (bottom-left vertical) -->
      <div class="legend-overlay">
        <div class="legend-items">
          <span class="legend-badge leader"><span class="dot"></span> 主 Hub</span>
          <span class="legend-badge standby"><span class="dot"></span> 备 Hub</span>
          <span class="legend-badge witness"><span class="dot"></span> Witness 仲裁</span>
          <span class="legend-badge spoke"><span class="dot"></span> Spoke 客户端 ({{ spokeList.length }})</span>
          <span class="legend-badge disabled"><span class="dot"></span> Hub 已禁用</span>
          <span class="legend-badge agent-offline"><span class="dot"></span> Manager Agent 离线</span>
          <span class="legend-badge ha-warning"><span class="dot"></span> HA 状态待确认</span>
          <span class="legend-badge offline"><span class="dot"></span> 节点离线</span>
        </div>
      </div>

      <svg
        :viewBox="computedViewBox"
        preserveAspectRatio="xMidYMid meet"
        class="topology-svg"
      >
        <!-- Background Grid & Defs -->
        <defs>
          <pattern id="topo-grid" width="28" height="28" patternUnits="userSpaceOnUse">
            <path d="M 28 0 L 0 0 0 28" fill="none" :stroke="store.isDark ? 'rgba(255,255,255,0.03)' : 'rgba(0,0,0,0.04)'" stroke-width="1" />
          </pattern>
        </defs>

        <rect width="100%" height="100%" fill="url(#topo-grid)" />

        <!-- Links Layer -->
        <g class="links">
          <!-- 1. Straight & Curved Links with stable key tracking -->
          <template v-for="(link, i) in links" :key="link.id || ('link-' + i)">
            <!-- Curved path for Spoke & Witness links -->
            <path
              v-if="link.isCurved"
              :d="link.pathD"
              :stroke="link.color || '#a855f7'"
              :stroke-width="link.width || 1.8"
              :stroke-dasharray="link.dashed ? '6,5' : 'none'"
              stroke-linecap="round"
              fill="none"
              class="topology-path"
            />
            <!-- Straight line for HA & Witness links -->
            <line
              v-else
              :x1="link.source.x"
              :y1="link.source.y"
              :x2="link.target.x"
              :y2="link.target.y"
              :stroke="link.color || '#18a058'"
              :stroke-dasharray="link.dashed ? '6,5' : 'none'"
              :stroke-width="link.width || 2"
              stroke-linecap="round"
              class="topology-line"
            />
          </template>

          <!-- 2. Link Label Pills -->
          <g
            v-for="(lbl, i) in linkLabels"
            :key="lbl.id || ('lbl-' + i)"
            :style="{ transform: `translate(${lbl.x}px, ${lbl.y}px)` }"
            class="link-label-group"
          >
            <rect
              :x="-lbl.width / 2"
              :y="-11"
              :width="lbl.width"
              height="22"
              rx="11"
              :fill="lbl.bg || (store.isDark ? '#18181b' : '#ffffff')"
              :stroke="lbl.border || (store.isDark ? '#27272a' : '#cbd5e1')"
              stroke-width="1"
            />
            <text
              y="4"
              :fill="lbl.textColor || (store.isDark ? '#d4d4d8' : '#334155')"
              font-size="11"
              font-weight="500"
              text-anchor="middle"
            >
              {{ lbl.text }}
            </text>
          </g>
        </g>

        <!-- Nodes Layer -->
        <g class="nodes">
          <g
            v-for="node in nodes"
            :key="node.id"
            :style="{ transform: `translate(${node.x}px, ${node.y}px)` }"
            class="node-group"
            :class="{
              clickable: (node.type === 'leader' || node.type === 'standby') && node.isSelectable,
              'is-unselectable': (node.type === 'leader' || node.type === 'standby') && !node.isSelectable,
              'is-offline': node.isOffline,
              'is-disabled': node.isDisabled,
              'is-agent-offline': node.isAgentOffline
            }"
            @mouseenter="hoveredNode = node"
            @mouseleave="hoveredNode = null"
            @click="handleNodeClick(node)"
          >
            <!-- Glowing active selection ring for currently inspected hub node -->
            <circle
              v-if="(node.type === 'leader' || node.type === 'standby') && node.id === (props.activeNodeId || store.activeNodeId)"
              :r="node.radius + 8"
              fill="none"
              stroke="#06b6d4"
              stroke-width="2.5"
              stroke-dasharray="5,3"
              class="active-selected-ring"
            />

            <!-- Pulsing outer ring for leader -->
            <circle
              v-if="node.type === 'leader' && !node.isOffline"
              r="38"
              :fill="store.isDark ? 'rgba(16, 185, 129, 0.15)' : 'rgba(16, 185, 129, 0.2)'"
              class="pulse-ring"
            />

            <!-- Outer selection ring if hovered -->
            <circle
              v-if="hoveredNode?.id === node.id"
              :r="node.radius + 6"
              fill="none"
              :stroke="node.stroke"
              stroke-width="2"
              stroke-dasharray="4,3"
              class="hover-ring"
            />

            <!-- Main Node Circle -->
            <circle
              :r="node.radius"
              :fill="node.bg"
              :stroke="node.stroke"
              :stroke-dasharray="node.isDisabled ? '5,4' : 'none'"
              stroke-width="2.5"
              class="main-node-circle"
            />

            <!-- Node Icon / Symbol inside Circle -->
            <component
              v-if="node.icon"
              :is="node.icon"
              :x="-node.radius / 2"
              :y="-node.radius / 2"
              :width="node.radius"
              :height="node.radius"
              :style="{ color: node.symbolColor || (store.isDark ? '#ffffff' : '#1e293b') }"
            />
            <text
              v-else
              y="5"
              :fill="node.symbolColor || (store.isDark ? '#ffffff' : '#1e293b')"
              :font-size="node.type === 'spoke' ? 12 : 14"
              font-weight="800"
              text-anchor="middle"
            >
              {{ node.symbol }}
            </text>

            <!-- Top Lease Badge (for Spoke nodes) -->
            <g v-if="node.type === 'spoke' && node.leaseText" :transform="`translate(0, -${node.radius + 12})`">
              <rect
                :x="-(node.leaseWidth || 60) / 2"
                y="-9"
                :width="node.leaseWidth || 60"
                height="18"
                rx="9"
                :fill="store.isDark ? '#18181b' : '#faf5ff'"
                :stroke="store.isDark ? '#6b21a8' : '#c084fc'"
                stroke-width="1"
              />
              <text
                y="4"
                :fill="store.isDark ? '#c084fc' : '#7e22ce'"
                font-size="10"
                font-weight="600"
                text-anchor="middle"
                font-family="'Fira Code', monospace"
              >
                {{ node.leaseText }}
              </text>
            </g>

            <!-- Text Labels: Witness Above, other nodes Underneath -->
            <!-- 1. Primary Title (Member Name / IP) -->
            <text
              :y="node.type === 'witness' ? -(node.radius + 18) : (node.radius + 18)"
              :fill="store.isDark ? '#f4f4f5' : '#0f172a'"
              :font-size="node.type === 'spoke' ? 11 : 13"
              font-weight="700"
              font-family="'Fira Code', monospace"
              text-anchor="middle"
            >
              {{ node.title }}
            </text>

            <!-- 2. Subtitle 1 (IP / Role text / SLA) -->
            <text
              v-if="node.subtitle1"
              :y="node.type === 'witness' ? -(node.radius + 5) : (node.radius + 32)"
              :fill="node.sub1Color || (store.isDark ? '#94a3b8' : '#475569')"
              :font-size="node.type === 'spoke' ? 10 : 11"
              font-weight="500"
              font-family="'Fira Code', monospace"
              text-anchor="middle"
            >
              {{ node.subtitle1 }}
            </text>

            <!-- 3. Subtitle 2 (Role Priority / Alias / Additional Info) -->
            <text
              v-if="node.subtitle2 && node.type !== 'witness'"
              :y="node.radius + 45"
              :fill="node.sub2Color || (store.isDark ? '#71717a' : '#64748b')"
              font-size="10"
              font-weight="500"
              text-anchor="middle"
            >
              {{ node.subtitle2 }}
            </text>
          </g>
        </g>
      </svg>
    </div>

    <!-- Floating Node Details Card on Hover -->
    <div v-if="hoveredNode" class="node-tooltip">
      <div class="tooltip-header">
        <span class="tooltip-badge" :style="{ background: hoveredNode.stroke }"></span>
        <span class="tooltip-title">{{ hoveredNode.title }}</span>
        <span class="tooltip-type">{{ hoveredNode.type.toUpperCase() }}</span>
      </div>
      <div class="tooltip-body">
        <div v-if="hoveredNode.subtitle1" class="tooltip-row">
          <span class="k">网络地址:</span>
          <span class="v code">{{ hoveredNode.subtitle1 }}</span>
        </div>
        <div v-if="hoveredNode.subtitle2" class="tooltip-row">
          <span class="k">节点描述:</span>
          <span class="v">{{ hoveredNode.subtitle2 }}</span>
        </div>
        <div v-if="hoveredNode.leaseText" class="tooltip-row">
          <span class="k">租约剩余:</span>
          <span class="v text-purple">{{ hoveredNode.leaseText }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, type Component } from 'vue'
import { EyeOutline } from '@vicons/ionicons5'
import { useAppStore } from '../store'
import { sortSpokesByIP } from '../utils/ip'
import {
  classifyHALink,
  classifyHubStatus,
  findLatestLeaderNode,
  formatHubStatus,
  formatWitnessQuorumStatus,
  isWitnessQuorumHealthy,
  isNodeSelectable,
  splitBalanced,
  type HubTopologyStatus,
} from '../utils/topologyStatus'
import type { ClusterStatus, MemberInfo, SpokeInfo, SLAMatrixItem, WitnessQuorumStatus } from '../types'

const props = defineProps<{
  activeNodeId?: string
  clusterStatus?: ClusterStatus | null
  witnessQuorum?: WitnessQuorumStatus | null
  spokes?: SpokeInfo[]
  slaMatrix?: SLAMatrixItem[]
}>()

const emit = defineEmits(['select-node'])

const store = useAppStore()
const hoveredNode = ref<GraphNode | null>(null)
const isMobile = ref(typeof window !== 'undefined' ? window.innerWidth <= 768 : false)

// ==========================================
// Centralized Topology Color System & Theme Tokens
// ==========================================
const TOPO_THEME = {
  leader: {
    stroke: '#10b981',
    bg: { dark: '#064e3b', light: '#dcfce7' },
    sub1: { dark: '#34d399', light: '#047857' },
    sub2: { dark: '#10b981', light: '#065f46' },
    symbol: { dark: '#6ee7b7', light: '#065f46' },
    glow: { dark: 'rgba(16, 185, 129, 0.15)', light: 'rgba(16, 185, 129, 0.2)' },
  },
  standby: {
    stroke: { dark: '#64748b', light: '#94a3b8' },
    bg: { dark: '#1e293b', light: '#f1f5f9' },
    sub1: { dark: '#94a3b8', light: '#475569' },
    sub2: { dark: '#a1a1aa', light: '#64748b' },
    symbol: { dark: '#e2e8f0', light: '#1e293b' },
  },
  witness: {
    stroke: '#f59e0b',
    bg: { dark: '#27272a', light: '#fffbeb' },
    healthyText: { dark: '#10b981', light: '#059669' },
    warningText: '#ef4444',
    sub2: { dark: '#71717a', light: '#64748b' },
  },
  offline: {
    stroke: '#ef4444',
    bg: { dark: '#27272a', light: '#fef2f2' },
    sub1: { dark: '#ef4444', light: '#dc2626' },
    sub2: '#ef4444',
    symbol: '#ef4444',
  },
  disabled: {
    stroke: '#71717a',
    bg: { dark: '#18181b', light: '#e4e4e7' },
    sub1: { dark: '#a1a1aa', light: '#52525b' },
    sub2: '#a1a1aa',
    symbol: '#a1a1aa',
  },
  agentOffline: {
    stroke: '#f97316',
    sub1: '#fb923c',
    sub2: '#fb923c',
    symbol: '#fb923c',
  },
  spoke: {
    stroke: '#a855f7',
    bg: { dark: '#18181b', light: '#faf5ff' },
    sub1: { dark: '#94a3b8', light: '#64748b' },
    sub2: { dark: '#71717a', light: '#94a3b8' },
    alias: { dark: '#c084fc', light: '#7e22ce' },
    symbol: { dark: '#ffffff', light: '#6b21a8' },
    badgeBg: { dark: '#18181b', light: '#faf5ff' },
    badgeStroke: { dark: '#6b21a8', light: '#c084fc' },
    badgeText: { dark: '#c084fc', light: '#7e22ce' },
  },
  link: {
    haOnline: '#3b82f6',
    haOffline: '#ef4444',
    haUnknown: '#f59e0b',
    disabled: '#71717a',
    probe: '#f59e0b',
    probeOffline: '#ef4444',
    spoke: '#a855f7',
    defaultStraight: '#18a058',
  },
  label: {
    bg: { dark: '#18181b', light: '#ffffff' },
    border: { dark: '#27272a', light: '#cbd5e1' },
    text: { dark: '#d4d4d8', light: '#334155' },
    offlineBg: { dark: '#3f1515', light: '#fef2f2' },
    offlineBorder: '#ef4444',
    offlineText: '#ef4444',
  },
  general: {
    title: { dark: '#f4f4f5', light: '#0f172a' },
    sub1Default: { dark: '#94a3b8', light: '#475569' },
    sub2Default: { dark: '#71717a', light: '#64748b' },
    symbolDefault: { dark: '#ffffff', light: '#1e293b' },
    activeSelectionRing: '#06b6d4',
  },
} as const

// ==========================================
// Centralized Layout Geometry & Coordinate Constants
// ==========================================
const TOPO_LAYOUT = {
  viewBox: {
    desktop: '0 0 1000 500',
    mobileWidth: 460,
    mobileBaseHeight: 490,
    mobileRowStep: 125,
    mobileEmptyHeight: 380,
  },
  witness: {
    y: { desktop: 70, mobile: 65 },
    radius: { desktop: 28, mobile: 26 },
  },
  hub: {
    y: { desktop: 205, mobile: 190 },
    radius: { desktop: 32, mobile: 28 },
    leftX: { desktop: 240, mobile: 95 },
    rightX: { desktop: 760, mobile: 365 },
    twoHubSpacing: { desktop: 260, mobile: 140 },
    columnSpreadY: 180,
    columnOffsets: {
      single: [0],
      double: [-75, 75],
      triple: [-80, 0, 80],
    },
    portPadding: 2,
  },
  bezier: {
    haLeaderControlOffset: 70,
    haTargetControlOffset: 55,
    haLineWidth: 2.2,
    probeDxRatio: 0.38,
    probeDyRatio: 0.35,
    probeMinVerticalDip: 35,
    probeEntryControlOffset: 55,
    probeLineWidth: 1.8,
    spokeOriginClearanceY: 56,
    spokeControlOffsetY: 12,
    spokeEntryPadding: 14,
  },
  spoke: {
    desktop: {
      y: 415,
      radius: 20,
      availableWidth: 840,
      maxStep: 130,
      linkWidth: 1.6,
    },
    mobile: {
      startY: 360,
      rowStep: 125,
      radius: 17,
      availableWidth: 370,
      linkWidth: 1.5,
    },
  },
} as const

interface GraphNode {
  id: string
  title: string
  subtitle1?: string
  sub1Color?: string
  subtitle2?: string
  sub2Color?: string
  type: 'leader' | 'standby' | 'witness' | 'spoke'
  symbol: string
  icon?: Component
  symbolColor?: string
  x: number
  y: number
  radius: number
  bg: string
  stroke: string
  leaseText?: string
  leaseWidth?: number
  isOffline?: boolean
  isDisabled?: boolean
  isAgentOffline?: boolean
  isSelectable?: boolean
  memberId?: string
  hubStatus?: HubTopologyStatus
}

interface GraphLink {
  id?: string
  source: { x: number; y: number }
  target: { x: number; y: number }
  color: string
  dashed?: boolean
  width?: number
  isCurved?: boolean
  pathD?: string
}

interface GraphLinkLabel {
  id?: string
  x: number
  y: number
  text: string
  width: number
  bg?: string
  border?: string
  textColor?: string
}

const nodes = ref<GraphNode[]>([])
const links = ref<GraphLink[]>([])
const linkLabels = ref<GraphLinkLabel[]>([])

const spokeList = computed(() => {
  return sortSpokesByIP((props.spokes || []).filter((s) => s.type !== 'local'))
})

const computedViewBox = computed(() => {
  if (!isMobile.value) {
    return TOPO_LAYOUT.viewBox.desktop
  }
  const total = spokeList.value.length
  if (total <= 0) return `0 0 ${TOPO_LAYOUT.viewBox.mobileWidth} ${TOPO_LAYOUT.viewBox.mobileEmptyHeight}`
  if (total <= 3) return `0 0 ${TOPO_LAYOUT.viewBox.mobileWidth} ${TOPO_LAYOUT.viewBox.mobileBaseHeight}`
  let remaining = total
  let rows = 0
  while (remaining > 0) {
    const capacity = (rows % 2 === 0) ? 3 : 4
    remaining -= Math.min(remaining, capacity)
    rows++
  }
  const height = TOPO_LAYOUT.viewBox.mobileBaseHeight + (rows - 1) * TOPO_LAYOUT.viewBox.mobileRowStep
  return `0 0 ${TOPO_LAYOUT.viewBox.mobileWidth} ${height}`
})

const handleResize = () => {
  const nextIsMobile = typeof window !== 'undefined' ? window.innerWidth <= 768 : false
  if (nextIsMobile !== isMobile.value) {
    isMobile.value = nextIsMobile
    renderTopology()
  }
}

const handleNodeClick = (node: GraphNode) => {
  if (!node.isSelectable) return
  if (node.type === 'leader' || node.type === 'standby') {
    emit('select-node', node)
  }
}

const formatLeaseTime = (sec?: number) => {
  if (sec === undefined || sec === null) return ''
  if (sec < 60) return `${sec}s`
  const m = Math.floor(sec / 60)
  const s = sec % 60
  if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`
  const h = Math.floor(m / 60)
  const remM = m % 60
  return remM > 0 ? `${h}h ${remM}m` : `${h}h`
}

/**
 * Clean helper to calculate symmetric column distribution coordinates
 */
function computeColumnCoords(count: number, colX: number, centerY: number): { x: number; y: number }[] {
  if (count <= 0) return []
  const { columnOffsets, columnSpreadY } = TOPO_LAYOUT.hub
  if (count === 1) return columnOffsets.single.map((dy) => ({ x: colX, y: centerY + dy }))
  if (count === 2) return columnOffsets.double.map((dy) => ({ x: colX, y: centerY + dy }))
  if (count === 3) return columnOffsets.triple.map((dy) => ({ x: colX, y: centerY + dy }))
  const yStep = columnSpreadY / (count - 1)
  const startY = centerY - columnSpreadY / 2
  return Array.from({ length: count }, (_, i) => ({ x: colX, y: startY + i * yStep }))
}

/**
 * Generates HA Sync Cubic Bezier path between Leader and Destination Hub
 */
function buildHaSyncPath(srcX: number, srcY: number, targetX: number, targetY: number, isRight: boolean): string {
  const ctrl1X = isRight ? srcX + TOPO_LAYOUT.bezier.haLeaderControlOffset : srcX - TOPO_LAYOUT.bezier.haLeaderControlOffset
  const ctrl2X = isRight ? targetX - TOPO_LAYOUT.bezier.haTargetControlOffset : targetX + TOPO_LAYOUT.bezier.haTargetControlOffset
  return `M ${srcX} ${srcY} C ${ctrl1X} ${srcY}, ${ctrl2X} ${targetY}, ${targetX} ${targetY}`
}

/**
 * Generates Witness S-curve (Double-arc) Cubic Bezier path
 */
function buildWitnessProbePath(originX: number, originY: number, targetX: number, targetY: number, isRight: boolean): string {
  const dx = targetX - originX
  const dy = targetY - originY
  const ctrl1X = originX + dx * TOPO_LAYOUT.bezier.probeDxRatio
  const ctrl1Y = originY + Math.max(TOPO_LAYOUT.bezier.probeMinVerticalDip, dy * TOPO_LAYOUT.bezier.probeDyRatio)
  const ctrl2X = isRight ? targetX - TOPO_LAYOUT.bezier.probeEntryControlOffset : targetX + TOPO_LAYOUT.bezier.probeEntryControlOffset
  return `M ${originX} ${originY} C ${ctrl1X} ${ctrl1Y}, ${ctrl2X} ${targetY}, ${targetX} ${targetY}`
}

/**
 * Generates Spoke fanout Cubic Bezier path
 */
function buildSpokePath(originX: number, originY: number, targetX: number, targetY: number, spokeRadius: number): string {
  const midY = (originY + targetY) / 2
  const ctrlOffset = TOPO_LAYOUT.bezier.spokeControlOffsetY
  const entryY = targetY - spokeRadius - TOPO_LAYOUT.bezier.spokeEntryPadding
  return `M ${originX} ${originY} C ${originX} ${midY + ctrlOffset}, ${targetX} ${midY - ctrlOffset}, ${targetX} ${entryY}`
}

/**
 * Standardized Spoke Node creation helper
 */
function createSpokeNode(
  spoke: SpokeInfo,
  idx: number,
  x: number,
  y: number,
  radius: number,
  isDark: boolean,
  isMobileView: boolean
): GraphNode {
  const mode = isDark ? 'dark' : 'light'
  const leaseFormatted = formatLeaseTime(spoke.expires_in_sec)
  const leaseLabel = leaseFormatted
  const cleanProtocolIP = spoke.protocol_address.replace('/32', '')

  return {
    id: spoke.protocol_address,
    title: cleanProtocolIP,
    subtitle1: spoke.nbma_address || '-',
    sub1Color: TOPO_THEME.spoke.sub1[mode],
    subtitle2: spoke.alias || (spoke.flags ? `Flag: ${spoke.flags}` : isMobileView ? 'Dynamic' : 'Dynamic NHRP'),
    sub2Color: spoke.alias ? TOPO_THEME.spoke.alias[mode] : TOPO_THEME.spoke.sub2[mode],
    type: 'spoke',
    symbol: `S${idx + 1}`,
    symbolColor: TOPO_THEME.spoke.symbol[mode],
    x,
    y,
    radius,
    bg: TOPO_THEME.spoke.bg[mode],
    stroke: TOPO_THEME.spoke.stroke,
    leaseText: leaseLabel,
    leaseWidth: isMobileView
      ? Math.max(48, leaseLabel.length * 6 + 10)
      : Math.max(56, leaseLabel.length * 7 + 14),
  }
}

const renderTopology = () => {
  const isDark = store.isDark
  const mode = isDark ? 'dark' : 'light'
  const mobile = isMobile.value
  const viewMode = mobile ? 'mobile' : 'desktop'
  const cx = mobile ? 230 : 500

  const nList: GraphNode[] = []
  const lList: GraphLink[] = []
  const lblList: GraphLinkLabel[] = []

  // ==========================================
  // Tier 1: Witness Center (Top)
  // ==========================================
  const witnessHealthy = isWitnessQuorumHealthy(props.witnessQuorum)
  const witnessNode: GraphNode = {
    id: 'witness',
    title: 'Witness 仲裁中心',
    subtitle1: formatWitnessQuorumStatus(props.witnessQuorum),
    sub1Color: !props.witnessQuorum
      ? TOPO_THEME.witness.stroke
      : witnessHealthy
        ? TOPO_THEME.witness.healthyText[mode]
        : TOPO_THEME.witness.warningText,
    subtitle2: '',
    sub2Color: TOPO_THEME.witness.sub2[mode],
    type: 'witness',
    symbol: '',
    icon: EyeOutline,
    x: cx,
    y: TOPO_LAYOUT.witness.y[viewMode],
    radius: TOPO_LAYOUT.witness.radius[viewMode],
    bg: TOPO_THEME.witness.bg[mode],
    stroke: TOPO_THEME.witness.stroke,
  }
  nList.push(witnessNode)

  // ==========================================
  // Tier 2: Hub Members (Middle)
  // ==========================================
  const members = props.clusterStatus?.members || []
  const localMemberId = props.clusterStatus?.member
  if (members.length === 0) {
    nodes.value = nList
    links.value = lList
    linkLabels.value = lblList
    return
  }

  const findRealNode = (memberId: string, advIp?: string) => {
    return store.nodes.find((n) =>
      n.id === memberId ||
      n.name === memberId ||
      (advIp && (n.host === advIp || n.advertised_ip === advIp))
    )
  }

  const maxPriority = Math.max(...members.map((m) => m.priority || 0))
  const primaryMember = members.find((m) => m.member_id === props.clusterStatus?.primary)
    || members.find((m) => (m.priority || 0) === maxPriority)
    || members[0]
  const clusterLeaderId = props.clusterStatus?.leader
    || members.find((m) => m.is_leader)?.member_id
  const latestLeaderNode = findLatestLeaderNode(store.nodes)
  const latestLeaderMember = latestLeaderNode
    ? members.find((m) => findRealNode(m.member_id, m.advertised_addresses?.[0])?.id === latestLeaderNode.id)
    : undefined
  const useLatestAgentLeader = Boolean(latestLeaderMember && (
    props.clusterStatus?.isolated ||
    props.clusterStatus?.stale ||
    !clusterLeaderId ||
    latestLeaderNode!.term > (props.clusterStatus?.term || 0)
  ))
  const declaredLeaderId = useLatestAgentLeader ? latestLeaderMember!.member_id : clusterLeaderId
  const reportedLeader = members.find((m) => m.member_id === declaredLeaderId && m.state !== 'disabled')
    || members.find((m) => m.is_leader && m.state !== 'disabled')
  const leaderMember = reportedLeader || primaryMember
  const statusContext = {
    selectedMemberId: localMemberId,
    leaderMemberId: reportedLeader?.member_id,
    primaryMemberId: primaryMember.member_id,
    term: Math.max(props.clusterStatus?.term || 0, latestLeaderNode?.term || 0),
    clusterIsolated: props.clusterStatus?.isolated,
    stale: props.clusterStatus?.stale,
  }
  const statusByMember = new Map<string, HubTopologyStatus>()

  members.forEach((m) => {
    const rNode = findRealNode(m.member_id, m.advertised_addresses?.[0])
    const sla = props.slaMatrix?.find((s) => s.node_id === rNode?.id || s.node_id === m.member_id)
    statusByMember.set(m.member_id, classifyHubStatus(m, rNode, sla, statusContext))
  })
  let leftMembers: MemberInfo[] = []
  let rightMembers: MemberInfo[] = []
  let orderedMembers: MemberInfo[] = []
  let leaderIdx = 0

  if (members.length === 2) {
    // Exactly 2 hubs: Symmetrically placed on Left and Right (Primary on Left, Backup on Right)
    const otherMember = members.find((m) => m !== primaryMember) || members[1]
    orderedMembers = [primaryMember, otherMember]
    leaderIdx = orderedMembers.indexOf(leaderMember)
    if (leaderIdx === -1) leaderIdx = 0
  } else {
    const standbyMembers = members.filter((m) => m !== leaderMember)
    ;[leftMembers, rightMembers] = splitBalanced(standbyMembers)
    orderedMembers = [...leftMembers, leaderMember, ...rightMembers]
    leaderIdx = leftMembers.length
  }

  const hubY = TOPO_LAYOUT.hub.y[viewMode]
  const hubRadius = TOPO_LAYOUT.hub.radius[viewMode]
  const leftX = TOPO_LAYOUT.hub.leftX[viewMode]
  const rightX = TOPO_LAYOUT.hub.rightX[viewMode]

  let hubCoords: { x: number; y: number }[] = []
  if (members.length === 2) {
    const halfSpacing = TOPO_LAYOUT.hub.twoHubSpacing[viewMode] / 2
    hubCoords = [
      { x: cx - halfSpacing, y: hubY },
      { x: cx + halfSpacing, y: hubY },
    ]
  } else {
    const leftCoords = computeColumnCoords(leftMembers.length, leftX, hubY)
    const rightCoords = computeColumnCoords(rightMembers.length, rightX, hubY)
    hubCoords = [...leftCoords, { x: cx, y: hubY }, ...rightCoords]
  }

  const hubNodes: GraphNode[] = []
  orderedMembers.forEach((m, idx) => {
    const rNode = findRealNode(m.member_id, m.advertised_addresses?.[0])
    const status = statusByMember.get(m.member_id)!
    const isWarning = status.isIsolated || status.session === 'disconnected' || status.termMismatch
    const originalIndex = members.indexOf(m) + 1
    const hubSymbolText = `H${originalIndex}`
    const coord = hubCoords[idx] || { x: cx, y: hubY }

    const hubNode: GraphNode = {
      id: rNode?.id || m.member_id,
      memberId: m.member_id,
      title: rNode?.name || m.member_id,
      subtitle1: m.advertised_addresses?.[0] || m.observed_address || '-',
      sub1Color: status.isDisabled
        ? TOPO_THEME.disabled.sub1[mode]
        : status.isOffline || status.isIsolated || status.session === 'disconnected'
        ? TOPO_THEME.offline.sub1[mode]
        : status.isAgentOffline
          ? TOPO_THEME.agentOffline.sub1
        : status.isLeader
          ? TOPO_THEME.leader.sub1[mode]
          : TOPO_THEME.standby.sub1[mode],
      subtitle2: formatHubStatus(status, m.priority, props.witnessQuorum),
      sub2Color: status.isDisabled
        ? TOPO_THEME.disabled.sub2
        : status.isOffline || isWarning
        ? TOPO_THEME.offline.sub2
        : status.isAgentOffline
          ? TOPO_THEME.agentOffline.sub2
        : status.isLeader
          ? TOPO_THEME.leader.sub2[mode]
          : TOPO_THEME.standby.sub2[mode],
      type: status.isLeader ? 'leader' : 'standby',
      symbol: hubSymbolText,
      symbolColor: status.isDisabled
        ? TOPO_THEME.disabled.symbol
        : status.isOffline || status.isIsolated || status.session === 'disconnected'
        ? TOPO_THEME.offline.symbol
        : status.isAgentOffline
          ? TOPO_THEME.agentOffline.symbol
        : status.isLeader
          ? TOPO_THEME.leader.symbol[mode]
          : TOPO_THEME.standby.symbol[mode],
      x: coord.x,
      y: coord.y,
      radius: hubRadius,
      bg: status.isDisabled
        ? TOPO_THEME.disabled.bg[mode]
        : status.isOffline || status.isIsolated
        ? TOPO_THEME.offline.bg[mode]
        : status.isLeader
          ? TOPO_THEME.leader.bg[mode]
          : TOPO_THEME.standby.bg[mode],
      stroke: status.isDisabled
        ? TOPO_THEME.disabled.stroke
        : status.isOffline || status.isIsolated || status.session === 'disconnected'
        ? TOPO_THEME.offline.stroke
        : status.isAgentOffline
          ? TOPO_THEME.agentOffline.stroke
        : status.isLeader
          ? TOPO_THEME.leader.stroke
          : TOPO_THEME.standby.stroke[mode],
      isOffline: status.isOffline,
      isDisabled: status.isDisabled,
      isAgentOffline: status.isAgentOffline,
      isSelectable: isNodeSelectable(rNode),
      hubStatus: status,
    }

    hubNodes.push(hubNode)
    nList.push(hubNode)
  })

  // ==========================================
  // HA Sync Links
  // ==========================================
  const leaderNode = hubNodes[leaderIdx]
  const otherHubs = hubNodes.filter((h) => h !== leaderNode)
  const hubEntryPorts = new Map<string, { x: number; y: number }>()

  if (otherHubs.length > 0) {
    const portPad = TOPO_LAYOUT.hub.portPadding

    otherHubs.forEach((hNode) => {
      const linkState = leaderNode.hubStatus && hNode.hubStatus
        ? classifyHALink(leaderNode.hubStatus, hNode.hubStatus, localMemberId)
        : 'unknown'
      const isRight = hNode.x > leaderNode.x

      const srcX = isRight ? (leaderNode.x + leaderNode.radius + portPad) : (leaderNode.x - leaderNode.radius - portPad)
      const srcY = leaderNode.y
      const targetX = isRight ? (hNode.x - hNode.radius - portPad) : (hNode.x + hNode.radius + portPad)
      const targetY = hNode.y
      hubEntryPorts.set(hNode.id, { x: targetX, y: targetY })
      hubEntryPorts.set(leaderNode.id, { x: srcX, y: srcY })

      const pathD = buildHaSyncPath(srcX, srcY, targetX, targetY, isRight)
      const linkKey = 'ha-mesh-' + [leaderNode.id, hNode.id].sort().join('-')

      lList.push({
        id: linkKey,
        source: { x: srcX, y: srcY },
        target: { x: targetX, y: targetY },
        color: linkState === 'disabled'
          ? TOPO_THEME.link.disabled
          : linkState === 'disconnected'
            ? TOPO_THEME.link.haOffline
            : linkState === 'unknown'
              ? TOPO_THEME.link.haUnknown
              : TOPO_THEME.link.haOnline,
        dashed: linkState !== 'online',
        width: TOPO_LAYOUT.bezier.haLineWidth,
        isCurved: true,
        pathD,
      })
    })
  }

  // ==========================================
  // Witness Probe Lines
  // ==========================================
  const portPad = TOPO_LAYOUT.hub.portPadding
  const wOriginY = witnessNode.y + witnessNode.radius + portPad

  hubNodes.forEach((hNode) => {
    const probeUnavailable = Boolean(
      hNode.isOffline || hNode.isAgentOffline || hNode.hubStatus?.isIsolated,
    )
    const isDirectlyBelow = Math.abs(hNode.x - witnessNode.x) < 5
    if (isDirectlyBelow) {
      const topY = hNode.y - hNode.radius - portPad
      lList.push({
        id: 'probe-hub-' + hNode.id,
        source: { x: witnessNode.x, y: wOriginY },
        target: { x: hNode.x, y: topY },
        color: hNode.isDisabled ? TOPO_THEME.link.disabled : probeUnavailable ? TOPO_THEME.link.probeOffline : TOPO_THEME.link.probe,
        dashed: true,
        width: TOPO_LAYOUT.bezier.probeLineWidth,
      })
    } else {
      const isRight = hNode.x > witnessNode.x
      const defaultPort = isRight
        ? { x: hNode.x - hNode.radius - portPad, y: hNode.y }
        : { x: hNode.x + hNode.radius + portPad, y: hNode.y }
      const port = hubEntryPorts.get(hNode.id) || defaultPort
      const pathD = buildWitnessProbePath(witnessNode.x, wOriginY, port.x, port.y, isRight)

      lList.push({
        id: 'probe-hub-' + hNode.id,
        source: { x: witnessNode.x, y: wOriginY },
        target: { x: port.x, y: port.y },
        color: hNode.isDisabled ? TOPO_THEME.link.disabled : probeUnavailable ? TOPO_THEME.link.probeOffline : TOPO_THEME.link.probe,
        dashed: true,
        width: TOPO_LAYOUT.bezier.probeLineWidth,
        isCurved: true,
        pathD,
      })

      if (probeUnavailable) {
        lblList.push({
          id: 'lbl-offline-' + hNode.id,
          x: (witnessNode.x + port.x) / 2,
          y: (wOriginY + port.y) / 2,
          text: hNode.isAgentOffline ? 'Agent 离线' : hNode.hubStatus?.isIsolated ? '节点隔离' : '节点离线',
          width: 68,
          bg: TOPO_THEME.label.offlineBg[mode],
          border: TOPO_THEME.label.offlineBorder,
          textColor: TOPO_THEME.label.offlineText,
        })
      }
    }
  })

  // ==========================================
  // Spokes
  // ==========================================
  const activeSpokeHub = hubNodes.find((h) =>
    h.hubStatus?.isLeader && h.hubStatus.isOnline && !h.hubStatus.isIsolated,
  )

  const displaySpokes = spokeList.value.slice(0, 10)
  const total = displaySpokes.length

  if (total > 0 && activeSpokeHub) {
    const spokeOriginY = activeSpokeHub.y + activeSpokeHub.radius + TOPO_LAYOUT.bezier.spokeOriginClearanceY

    if (!mobile) {
      const { y: spokeY, radius: spRadius, availableWidth, maxStep, linkWidth } = TOPO_LAYOUT.spoke.desktop
      const spokeStep = total > 1 ? Math.min(maxStep, availableWidth / (total - 1)) : 0
      const startX = cx - (spokeStep * (total - 1)) / 2

      displaySpokes.forEach((spoke, idx) => {
        const sx = startX + idx * spokeStep
        const spNode = createSpokeNode(spoke, idx, sx, spokeY, spRadius, isDark, false)
        nList.push(spNode)

        const pathD = buildSpokePath(activeSpokeHub.x, spokeOriginY, sx, spokeY, spRadius)
        lList.push({
          id: 'spoke-' + spoke.protocol_address,
          source: { x: activeSpokeHub.x, y: spokeOriginY },
          target: { x: sx, y: spokeY },
          color: TOPO_THEME.link.spoke,
          width: linkWidth,
          isCurved: true,
          pathD,
        })
      })
    } else {
      const { startY, rowStep, radius: spRadius, availableWidth, linkWidth } = TOPO_LAYOUT.spoke.mobile
      const baseStep = availableWidth / 3
      const rowCounts: number[] = []
      let remaining = total
      let rowIdx = 0

      while (remaining > 0) {
        const capacity = (rowIdx % 2 === 0) ? 3 : 4
        rowCounts.push(Math.min(remaining, capacity))
        remaining -= rowCounts[rowCounts.length - 1]
        rowIdx++
      }

      let spokeIndex = 0
      for (let r = 0; r < rowCounts.length; r++) {
        const count = rowCounts[r]
        const rowY = startY + r * rowStep
        const isWideRow = (r % 2 === 1)

        for (let c = 0; c < count; c++) {
          const spoke = displaySpokes[spokeIndex]
          const sx = isWideRow ? cx - (availableWidth / 2) + c * baseStep : cx - (availableWidth / 2) + (c + 0.5) * baseStep
          const spNode = createSpokeNode(spoke, spokeIndex, sx, rowY, spRadius, isDark, true)
          nList.push(spNode)

          const pathD = buildSpokePath(activeSpokeHub.x, spokeOriginY, sx, rowY, spRadius)
          lList.push({
            id: 'spoke-' + spoke.protocol_address,
            source: { x: activeSpokeHub.x, y: spokeOriginY },
            target: { x: sx, y: rowY },
            color: TOPO_THEME.link.spoke,
            width: linkWidth,
            isCurved: true,
            pathD,
          })

          spokeIndex++
        }
      }
    }
  }

  nodes.value = nList
  links.value = lList
  linkLabels.value = lblList
}

onMounted(() => {
  handleResize()
  window.addEventListener('resize', handleResize)
  renderTopology()
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})

watch(
  () => [props.clusterStatus, props.witnessQuorum, props.spokes, props.slaMatrix, store.nodes, store.isDark],
  () => {
    renderTopology()
  },
  { deep: true }
)
</script>

<style scoped>
.topology-container {
  position: relative;
  background: var(--bg-card);
  border-radius: 10px;
  border: 1px solid var(--border-color);
  padding: 12px;
  margin-top: 0;
  height: 100%;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  box-shadow: var(--card-shadow);
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.canvas-wrapper {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 8px;
  background: var(--bg-body);
  border: 1px solid var(--border-color);
  width: 100%;
  min-height: 440px;
}

.legend-overlay {
  position: absolute;
  top: 12px;
  left: 12px;
  z-index: 4;
  background: var(--bg-card-secondary, rgba(24, 24, 27, 0.75));
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.08));
  border-radius: 6px;
  padding: 8px 12px;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.15);
  pointer-events: none;
}

.legend-items {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  font-size: 11.5px;
}

.legend-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-muted);
  font-weight: 500;
  white-space: nowrap;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.legend-badge.leader .dot {
  background: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.6);
}

.legend-badge.standby .dot {
  background: #64748b;
}

.legend-badge.witness .dot {
  background: #f59e0b;
  box-shadow: 0 0 6px rgba(245, 158, 11, 0.6);
}

.legend-badge.spoke .dot {
  background: #a855f7;
  box-shadow: 0 0 6px rgba(168, 85, 247, 0.6);
}

.legend-badge.disabled .dot {
  background: #71717a;
}

.legend-badge.agent-offline .dot {
  background: #f97316;
}

.legend-badge.ha-warning .dot {
  background: #f59e0b;
}

.legend-badge.offline .dot {
  background: #ef4444;
}

.header-tools {
  display: flex;
  align-items: center;
}

.topology-svg {
  display: block;
  width: 100%;
  height: auto;
  min-height: 380px;
  max-height: 520px;
}

@media (max-width: 768px) {
  .topology-container {
    padding: 8px;
  }
  .legend-overlay {
    top: 8px;
    left: 8px;
    bottom: auto;
    right: auto;
    padding: 6px 10px;
  }
  .legend-items {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
    font-size: 10px;
  }
  .topology-svg {
    min-height: 350px;
    max-height: none;
  }
  .node-tooltip {
    bottom: 12px !important;
    top: unset !important;
    right: 12px !important;
    left: 12px !important;
    min-width: unset !important;
  }
}

.topology-line {
  shape-rendering: geometricPrecision;
  transition: x1 1.3s cubic-bezier(0.25, 1, 0.35, 1), y1 1.3s cubic-bezier(0.25, 1, 0.35, 1),
              x2 1.3s cubic-bezier(0.25, 1, 0.35, 1), y2 1.3s cubic-bezier(0.25, 1, 0.35, 1),
              stroke 1s ease, stroke-dasharray 1s ease, opacity 1s ease;
}

.topology-path {
  shape-rendering: geometricPrecision;
  transition: d 1.3s cubic-bezier(0.25, 1, 0.35, 1), stroke 1s ease, stroke-dasharray 1s ease, opacity 1s ease;
}

.link-label-group {
  transition: transform 1.3s cubic-bezier(0.25, 1, 0.35, 1), opacity 1s ease;
  will-change: transform;
}

.pulse-ring {
  animation: pulse-ring 2.4s cubic-bezier(0.215, 0.61, 0.355, 1) infinite;
}

@keyframes pulse-ring {
  0% {
    r: 32;
    opacity: 0.8;
  }
  60% {
    r: 44;
    opacity: 0;
  }
  100% {
    r: 44;
    opacity: 0;
  }
}

.node-group {
  cursor: pointer;
  transition: transform 1.3s cubic-bezier(0.25, 1, 0.35, 1), opacity 1s ease;
  will-change: transform;
}

.node-group:hover {
  filter: brightness(1.12);
}

.main-node-circle {
  transition: fill 1s ease, stroke 1s ease, r 1s ease;
}

.node-group text {
  transition: fill 1s ease, opacity 1s ease;
}

.active-selected-ring {
  transition: r 1s ease;
}

.hover-ring {
  animation: spin 6s linear infinite;
  transform-origin: center;
}

@keyframes spin {
  from {
    stroke-dashoffset: 0;
  }
  to {
    stroke-dashoffset: 24;
  }
}

.node-tooltip {
  position: absolute;
  bottom: 24px;
  right: 24px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  box-shadow: var(--card-shadow);
  border-radius: 8px;
  padding: 12px 16px;
  min-width: 220px;
  pointer-events: none;
  z-index: 10;
  animation: fadeIn 0.15s ease-out;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.tooltip-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 6px;
}

.tooltip-badge {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.tooltip-title {
  font-weight: 700;
  font-size: 13px;
  color: var(--text-title);
}

.tooltip-type {
  margin-left: auto;
  font-size: 10px;
  font-weight: 600;
  color: var(--text-muted);
}

.tooltip-row {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  margin-bottom: 4px;
  gap: 12px;
}

.tooltip-row .k {
  color: var(--text-muted);
}

.tooltip-row .v {
  font-weight: 500;
  color: var(--text-body);
}

.tooltip-row .code {
  font-family: 'Fira Code', monospace;
  font-size: 11px;
}

.text-purple {
  color: #a855f7;
  font-weight: 600;
}

.mr-2 {
  margin-right: 8px;
}

.node-group.clickable {
  cursor: pointer;
}

.node-group.clickable:hover {
  transform-origin: center;
  filter: drop-shadow(0 0 6px rgba(6, 182, 212, 0.45));
}

.node-group.is-offline {
  cursor: not-allowed !important;
}

.node-group.is-unselectable {
  cursor: not-allowed !important;
}

.node-group.is-offline:hover {
  filter: drop-shadow(0 0 6px rgba(239, 68, 68, 0.55));
}

.node-group.is-disabled:hover {
  filter: drop-shadow(0 0 6px rgba(113, 113, 122, 0.55));
}

.node-group.is-agent-offline:hover {
  filter: drop-shadow(0 0 6px rgba(249, 115, 22, 0.55));
}

.legend-badge.offline .dot {
  background: #ef4444;
}

.active-selected-ring {
  animation: rotateSelectedRing 8s linear infinite;
}

@keyframes rotateSelectedRing {
  from {
    stroke-dashoffset: 0;
  }
  to {
    stroke-dashoffset: 60;
  }
}
</style>
