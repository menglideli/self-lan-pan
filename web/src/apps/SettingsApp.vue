<template>
  <div class="app-root">
    <div style="display: flex; flex: 1; overflow: hidden">
      <div style="width: 220px; flex: none; border-right: 1px solid var(--stroke); padding: 14px 8px">
        <div v-for="t in tabs" :key="t.id" class="side-item" :class="{ active: tab === t.id }" @click="tab = t.id">
          <AppIcon :name="t.icon" :size="17" /><span>{{ t.name }}</span>
        </div>
      </div>
      <div style="flex: 1; overflow: auto; padding: 26px 34px">
        <!-- 个性化 -->
        <template v-if="tab === 'person'">
          <h2 style="font-size: 19px; margin-bottom: 20px">个性化</h2>
          <div style="font-size: 13px; color: var(--text-2); margin-bottom: 10px">站点主题（管理员设置，对全体用户生效——游客与普通用户访问都看到该主题，且无此设置入口）</div>
          <div style="display: flex; gap: 12px; margin-bottom: 22px">
            <div v-for="t in themes" :key="t.id"
              style="cursor: pointer; display: flex; align-items: center; gap: 12px; padding: 14px 18px; border-radius: var(--radius); border: 1.5px solid var(--stroke); background: var(--card)"
              :style="session.osTheme === t.id ? { borderColor: 'var(--theme-2)', background: 'var(--hover-b)' } : {}"
              @click="session.setOsTheme(t.id)">
              <AppIcon :name="t.icon" :size="34" />
              <div>
                <div style="font-size: 14px; font-weight: 600">{{ t.name }}</div>
                <div style="font-size: 11.5px; color: var(--text-3)">{{ session.osTheme === t.id ? '当前使用' : '点击切换' }}</div>
              </div>
              <AppIcon v-if="session.osTheme === t.id" name="check" :size="16" style="color: var(--theme-2)" />
            </div>
          </div>
          <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 22px">
            <span style="font-size: 13px; color: var(--text-2)">深色模式</span>
            <button class="btn" @click="session.setDark(!session.dark)">{{ session.dark ? '开' : '关' }}</button>
          </div>
          <div style="font-size: 13px; color: var(--text-2); margin-bottom: 10px">选择壁纸（{{ theme.name }}）</div>
          <div style="display: flex; flex-wrap: wrap; gap: 12px; overflow: hidden">
            <div v-for="w in theme.wallpapers" :key="w.key" style="cursor: pointer" @click="session.setWallpaper(w.key)">
              <div class="wp-thumb" :class="wallpaperClass(w.key)" :style="{ outline: session.wallpaper === w.key ? '2px solid var(--theme-2)' : 'none' }"></div>
              <div style="font-size: 12px; color: var(--text-2); text-align: center; margin-top: 5px">{{ w.name }}</div>
            </div>
          </div>
        </template>

        <!-- 账号 -->
        <template v-else-if="tab === 'account'">
          <h2 style="font-size: 19px; margin-bottom: 20px">账号信息</h2>
          <div style="display: flex; align-items: center; gap: 18px; margin-bottom: 24px">
            <div class="avatar" style="width: 72px; height: 72px; font-size: 30px">{{ initial }}</div>
            <div>
              <div style="font-size: 17px">{{ session.user?.nickname }}</div>
              <div style="font-size: 12.5px; color: var(--text-3)">@{{ session.user?.username }} · {{ session.perms?.name }}</div>
              <div style="font-size: 12.5px; color: var(--text-3)">
                已用 {{ fmt(session.user?.usedBytes || 0) }} / {{ session.perms?.quotaMB && session.perms.quotaMB > 0 ? session.perms.quotaMB + ' MB' : '不限量' }}
              </div>
            </div>
          </div>
          <div class="form-row">
            <label>昵称</label>
            <input class="input" v-model="nickname" style="width: 260px" />
            <button class="btn" @click="saveProfile">保存</button>
          </div>
          <div class="form-row" style="margin-top: 22px">
            <label>修改密码</label>
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <input class="input" type="password" v-model="oldPwd" placeholder="原密码" style="width: 180px" />
              <input class="input" type="password" v-model="newPwd" placeholder="新密码（至少6位）" style="width: 180px" />
              <button class="btn" @click="doChangePwd">修改密码</button>
            </div>
          </div>
          <div class="form-row" style="margin-top: 22px">
            <label>WebDAV 独立密码（用于挂载到 Windows 资源管理器 / 手机）</label>
            <div style="display: flex; gap: 8px">
              <input class="input" v-model="davPwd" placeholder="设置/重置 WebDAV 密码" style="width: 260px" />
              <button class="btn" @click="doDavPwd">保存</button>
            </div>
            <template v-if="session.perms?.allowWebdav">
              <div class="dav-head">
                <span>访问地址 —— 本机有多张网卡时，选一个「对方能访问到」的复制</span>
                <button class="tool-btn" style="padding: 2px 8px; flex: none" @click="refreshAddrs()">刷新</button>
              </div>
              <div class="dav-list">
                <div v-for="a in addresses" :key="a.ip" class="dav-row">
                  <span class="dav-iface" :title="a.iface">{{ a.iface }}</span>
                  <span v-if="isCurrent(a)" class="dav-tag cur">当前</span>
                  <span v-else-if="a.kind === 'virtual'" class="dav-tag">虚拟网卡</span>
                  <span class="dav-url">{{ a.url }}/dav/</span>
                  <button class="btn" style="padding: 2px 10px; flex: none" @click="copyDav(a)">复制</button>
                </div>
                <div v-if="!addresses.length" class="dav-empty">
                  没有枚举到可用地址（网卡可能未连接），可手动填写 http://&lt;本机IP&gt;:18322/dav/
                </div>
              </div>
              <div class="dav-tip">
                粘贴到手机的 WebDAV 客户端（ES 文件浏览器 / Solid Explorer / nPlayer 等）即可。<br>
                这一个地址就是全部挂载（不用一个个加）；只想单独挂某一个用 &lt;地址&gt;&lt;挂载名&gt;/<br>
                登录用网页账号 + 上面这个独立密码（<b>不是网页登录密码</b>）。<br>
                <span v-if="lastCopied">已复制：<b>{{ lastCopied }}</b></span>
              </div>
            </template>
            <div v-else style="font-size: 12px; color: var(--text-3); margin-top: 6px">WebDAV 未启用</div>
          </div>
        </template>

        <!-- 共享管理 -->
        <template v-else-if="tab === 'shares'">
          <h2 style="font-size: 19px; margin-bottom: 20px">我的分享</h2>
          <div class="ac-sub">外链分享</div>
          <table class="file-list" style="position: static">
            <thead><tr><th>名称</th><th>提取码</th><th>浏览/下载</th><th>到期</th><th>操作</th></tr></thead>
            <tbody>
              <tr v-for="s in shares" :key="s.id">
                <td>{{ s.name }}</td>
                <td>{{ s.passwordHash ? '有密码' : '公开' }}</td>
                <td>{{ s.views }} / {{ s.downloads }}</td>
                <td>{{ s.expiresAt ? new Date(s.expiresAt).toLocaleDateString() : '永久' }}</td>
                <td>
                  <button class="tool-btn" style="padding: 3px 8px" @click="copyShare(s)">复制链接</button>
                  <button class="tool-btn danger" style="padding: 3px 8px" @click="cancelShare(s)">取消</button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-if="!shares.length" style="color: var(--text-3); padding: 16px; text-align: center">暂无外链分享</div>
        </template>

        <!-- 离线下载 -->
        <template v-else-if="tab === 'offline'">
          <h2 style="font-size: 19px; margin-bottom: 20px">离线下载</h2>
          <div style="display: flex; gap: 8px; margin-bottom: 18px; flex-wrap: wrap">
            <select class="input" v-model.number="offPolicyId" style="width: 150px">
              <option v-for="p in policies" :key="p.id" :value="p.id">{{ p.name }} ({{ p.letter }})</option>
            </select>
            <input class="input" v-model="offUrl" placeholder="HTTP 直链 / m3u8 视频 / .torrent 种子 / magnet: 磁力链" style="flex: 1; min-width: 260px" />
            <input class="input" v-model="offName" placeholder="文件名(可选)" style="width: 150px" />
            <input class="input" v-model="offReferer" placeholder="Referer(可选，防盗链站点用)" style="width: 200px" />
            <select class="input" v-model="offDest" style="width: 130px">
              <option value="/">根目录</option>
              <option v-for="s in stars" :key="s.id" :value="s.path">{{ s.name }}</option>
            </select>
            <button class="btn primary" @click="addOffline" :disabled="!offUrl">添加任务</button>
          </div>
          <div style="font-size: 11.5px; color: var(--text-3); margin-top: 8px; line-height: 1.8">
            • <b>HTTP 直链</b>：服务器代下载；若该地址实际返回 m3u8 清单会自动转成 m3u8 下载。<br>
            • <b>m3u8 视频</b>：自动选最高码率 → 并发抓分片 → AES-128 自动解密 → 合并保存为 <code>.ts</code>（用 VLC / PotPlayer / 手机播放器播放）。<br>
            • <b>.torrent / magnet:</b> 由内置 BT 引擎（DHT + 公共 tracker）下载，完成后自动导入所选磁盘。<br>
            • 内网地址（如 NAS、内网媒体服务器）默认被安全策略拒绝，需在「管理控制台 → 站点设置」开启<span style="color: var(--text-2)">「允许离线下载访问内网地址」</span>。
          </div>
          <table class="file-list" style="position: static">
            <thead><tr><th>文件</th><th>状态</th><th>进度</th><th>时间</th><th>操作</th></tr></thead>
            <tbody>
              <tr v-for="t in offTasks" :key="t.id">
                <td style="max-width: 260px; overflow: hidden; text-overflow: ellipsis">{{ taskName(t) }}</td>
                <td><span class="tag" :style="t.status === 'error' ? 'color:var(--danger)' : ''">{{ taskStatus(t.status) }}</span></td>
                <td style="width: 160px">
                  <div class="progress-track"><div class="progress-fill" :style="{ width: t.progress + '%' }"></div></div>
                </td>
                <td style="color: var(--text-3)">{{ new Date(t.createdAt).toLocaleTimeString() }}</td>
                <td><button v-if="t.status === 'queued' || t.status === 'processing'" class="tool-btn danger" style="padding: 3px 8px" @click="cancelOffline(t)">取消</button></td>
              </tr>
            </tbody>
          </table>
          <div v-if="!offTasks.length" style="color: var(--text-3); padding: 30px; text-align: center">暂无离线任务</div>
        </template>

        <!-- 关于 -->
        <template v-else>
          <h2 style="font-size: 19px; margin-bottom: 20px">关于</h2>
          <div style="display: flex; align-items: center; gap: 16px">
            <AppIcon name="windows" :size="52" />
            <div>
              <div style="font-size: 18px; font-weight: 600">CloudPan 网盘</div>
              <div style="font-size: 12.5px; color: var(--text-3)">Go + Vue 仿 Windows12 桌面网盘系统 · v1.0.0</div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- 多网卡地址选择：复制分享链接时让用户挑对外可达的地址 -->
    <AddressPicker v-model:show="sharePickerShow" title="复制分享链接" :path="sharePickerPath" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useSession } from '../stores/session'
import { canUseOffline } from '../stores/apps'
import { wallpaperClass } from '../assets/wallpapers'
import { availableThemes, resolveTheme } from '../themes/registry'
import { fsApi, shareApi, authApi } from '../api/modules'
import { get as aget, post as apost, del as adel } from '../api/http'
import { useToast, useUiDialog } from '../stores/dialog'
import { copyText } from '../utils/clipboard'
import { loadAddresses, withCurrentFirst, isCurrent, type LanAddr } from '../utils/lanaddr'
import AppIcon from '../components/AppIcon.vue'
import AddressPicker from '../components/AddressPicker.vue'

const toast = useToast()
const uiDlg = useUiDialog()

const session = useSession()
// 主题
const themes = availableThemes()
const theme = computed(() => resolveTheme(session.osTheme))
// 离线下载
const policies = ref<any[]>([])
const stars = ref<any[]>([])
const offTasks = ref<any[]>([])
const offUrl = ref(''); const offName = ref(''); const offDest = ref('/')
const offReferer = ref('')
const offPolicyId = ref(0)
let pollTimer = 0

function taskName(t: any) {
  try { const p = JSON.parse(t.props); return p.name || p.url } catch { return t.props }
}
function taskStatus(s: string) {
  return ({ queued: '排队中', processing: '下载中', finished: '完成', error: '失败', canceled: '已取消' } as any)[s] || s
}
async function loadOffline() {
  try {
    policies.value = await fsApi.policies()
    stars.value = await fsApi.starList()
    offTasks.value = await aget('/offline')
    if (!offPolicyId.value && policies.value.length) offPolicyId.value = policies.value[0].id
  } catch {}
}
async function addOffline() {
  try {
    await apost('/offline', {
      policyId: offPolicyId.value, dest: offDest.value, url: offUrl.value,
      name: offName.value || undefined, referer: offReferer.value.trim() || undefined
    })
    offUrl.value = ''; offName.value = ''
    loadOffline()
  } catch (e: any) { toast.error(e.message) }
}
async function cancelOffline(t: any) {
  try { await adel(`/offline/${t.id}`); loadOffline() } catch (e: any) { alert(e.message) }
}
// 个性化（站点主题/壁纸/深色）为管理员全局设置：仅管理员可见此 tab，
// 游客与普通用户从所有入口都看不到主题个性化功能
const isAdmin = computed(() => session.user?.role === 'admin')
// 离线下载页签：无权限用户（如游客）整体不显示，与后端 403 语义一致
const canOffline = computed(() => canUseOffline())
const tabs = computed(() => [
  ...(isAdmin.value ? [{ id: 'person', name: '个性化', icon: 'sun' }] : []),
  { id: 'account', name: '账号', icon: 'user' },
  { id: 'shares', name: '我的分享', icon: 'share' },
  ...(canOffline.value ? [{ id: 'offline', name: '离线下载', icon: 'download' }] : []),
  { id: 'about', name: '关于', icon: 'info' }
])
const winProps = defineProps<{ winId: number; props: any }>()
const tab = ref('person')
// 当前 tab 不可见时（非管理员无个性化 / 角色异步到位）回落到第一个可见 tab
watch([tabs, () => session.user?.role], () => {
  if (!tabs.value.some(t => t.id === tab.value)) tab.value = tabs.value[0]?.id || 'account'
}, { immediate: true })
// 深链：搜索面板「设置」分类 / 其他入口带 { tab } 打开时直接跳到对应页（仍受可见性约束）
watch(() => winProps.props?.tab, v => { if (v && tabs.value.some(t => t.id === v)) tab.value = v }, { immediate: true })
const nickname = ref(session.user?.nickname || '')
const oldPwd = ref(''); const newPwd = ref(''); const davPwd = ref('')
const shares = ref<any[]>([])

const initial = computed(() => (session.user?.nickname || 'C').charAt(0).toUpperCase())

onMounted(() => {
  loadShares()
  loadOffline()
  refreshAddrs(false) // 地址列表用缓存的，避免每次开设置页都打一次接口
  pollTimer = window.setInterval(() => { if (tab.value === 'offline') loadOffline() }, 3000)
})
onBeforeUnmount(() => clearInterval(pollTimer))
async function loadShares() {
  try { shares.value = await shareApi.mine() } catch {}
}
async function saveProfile() {
  try { await authApi.updateMe({ nickname: nickname.value }); await session.loadMe() } catch (e: any) { alert(e.message) }
}
async function doChangePwd() {
  try { await authApi.changePassword(oldPwd.value, newPwd.value); toast.success('密码已修改'); oldPwd.value = newPwd.value = '' } catch (e: any) { toast.error(e.message) }
}
async function doDavPwd() {
  try { await authApi.setWebdavPassword(davPwd.value); toast.success('WebDAV 密码已设置'); davPwd.value = '' } catch (e: any) { toast.error(e.message) }
}
// WebDAV 访问地址：不再用 location.origin 猜（多网卡的机器上会给出别人打不开的地址），
// 改为由后端枚举本机全部网卡地址，逐条列出让用户自己挑。
const addresses = ref<LanAddr[]>([])
const lastCopied = ref('')
async function refreshAddrs(force = true) {
  addresses.value = withCurrentFirst(await loadAddresses(force))
}
function copyDav(a: LanAddr) {
  const u = a.url + '/dav/'
  copyText(u).then(ok => {
    if (ok) { lastCopied.value = u; toast.success('WebDAV 地址已复制') }
    else toast.error('复制失败，请手动选择复制')
  })
}
// 分享链接：多网卡时弹窗让用户挑一个对方能访问的地址
const sharePickerShow = ref(false)
const sharePickerPath = ref('')
function copyShare(s: any) {
  sharePickerPath.value = '/#/s/' + s.token + (s.passwordHash ? '  提取码见分享设置' : '')
  sharePickerShow.value = true
}
async function cancelShare(s: any) {
  try { await shareApi.cancel(s.id); loadShares() } catch (e: any) { alert(e.message) }
}
function fmt(n: number) {
  if (n > 1 << 30) return (n / (1 << 30)).toFixed(2) + ' GB'
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  return (n / 1024).toFixed(1) + ' KB'
}
</script>

<style scoped>
/* ---- WebDAV 访问地址列表：多网卡逐条展示 + 各自复制 ---- */
.dav-head {
  display: flex; align-items: center; gap: 8px; margin-top: 12px;
  font-size: 12px; color: var(--text-3);
}
.dav-list { margin-top: 6px; max-width: 660px; }
.dav-row {
  display: flex; align-items: center; gap: 8px; padding: 6px 10px;
  border: 1px solid var(--stroke-b); border-radius: 7px; margin-bottom: 5px;
  background: var(--bg50);
}
.dav-iface {
  flex: none; max-width: 190px; font-size: 12px; color: var(--text-2);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.dav-tag {
  flex: none; font-size: 10.5px; padding: 1px 6px; border-radius: 999px;
  border: 1px solid var(--stroke); color: var(--text-3);
}
.dav-tag.cur { color: var(--theme-2); border-color: color-mix(in srgb, var(--theme-2) 45%, transparent); }
.dav-url { flex: 1; font-size: 12.5px; color: var(--text); word-break: break-all; }
.dav-empty { font-size: 12px; color: var(--text-3); padding: 8px 10px; }
.dav-tip { font-size: 12px; color: var(--text-3); margin-top: 8px; line-height: 1.9; }

.side-item {
  display: flex; align-items: center; gap: 10px; padding: 8px 12px; border-radius: 6px;
  font-size: 13.5px; cursor: pointer; color: var(--text-2); margin-bottom: 2px;
}
.side-item:hover { background: var(--hover-b); color: var(--text); }
.side-item.active { background: #3b91d825; color: var(--text); }
.form-row label { display: block; font-size: 12.5px; color: var(--text-2); margin-bottom: 8px; }
.ac-sub { font-size: 13px; font-weight: 600; color: var(--theme-2); margin: 6px 0 10px; }
.ac-sub + .file-list { margin-bottom: 8px; }
.wp-thumb { width: 130px; height: 78px; border-radius: 8px; border: 1px solid var(--stroke); }
</style>
