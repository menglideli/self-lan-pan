<template>
  <div v-if="show" class="ap-mask" @click.self="close">
    <div class="dialog ap-box">
      <h3>{{ title }}</h3>
      <div class="ap-hint">
        本机有多张网卡，选一个「对方能访问到」的地址。带「当前」标记的就是你此刻打开本页用的那一个。
      </div>
      <div class="ap-list">
        <div v-for="a in list" :key="a.ip" class="ap-row" @click="pick(a)">
          <div class="ap-iface">
            <span class="ap-name">{{ a.iface }}</span>
            <span v-if="isCurrent(a)" class="ap-tag cur">当前</span>
            <span v-else-if="a.kind === 'virtual'" class="ap-tag vir">虚拟网卡</span>
          </div>
          <div class="ap-url">{{ a.url }}{{ path }}</div>
        </div>
        <div v-if="!list.length" class="ap-empty">
          没有枚举到可用地址（可能网卡未连接），请手动填写。
        </div>
      </div>
      <div class="actions">
        <button class="btn" @click="close">取消</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { loadAddresses, withCurrentFirst, isCurrent, type LanAddr } from '../utils/lanaddr'
import { copyText } from '../utils/clipboard'
import { useToast } from '../stores/dialog'

const props = withDefaults(defineProps<{
  show: boolean
  title?: string
  // 拼在地址后面的路径，如 '/dav/' 或 '/#/s/token'
  path?: string
}>(), { title: '选择地址', path: '' })

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  (e: 'picked', full: string): void
}>()

const toast = useToast()
const list = ref<LanAddr[]>([])

watch(() => props.show, async v => {
  if (v) list.value = withCurrentFirst(await loadAddresses(true))
})

async function pick(a: LanAddr) {
  const full = a.url + props.path
  const ok = await copyText(full)
  if (ok) toast.success('已复制：' + full)
  else toast.error('复制失败，请手动选择复制')
  emit('picked', full)
  close()
}

function close() { emit('update:show', false) }
</script>

<style scoped>
/* 自己写遮罩而不是复用 .dialog-mask：后者是 position:absolute（相对最近的定位祖先），
   组件被放在滚动容器里时定位会漂；这里固定全屏更稳。 */
.ap-mask {
  position: fixed; inset: 0; z-index: 900; background: rgba(0, 0, 0, 0.35);
  display: flex; align-items: center; justify-content: center;
}
.ap-box { width: 540px; max-width: 92%; }
.ap-hint { font-size: 12px; color: var(--text-3); line-height: 1.7; margin-bottom: 10px; }
.ap-list { max-height: 320px; overflow: auto; }
.ap-row {
  padding: 8px 10px; border-radius: 7px; cursor: pointer;
  border-bottom: 1px solid var(--hover-b);
}
.ap-row:last-child { border-bottom: none; }
.ap-row:hover { background: var(--hover-b); }
.ap-iface { display: flex; align-items: center; gap: 6px; font-size: 12.5px; color: var(--text-2); }
.ap-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ap-tag {
  font-size: 10.5px; padding: 1px 6px; border-radius: 999px;
  border: 1px solid var(--stroke); color: var(--text-3); flex: none;
}
.ap-tag.cur { color: var(--theme-2); border-color: color-mix(in srgb, var(--theme-2) 45%, transparent); }
.ap-url { font-size: 12.5px; color: var(--text); margin-top: 3px; word-break: break-all; }
.ap-empty { color: var(--text-3); font-size: 12.5px; padding: 14px 4px; }
</style>
