<template>
  <!-- #loginback：壁纸上居中 150px 头像 + 名称 + 密码输入 + 登录钮；
       成功后 #login-welc（转环+欢迎）→ .close（整体压暗）→ 淡出进桌面。
       单用户私有部署：账号固定为管理员，不再有用户名输入 / 游客登录 / 注册入口 -->
  <div class="w12-login" :class="wallpaperClass(session.wallpaper)" :style="{ backgroundColor: 'transparent' }">
    <div class="w12-login-user"></div>
    <div class="w12-login-name">{{ displayName }}</div>

    <template v-if="stage === 'form'">
      <input class="w12-login-pwd" type="password" placeholder="密码" v-model="password" :disabled="loading"
        @keyup.enter="doLogin" ref="pwdEl" />
      <div class="w12-login-err">{{ errMsg }}</div>
      <button class="w12-login-btn" :disabled="loading" @click="doLogin">{{ loading ? '登录中' : '登录' }}</button>
    </template>

    <div class="w12-login-welc" :class="{ on: stage === 'welcome' }">
      <svg width="50" height="50" viewBox="0 0 16 16">
        <circle cx="8px" cy="8px" r="6px" style="stroke: #ffffff40; fill: none; stroke-width: 2.5px;"></circle>
        <circle cx="8px" cy="8px" r="6px"></circle>
      </svg>
      <p>欢迎，{{ displayName }}</p>
    </div>

    <div class="w12-login-notice" v-if="session.site.announcement" v-html="noticeHtml"></div>
    <!-- 演示文档入口（管理台站点设置填入演示分享 token 后显示） -->
    <div class="w12-login-demo" v-if="session.site.demoShare">
      <a :href="'#/s/' + session.site.demoShare">体验在线文档</a>
    </div>
    <div class="w12-login-site">{{ session.site.siteName }} · CloudPan</div>

    <div class="w12-login-power">
      <button title="关机" @click="shutdown">
        <svg width="22" height="22" viewBox="0 0 16 16"><path d="M8 1.8v5.4" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/><path d="M4.9 4a5.2 5.2 0 1 0 6.2 0" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/></svg>
      </button>
      <button title="重启" @click="reboot">
        <svg width="22" height="22" viewBox="0 0 16 16"><path d="M3.6 5.4A5 5 0 1 1 3.2 9" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/><path d="M3.4 2.4v3.2h3.2" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useSession } from '../../stores/session'
import { wallpaperClass } from '../../assets/wallpapers'
import { setToken } from '../../api/http'
import { renderMD } from '../../utils/markdown'

const router = useRouter()
const session = useSession()
// 单用户私有部署：系统只有管理员一个账号，登录页不再暴露用户名（固定用此账号名提交）
const ADMIN_USER = 'admin'
// 公告支持 Markdown（管理台站点设置编写，DOMPurify 消毒后渲染）
const noticeHtml = computed(() => renderMD(session.site.announcement || ''))
const password = ref('')
const loading = ref(false)
const stage = ref<'form' | 'welcome'>('form')
const errMsg = ref('')
const pwdEl = ref<HTMLInputElement>()

const displayName = computed(() => session.user?.nickname || session.user?.username || '管理员')

onMounted(() => {
  session.loadSite()
  nextTick(() => pwdEl.value?.focus())
})

// 登录成功后的统一收尾：欢迎动画 → 压暗 → 淡出进桌面
function enterDesktop() {
  stage.value = 'welcome'
  // 演示站时序：欢迎 2s → .close 压暗 500ms → 淡出（1.5s）→ 桌面
  setTimeout(() => {
    const el = document.querySelector('.w12-login') as HTMLElement | null
    el?.classList.add('close')
    setTimeout(() => {
      if (el) el.style.opacity = '0'
      setTimeout(() => router.replace('/desktop'), 900)
    }, 550)
  }, 1400)
}

async function doLogin() {
  if (!password.value || loading.value) return
  loading.value = true
  errMsg.value = ''
  try {
    const res = await import('../../api/modules').then(m => m.authApi.login(ADMIN_USER, password.value))
    setToken(res.token, res.refreshToken)
    await session.loadMe()
    enterDesktop()
  } catch (e: any) {
    errMsg.value = e.message
    password.value = ''
  } finally {
    loading.value = false
  }
}

function shutdown() { router.replace('/boot') }
function reboot() { location.reload() }
</script>

<style scoped>
.w12-login-demo {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  bottom: 128px;
  text-align: center;
}
.w12-login-demo a {
  display: inline-block;
  color: #ffffffb8;
  font-size: 12.5px;
  padding: 6px 14px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.09);
  backdrop-filter: blur(8px);
  cursor: pointer;
  user-select: none;
  transition: background 150ms, color 150ms;
  text-decoration: none;
}
.w12-login-demo a:hover {
  background: rgba(255, 255, 255, 0.18);
  color: #fff;
}
.w12-login-switch {
  margin-top: 14px;
  text-align: center;
}
.w12-login-switch a {
  color: #ffffffb0;
  font-size: 12.5px;
  cursor: pointer;
  user-select: none;
  transition: color 150ms;
}
.w12-login-switch a:hover {
  color: #ffffff;
  text-decoration: underline;
}
</style>
