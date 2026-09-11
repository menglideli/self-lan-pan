import { ref } from 'vue'
import { get } from '../api/http'

// 本机可访问地址（多网卡）。以前前端一律用 location.origin 拼地址，
// 等于假设"你能访问的地址，别人也能访问" —— 多网卡机器上这是错的。
// 这里统一从后端枚举，并带上"当前访问来源"的标记，让用户自己挑。

export interface LanAddr {
  iface: string // 网卡名：WLAN / 以太网 / VMware Network Adapter VMnet8 …
  ip: string
  url: string // http://<ip>:<port>
  kind: string // lan | virtual | other
}

const cache = ref<LanAddr[] | null>(null)
let inflight: Promise<LanAddr[]> | null = null

export async function loadAddresses(force = false): Promise<LanAddr[]> {
  if (!force && cache.value) return cache.value
  if (!inflight) {
    inflight = get<LanAddr[]>('/system/addresses')
      .then(d => {
        cache.value = Array.isArray(d) ? d : []
        return cache.value
      })
      .catch(() => {
        cache.value = []
        return [] as LanAddr[]
      })
      .finally(() => { inflight = null })
  }
  return inflight
}

// 当前页面是用哪个地址打开的 —— 那条一定是通的，排最前面并标记出来
export function currentHost(): string {
  return location.hostname
}

export function isCurrent(a: LanAddr): boolean {
  return a.ip === location.hostname
}

// 排序：当前访问来源 > 常规局域网 > 其他 > 虚拟网卡（后端已按 kind 排过，这里只把当前来源提前）
export function withCurrentFirst(list: LanAddr[]): LanAddr[] {
  const i = list.findIndex(isCurrent)
  if (i <= 0) return list
  return [list[i], ...list.slice(0, i), ...list.slice(i + 1)]
}

// 建议默认选中的那一条：优先当前访问来源，否则第一个常规局域网地址
export function preferred(list: LanAddr[]): LanAddr | null {
  if (!list.length) return null
  return list.find(isCurrent) || list.find(a => a.kind === 'lan') || list[0]
}
