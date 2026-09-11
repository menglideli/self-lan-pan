<template>
  <!-- 统一任务中心：聚合「本机上传/秒传队列（transfer store）」与「服务端离线下载队列（5s 轮询）」 -->
  <div class="app-root tc-root">
    <div class="app-toolbar">
      <AppIcon name="tasks" :size="18" />
      <b>任务中心</b>
      <span class="tc-sub">进行中 {{ runningCount }} · 已完成 {{ finishedCount }}</span>
      <div style="flex: 1"></div>
      <button class="tool-btn" :disabled="refreshing" @click="loadOffline" title="刷新离线任务">
        <AppIcon name="refresh" :size="15" />
      </button>
      <button class="tool-btn" :disabled="!tc.tasks.some(t => t.status === 'done' || t.status === 'instant')"
        @click="tc.clearFinished()">清除已完成</button>
    </div>

    <div class="tc-body">
      <!-- 上传 / 秒传（本机队列，随 transfer store 实时响应） -->
      <div class="tc-sec">
        <div class="tc-sec-title">
          <AppIcon name="upload" :size="15" /> 上传 / 秒传
          <span class="tc-sec-count">{{ tc.tasks.length }}</span>
        </div>
        <div v-if="!tc.tasks.length" class="tc-empty">暂无上传任务（在资源管理器中上传文件后会出现在这里）</div>
        <div v-for="t in tc.tasks" :key="t.id" class="tc-row">
          <AppIcon :name="t.status === 'error' ? 'file' : 'upload'" :size="20" />
          <div class="tc-main">
            <div class="tc-name" :title="t.name">{{ t.name }}</div>
            <div class="tc-bar">
              <div class="tc-bar-fill" :class="{ err: t.status === 'error', ok: t.status === 'done' || t.status === 'instant' }"
                :style="{ width: (t.status === 'done' || t.status === 'instant' ? 100 : t.progress) + '%' }"></div>
            </div>
          </div>
          <div class="tc-meta">{{ fmtSize(t.size) }}</div>
          <div class="tc-st" :class="tCls(t.status)">{{ tStatusText(t.status, t.errMsg) }}</div>
          <div class="tc-ops">
            <button v-if="t.status === 'uploading' || t.status === 'hashing'" class="tool-btn" title="暂停" @click="tc.pause(t.id)">
              <AppIcon name="pause" :size="14" />
            </button>
            <button v-else-if="t.status === 'paused'" class="tool-btn" title="继续" @click="tc.resume(t.id)">
              <AppIcon name="play" :size="14" />
            </button>
            <button class="tool-btn" title="取消" @click="tc.cancel(t.id)"><AppIcon name="close" :size="14" /></button>
          </div>
        </div>
      </div>

      <!-- 离线下载（服务端队列，5s 轮询） -->
      <div v-if="offlineAllowed" class="tc-sec">
        <div class="tc-sec-title">
          <AppIcon name="download" :size="15" /> 离线下载
          <span class="tc-sec-count">{{ offline.length }}</span>
          <button class="tool-btn" @click="offAddShow = !offAddShow">{{ offAddShow ? '收起' : '＋ 新建' }}</button>
        </div>
        <div v-if="offAddShow" class="tc-addform">
          <input class="input" v-model="offUrl" placeholder="http(s) 直链 / m3u8 视频 / .torrent / magnet: 磁力链接" style="flex: 1" @keyup.enter="addOffline" />
          <select class="input" v-model.number="offPolicyId" style="width: 150px">
            <option v-for="p in offPolicies" :key="p.id" :value="p.id">{{ p.name }} ({{ p.letter }})</option>
          </select>
          <input class="input" v-model="offDest" placeholder="保存目录，如 /downloads" style="width: 160px" @keyup.enter="addOffline" />
          <button class="btn primary" :disabled="!offUrl.trim() || offBusy" @click="addOffline">{{ offBusy ? '添加中…' : '添加' }}</button>
          <div class="tc-adv">
            <input class="input" v-model="offReferer" placeholder="Referer（可选；防盗链站点才需要）" style="flex: 1" />
          </div>
          <div class="tc-kindhint">{{ kindHint }}</div>
          <div v-if="offMsg" class="tc-errmsg">{{ offMsg }}</div>
        </div>
        <div v-if="!offline.length && !offAddShow" class="tc-empty">暂无离线下载任务</div>
        <div v-for="t in offline" :key="t.id" class="tc-row">
          <AppIcon :name="t.status === 'error' ? 'file' : (t.kind === 'bt' ? 'cloud' : (t.kind === 'm3u8' ? 'video' : 'download'))" :size="20" />
          <div class="tc-main">
            <div class="tc-name" :title="t.url">{{ t.name || t.url }}</div>
            <div class="tc-bar">
              <div class="tc-bar-fill" :class="{ err: t.status === 'error', ok: t.status === 'finished' }"
                :style="{ width: (t.status === 'finished' ? 100 : t.progress) + '%' }"></div>
            </div>
            <div class="tc-subline" v-if="t.kind === 'm3u8' && t.segTotal">
              分片 {{ t.segDone || 0 }} / {{ t.segTotal }}<span v-if="t.variant"> · {{ t.variant }}</span>
            </div>
            <div class="tc-subline" v-if="t.dest !== '/'">保存到 {{ t.dest }}</div>
            <!-- 失败原因 / 阶段提示（后端把诊断写在这里，之前只在任务中心外露不出来） -->
            <div v-if="t.error" class="tc-subline err" :title="t.error">{{ t.error }}</div>
          </div>
          <div class="tc-st" :class="tCls(t.status)">{{ offStatusText(t.status) }}</div>
          <div class="tc-ops">
            <button v-if="t.status === 'queued' || t.status === 'processing'" class="tool-btn" title="取消" @click="cancelOffline(t.id)">
              <AppIcon name="close" :size="14" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { get, post, del } from '../api/http'
import { useTransfer } from '../stores/transfer'
import { canUseOffline } from '../stores/apps'

const tc = useTransfer()
// 响应式：离线下载可用性取决于异步加载的用户组权限与系统功能开关
const offlineAllowed = computed(() => canUseOffline())

const runningCount = computed(() =>
  tc.tasks.filter(t => t.status === 'uploading' || t.status === 'hashing').length +
  offline.value.filter(t => t.status === 'queued' || t.status === 'processing').length)
const finishedCount = computed(() =>
  tc.tasks.filter(t => t.status === 'done' || t.status === 'instant').length +
  offline.value.filter(t => t.status === 'finished').length)

function fmtSize(n: number) {
  if (n > 1 << 30) return (n / (1 << 30)).toFixed(2) + ' GB'
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  if (n > 1024) return (n / 1024).toFixed(1) + ' KB'
  return n + ' B'
}
function tCls(s: string) {
  if (s === 'done' || s === 'instant' || s === 'finished') return 'ok'
  if (s === 'error') return 'err'
  if (s === 'paused' || s === 'canceled') return 'idle'
  return 'run'
}
function tStatusText(s: string, errMsg?: string) {
  switch (s) {
    case 'hashing': return '计算哈希…'
    case 'uploading': return '上传中'
    case 'paused': return '已暂停'
    case 'done': return '已完成'
    case 'instant': return '秒传'
    case 'error': return errMsg ? '失败：' + errMsg.slice(0, 40) : '失败'
    default: return s
  }
}
function offStatusText(s: string) {
  switch (s) {
    case 'queued': return '排队中'
    case 'processing': return '下载中'
    case 'finished': return '已完成'
    case 'error': return '失败'
    case 'canceled': return '已取消'
    default: return s
  }
}

// ---- 离线下载（服务端任务队列，5s 轮询）----
interface OffTask {
  id: number; type: string; status: string; progress: number; error: string
  url: string; name?: string; dest: string; kind: string
  segDone?: number; segTotal?: number; variant?: string
}
const offline = ref<OffTask[]>([])
const offAddShow = ref(false)
const offUrl = ref('')
const offDest = ref('/downloads')
const offReferer = ref('')
const offPolicyId = ref(0)
const offPolicies = ref<any[]>([])
const offBusy = ref(false)
const offMsg = ref('')
const refreshing = ref(false)
let timer: number | undefined

// 按链接形状实时告诉用户会被当成哪种任务，省得"以为在下视频、实际存了个清单文本"
const kindHint = computed(() => {
  const u = offUrl.value.trim().toLowerCase()
  if (!u) return '支持：当直链 / m3u8 视频 / .torrent 种子 / magnet: 磁力链接'
  if (u.startsWith('magnet:')) return '将按「磁力链」下载（内置 BT 引擎，需要能访问 DHT/公共 tracker）'
  if (u.includes('.m3u8') || u.includes('.m3u')) return '将按「m3u8 视频」下载：自动选最高码率 → 并发抓分片 → AES-128 自动解密 → 合并保存为 .ts'
  if (u.endsWith('.torrent')) return '将按「种子文件」下载'
  if (u.startsWith('http')) return '将按「HTTP 直链」代下载；若该地址实际返回 m3u8 清单，会自动转成 m3u8 下载'
  return ''
})

async function loadOffline() {
  if (!offlineAllowed.value) return
  refreshing.value = true
  try {
    const d = await get<any[]>('/offline')
    offline.value = (d || []).map(t => {
      let p: any = {}
      try { p = JSON.parse(t.props || '{}') } catch { /* ignore */ }
      return { id: t.id, type: t.type, status: t.status, progress: t.progress ?? 0, error: t.error || '',
        url: p.url || '', name: p.name || p.rtName || '', dest: p.dest || '/', kind: p.kind || 'http',
        segDone: p.segDone || 0, segTotal: p.segTotal || 0, variant: p.variant || '' }
    })
  } catch { /* 轮询失败静默，下轮重试 */ } finally {
    refreshing.value = false
  }
}
async function loadPolicies() {
  if (!offlineAllowed.value) return
  try {
    const d = await get<any[]>('/policies')
    offPolicies.value = d || []
    if (offPolicies.value.length && !offPolicyId.value) offPolicyId.value = offPolicies.value[0].id
  } catch { /* ignore */ }
}
async function addOffline() {
  offMsg.value = ''
  if (!offUrl.value.trim() || !offPolicyId.value) return
  offBusy.value = true
  try {
    await post('/offline', {
      policyId: offPolicyId.value,
      dest: offDest.value.trim() || '/',
      url: offUrl.value.trim(),
      referer: offReferer.value.trim() || undefined
    })
    offUrl.value = ''
    offAddShow.value = false
    await loadOffline()
  } catch (e: any) {
    offMsg.value = e?.message || '添加失败'
  } finally {
    offBusy.value = false
  }
}
async function cancelOffline(id: number) {
  try { await del(`/offline/${id}`); await loadOffline() } catch (e: any) { offMsg.value = e?.message || '取消失败' }
}

onMounted(() => {
  loadOffline()
  loadPolicies()
  timer = window.setInterval(loadOffline, 5000)
})
onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<style scoped>
.tc-root { display: flex; flex-direction: column }
.tc-sub { font-size: 12px; color: var(--text-3); margin-left: 6px }
.tc-body { flex: 1; overflow: auto; padding: 8px 16px 16px }
.tc-sec { margin-top: 10px }
.tc-sec-title {
  display: flex; align-items: center; gap: 7px;
  font-size: 13px; font-weight: 600; color: var(--text-2);
  padding: 6px 2px; position: sticky; top: 0;
  background: var(--bg1); z-index: 1;
}
.tc-sec-count {
  font-size: 11px; color: var(--text-3); font-weight: 400;
  background: rgba(127, 127, 127, 0.15); border-radius: 999px; padding: 1px 8px;
}
.tc-empty {
  font-size: 12.5px; color: var(--text-3);
  padding: 18px 4px; text-align: center;
}
.tc-row {
  display: grid;
  grid-template-columns: 30px 1fr 90px 110px 74px;
  align-items: center; gap: 10px;
  padding: 8px 4px; border-bottom: 1px solid var(--stroke);
}
.tc-main { min-width: 0 }
.tc-name {
  font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.tc-bar {
  height: 4px; border-radius: 2px; margin-top: 5px;
  background: rgba(127, 127, 127, 0.18); overflow: hidden;
}
.tc-bar-fill { height: 100%; border-radius: 2px; background: var(--theme-1, #3b91d8); transition: width 300ms }
.tc-bar-fill.ok { background: #4caf50 }
.tc-bar-fill.err { background: #e53935 }
.tc-meta { font-size: 12px; color: var(--text-3); text-align: right; white-space: nowrap }
.tc-st { font-size: 12px; text-align: right; white-space: nowrap; overflow: hidden; text-overflow: ellipsis }
.tc-st.ok { color: #4caf50 }
.tc-st.err { color: #e53935 }
.tc-st.run { color: var(--theme-1, #3b91d8) }
.tc-st.idle { color: var(--text-3) }
.tc-ops { display: flex; justify-content: flex-end; gap: 2px }
.tc-subline { font-size: 11px; color: var(--text-3); margin-top: 2px }
.tc-addform {
  display: flex; flex-wrap: wrap; gap: 8px; align-items: center;
  padding: 10px 4px; border-bottom: 1px solid var(--stroke);
}
.tc-errmsg { width: 100%; font-size: 12px; color: #e53935 }
/* 高级参数行（Referer）与"这条链接会被当成什么任务"的即时提示 */
.tc-adv { width: 100%; display: flex; gap: 8px; align-items: center; }
.tc-kindhint { width: 100%; font-size: 11.5px; color: var(--text-3); line-height: 1.6 }
/* 任务失败原因/阶段提示：只在有内容时出现，过长省略并可悬停看全文 */
.tc-subline.err {
  color: #e53935; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 520px;
}
</style>
