<template>
  <div class="app-root">
    <div style="display: flex; flex: 1; overflow: hidden">
      <!-- 侧栏（win12 设置风格） -->
      <div class="ac-side">
        <div class="ac-user">
          <div class="mini-avatar">{{ (session.user?.nickname || 'A').charAt(0) }}</div>
          <div>
            <div style="font-size: 13px; font-weight: 600">{{ session.user?.nickname }}</div>
            <div style="font-size: 11px; color: var(--text-3)">管理员 · {{ session.site.siteName }}</div>
          </div>
        </div>
        <div style="padding: 4px">
          <div v-for="t in tabs" :key="t.id" class="ac-nav" :class="{ active: tab === t.id }" @click="switchTab(t.id)">
            <AppIcon :name="t.icon" :size="17" />
            <span style="flex: 1">{{ t.name }}</span>
            <span v-if="t.badge" class="ac-badge">{{ t.badge }}</span>
          </div>
        </div>
      </div>

      <!-- 内容区 -->
      <div class="ac-content">
        <!-- 仪表盘 -->
        <template v-if="tab === 'dash'">
          <h2 class="ac-h2">仪表盘</h2>
          <div class="ac-cards">
            <div class="ac-card ac-stat">
              <div class="ac-stat-num">{{ dash.users ?? '-' }}</div>
              <div class="ac-stat-lbl">用户总数</div>
            </div>
            <div class="ac-card ac-stat">
              <div class="ac-stat-num">{{ dash.policies ?? '-' }}</div>
              <div class="ac-stat-lbl">已挂载存储</div>
            </div>
            <div class="ac-card ac-stat">
              <div class="ac-stat-num">{{ dash.shares ?? '-' }}</div>
              <div class="ac-stat-lbl">分享总数</div>
            </div>
            <div class="ac-card ac-stat">
              <div class="ac-stat-num">{{ fmtGB(dash.usedBytes) }}</div>
              <div class="ac-stat-lbl">全站用量</div>
            </div>
          </div>
          <!-- 系统资源（实时采样，3s 轮询） -->
          <div class="ac-card sys-card" style="margin-top: 14px; padding: 16px 20px">
            <div class="ac-row-title">
              系统资源
              <span class="sys-live" v-if="sys && !sysMsg">实时</span>
              <span class="sys-live off" v-else-if="sysMsg">已停用</span>
            </div>
            <div v-if="sysMsg" class="sys-muted">{{ sysMsg }}</div>
            <div v-else-if="!sys" class="sys-muted">正在采样…</div>
            <div v-else class="sys-grid">
              <div class="sys-col">
                <div class="sys-block">
                  <div class="sys-line"><span class="sys-name">CPU</span><span class="sys-val">{{ sys.cpuPct }}%</span></div>
                  <div class="ac-progress"><div class="ac-progress-fill" :style="{ width: Math.min(100, sys.cpuPct) + '%' }"></div></div>
                  <div class="sys-sub">{{ sys.model || '处理器' }} · {{ sys.cores }} 核</div>
                  <div class="sys-cores" v-if="sys.perCore?.length">
                    <div v-for="(p, i) in sys.perCore.slice(0, 16)" :key="i" class="sys-core" :title="'核心 ' + (i + 1) + ': ' + p + '%'">
                      <div class="sys-core-fill" :style="{ height: Math.max(5, Math.min(100, p)) + '%' }"></div>
                    </div>
                    <span v-if="sys.perCore.length > 16" class="sys-core-more">×{{ sys.perCore.length }}</span>
                  </div>
                </div>
                <div class="sys-block">
                  <div class="sys-line"><span class="sys-name">内存</span><span class="sys-val">{{ fmt(sys.memUsed) }} / {{ fmt(sys.memTotal) }} · {{ sys.memPct }}%</span></div>
                  <div class="ac-progress"><div class="ac-progress-fill" :style="{ width: Math.min(100, sys.memPct) + '%' }"></div></div>
                </div>
                <div class="sys-block">
                  <div class="ac-kv"><span>主机名</span><b>{{ sys.hostname }}</b></div>
                  <div class="ac-kv"><span>系统</span><b>{{ sys.platform }} {{ sys.kernel }}</b></div>
                  <div class="ac-kv"><span>运行时长</span><b>{{ fmtUptime(sys.uptime) }}</b></div>
                </div>
              </div>
              <div class="sys-col">
                <div class="sys-block">
                  <div class="sys-line"><span class="sys-name">磁盘</span></div>
                  <div v-for="d in sys.disks" :key="d.mount" class="sys-disk">
                    <div class="sys-disk-head">
                      <span class="sys-disk-name">{{ d.mount }}{{ d.label ? ' · ' + d.label : '' }}</span>
                      <span class="sys-disk-num">{{ fmt(d.used) }} / {{ fmt(d.total) }} · {{ d.pct }}%</span>
                    </div>
                    <div class="ac-progress"><div class="ac-progress-fill" :class="{ warn: d.pct > 85 }" :style="{ width: Math.max(2, Math.min(100, d.pct)) + '%' }"></div></div>
                  </div>
                  <div v-if="!sys.disks.length" class="sys-muted">未检测到磁盘</div>
                </div>
                <div class="sys-block">
                  <div class="sys-line"><span class="sys-name">网络</span></div>
                  <div v-for="n in sys.nets" :key="n.name" class="sys-net">
                    <span class="sys-net-name">{{ n.name }}</span>
                    <span class="sys-rx">↓ {{ fmtRate(n.rxRate) }}</span>
                    <span class="sys-tx">↑ {{ fmtRate(n.txRate) }}</span>
                  </div>
                  <div v-if="!sys.nets.length" class="sys-muted">暂无活动的网络接口</div>
                </div>
              </div>
            </div>
          </div>

          <div class="ac-card" style="margin-top: 14px; padding: 16px 20px">
            <div class="ac-row-title">存储概览</div>
            <div v-for="p in policies" :key="p.id" style="margin: 12px 0">
              <div style="display: flex; justify-content: space-between; font-size: 12.5px; margin-bottom: 6px">
                <span style="font-weight: 600">{{ p.name }}</span>
                <span style="color: var(--text-3)">{{ fmt(p.usageBytes) }} 已用 · {{ typeName(p.type) }}</span>
              </div>
              <div class="ac-progress"><div class="ac-progress-fill" :style="{ width: Math.max(3, Math.min(100, p.usageBytes / ((1<<30)*10) * 100)) + '%' }"></div></div>
            </div>
            <div v-if="!policies.length" style="color: var(--text-3); font-size: 12.5px">尚未挂载存储</div>
          </div>
        </template>

        <!-- 单用户私有部署：用户管理与用户组管理已整体移除（只有管理员一个账号） -->

        <!-- 存储策略 -->
        <template v-else-if="tab === 'policies'">
          <div class="ac-head">
            <h2 class="ac-h2">挂载管理</h2>
            <button class="btn primary" @click="newPolicy"><AppIcon name="plus" :size="15" />挂载存储</button>
          </div>
          <div class="ac-card" style="padding: 4px 0">
            <table class="ac-table">
              <thead><tr><th>名称</th><th>类型</th><th>根目录 / 状态</th><th>用量</th><th style="width: 240px">操作</th></tr></thead>
              <tbody>
                <tr v-for="p in policies" :key="p.id">
                  <td class="ac-strong">{{ p.name }}</td>
                  <td><span class="ac-tag" :class="{ admin: p.type !== 'local' }">{{ typeName(p.type) }}</span></td>
                  <td>
                    <div style="font-size: 12px; max-width: 240px; overflow: hidden; text-overflow: ellipsis" :title="p.rootPath || ''">{{ p.rootPath || p.statusMsg || '-' }}</div>
                    <div class="ac-status" :class="p.status"><span class="ac-dot" :class="{ off: p.status !== 'active' }"></span>{{ p.status === 'active' ? '正常' : p.status === 'disabled' ? '已停用' : '异常' }}</div>
                  </td>
                  <td>{{ fmt(p.usageBytes) }}</td>
                  <td>
                    <button class="btn" style="padding: 4px 10px" @click="editPolicyRow(p)">编辑</button>
                    <button class="btn" style="padding: 4px 10px" @click="checkPolicy(p)">测通</button>
                    <button class="btn" style="padding: 4px 10px" @click="togglePolicy(p)">{{ p.status === 'disabled' ? '启用' : '停用' }}</button>
                    <button class="btn danger" style="padding: 4px 10px" @click="delPolicy(p)">卸载</button>
                  </td>
                </tr>
              </tbody>
            </table>
            <div v-if="!policies.length" style="padding: 24px; text-align: center; color: var(--text-3); font-size: 13px">点击右上角「挂载存储」把本机目录或云盘加进网盘（也可在文件管理器里点「挂载文件夹」）</div>
          </div>
        </template>

        <!-- 分享审计 -->
        <template v-else-if="tab === 'shares'">
          <div class="ac-head"><h2 class="ac-h2">分享审计</h2></div>
          <div class="ac-card" style="padding: 4px 0">
            <table class="ac-table">
              <thead><tr><th>文件</th><th>分享者</th><th>提取码</th><th>浏览/下载</th><th>到期</th><th style="width: 90px">操作</th></tr></thead>
              <tbody>
                <tr v-for="s in allShares" :key="s.id">
                  <td class="ac-strong" style="max-width: 220px; overflow: hidden; text-overflow: ellipsis">{{ s.name }}{{ s.isDir ? ' (目录)' : '' }}</td>
                  <td>{{ s.owner }} ({{ s.ownerName }})</td>
                  <td>{{ s.hasPassword ? '有' : '无' }}</td>
                  <td>{{ s.views }} / {{ s.downloads }}</td>
                  <td>{{ s.expiresAt ? new Date(s.expiresAt).toLocaleDateString() : '永久' }}</td>
                  <td><button class="btn danger" style="padding: 4px 10px" @click="adminDelShare(s)">取消</button></td>
                </tr>
              </tbody>
            </table>
            <div v-if="!allShares.length" style="padding: 24px; text-align: center; color: var(--text-3); font-size: 13px">暂无分享</div>
          </div>
        </template>

        <!-- 任务监控 -->
        <template v-else-if="tab === 'tasks'">
          <div class="ac-head"><h2 class="ac-h2">任务队列监控</h2></div>
          <div class="ac-card" style="padding: 4px 0">
            <table class="ac-table">
              <thead><tr><th>ID</th><th>类型</th><th>状态</th><th>进度</th><th>时间</th><th>错误</th></tr></thead>
              <tbody>
                <tr v-for="t in tasks" :key="t.id">
                  <td>#{{ t.id }}</td>
                  <td><span class="ac-tag">{{ taskTypeName(t.type) }}</span></td>
                  <td><span class="ac-status" :class="t.status === 'finished' ? 'active' : t.status"><span class="ac-dot" :class="{ off: t.status === 'error' || t.status === 'canceled' }"></span>{{ taskStatusName(t.status) }}</span></td>
                  <td style="width: 160px"><div class="ac-progress"><div class="ac-progress-fill" :style="{ width: t.progress + '%' }"></div></div></td>
                  <td style="color: var(--text-3)">{{ new Date(t.createdAt).toLocaleString() }}</td>
                  <td style="color: var(--danger); font-size: 12px; max-width: 200px; overflow: hidden; text-overflow: ellipsis">{{ t.error }}</td>
                </tr>
              </tbody>
            </table>
            <div v-if="!tasks.length" style="padding: 24px; text-align: center; color: var(--text-3); font-size: 13px">暂无后台任务</div>
          </div>
        </template>

        <!-- 站点设置 -->
        <template v-else-if="tab === 'settings'">
          <h2 class="ac-h2">站点设置</h2>
          <div class="ac-card" style="padding: 20px 24px">
            <div class="ac-set-title">基础</div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>站点名称</b><span>显示在登录页与开始菜单</span></div>
              <input class="input" v-model="settings.site_name" style="width: 260px" />
            </div>
            <div class="ac-set-row" style="flex-direction: column; align-items: flex-start; gap: 6px">
              <div class="ac-set-lbl"><b>公告</b><span>登录页展示，支持 Markdown（标题/加粗/链接/列表/图片等），留空则不显示</span></div>
              <textarea class="input" v-model="settings.announcement" rows="5" style="width: 100%; max-width: 560px; resize: vertical; line-height: 1.6"
                placeholder="支持 Markdown，例如：&#10;**系统升级通知**&#10;- 今晚 22:00 维护&#10;- 详情见 [公告](https://example.com)"></textarea>
            </div>
            <div class="ac-set-row" style="flex-direction: column; align-items: flex-start; gap: 6px">
              <div class="ac-set-lbl"><b>演示文档分享</b><span>填一个分享链接的 token（如 abc123），登录页显示「体验在线文档」入口；分享不存在或过期时自动隐藏。建议分享一个 Markdown 演示文件</span></div>
              <input class="input" v-model="settings.demo_share" style="width: 300px" placeholder="分享 token，如 abc123；留空 = 不显示入口" />
            </div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>站点主题</b><span>单用户私有部署只保留 Windows 12 概念风格</span></div>
              <select class="input" v-model="settings.site_theme" style="width: 260px" disabled>
                <option value="win12">Windows 12 概念版</option>
              </select>
            </div>
            <div class="ac-set-sep"></div>
            <div class="ac-set-title">登录</div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>独立应用模式</b><span>允许通过 #/app/&lt;应用ID&gt; 链接直接全屏打开单个应用（如 #/app/explorer），未登录时显示极简登录；关闭后访问显示拦截页</span></div>
              <label class="ac-switch"><input type="checkbox" :checked="settings.standalone_apps !== 'false'" @change="settings.standalone_apps = ($event.target as HTMLInputElement).checked ? 'true' : 'false'" /><span class="ac-slider"></span></label>
            </div>
            <div class="ac-set-sep"></div>
            <div class="ac-set-title">WebDAV</div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>启用 WebDAV</b><span>允许用户把网盘挂载进 Windows 资源管理器</span></div>
              <label class="ac-switch"><input type="checkbox" :checked="settings.webdav_enabled === 'true'" @change="settings.webdav_enabled = ($event.target as HTMLInputElement).checked ? 'true' : 'false'" /><span class="ac-slider"></span></label>
            </div>
            <div class="ac-set-sep"></div>
            <div class="ac-set-title">通知外发</div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>Webhook 地址</b><span>通知产生时 POST JSON 到该地址（任务/配额/系统）；留空关闭</span></div>
              <input class="input" v-model="settings.notify_webhook_url" style="width: 340px" placeholder="https://example.com/hook" />
            </div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>Webhook Token</b><span>可选，作为 Authorization 请求头发给接收端</span></div>
              <input class="input" v-model="settings.notify_webhook_token" style="width: 340px" placeholder="留空 = 不带" />
            </div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>邮件通知</b><span>通过 SMTP 把通知发到固定邮箱</span></div>
              <label class="ac-switch"><input type="checkbox" :checked="settings.notify_email_enabled === 'true'" @change="settings.notify_email_enabled = ($event.target as HTMLInputElement).checked ? 'true' : 'false'" /><span class="ac-slider"></span></label>
            </div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>SMTP 服务器</b><span>主机与端口（支持 STARTTLS）</span></div>
              <div style="display: flex; gap: 8px">
                <input class="input" v-model="settings.notify_email_host" style="width: 220px" placeholder="smtp.example.com" />
                <input class="input" type="number" v-model="settings.notify_email_port" style="width: 90px" placeholder="465/587" />
              </div>
            </div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>SMTP 账号</b><span>登录账号与密码（授权码）</span></div>
              <div style="display: flex; gap: 8px">
                <input class="input" v-model="settings.notify_email_user" style="width: 220px" placeholder="noreply@example.com" />
                <input class="input" type="password" v-model="settings.notify_email_pass" style="width: 150px" placeholder="密码/授权码" />
              </div>
            </div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>发件 / 收件人</b><span>收件人可用逗号分隔多个</span></div>
              <div style="display: flex; gap: 8px">
                <input class="input" v-model="settings.notify_email_from" style="width: 220px" placeholder="留空 = 同账号" />
                <input class="input" v-model="settings.notify_email_to" style="width: 220px" placeholder="admin@example.com" />
              </div>
            </div>
            <button class="btn primary" style="margin-top: 18px" @click="saveSettings"><AppIcon name="check" :size="15" />保存全部设置</button>
          </div>
        </template>

        <!-- 系统更新 -->
        <template v-else-if="tab === 'update'">
          <div class="ac-card" style="padding: 20px 24px">
            <div class="ac-set-title">当前版本</div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>运行版本</b><span>构建时注入的版本号（dev = 本地构建）</span></div>
              <b style="font-size: 14px">{{ updStatus.current || '…' }}</b>
            </div>
            <div class="ac-set-sep"></div>
            <div class="ac-set-title">更新来源</div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>来源类型</b><span>GitHub 仓库：自动拉取 latest release 并挑选 linux 资产；直链：update_url 直接指向二进制/压缩包</span></div>
              <select class="input" v-model="updMode" style="width: 220px">
                <option value="github">GitHub 仓库</option>
                <option value="direct">直链下载</option>
              </select>
            </div>
            <template v-if="updMode === 'github'">
              <div class="ac-set-row">
                <div class="ac-set-lbl"><b>GitHub 仓库</b><span>owner/repo，默认 johngko/cloudpan</span></div>
                <input class="input" v-model="settings.update_repo" style="width: 300px" placeholder="johngko/cloudpan" />
              </div>
              <div class="ac-set-row">
                <div class="ac-set-lbl"><b>下载代理前缀</b><span>可选，如 https://ghproxy.com 或自建镜像；拼接在资产 URL 之前，留空 = 直连 GitHub</span></div>
                <input class="input" v-model="settings.update_proxy" style="width: 340px" placeholder="留空 = 直连" />
              </div>
            </template>
            <template v-else>
              <div class="ac-set-row">
                <div class="ac-set-lbl"><b>下载地址</b><span>二进制或 .tar.gz/.zip 包（包内需含 cloudpan 可执行文件）</span></div>
                <input class="input" v-model="settings.update_url" style="width: 420px" placeholder="https://example.com/cloudpan-linux-amd64" />
              </div>
              <div class="ac-set-row">
                <div class="ac-set-lbl"><b>版本号</b><span>可选，用于显示与「是否有更新」判断</span></div>
                <input class="input" v-model="settings.update_version" style="width: 200px" placeholder="如 1.2.0" />
              </div>
            </template>
            <div style="margin-top: 14px">
              <button class="btn" @click="saveUpdSource"><AppIcon name="check" :size="14" />保存来源配置</button>
            </div>
            <div class="ac-set-sep"></div>
            <div class="ac-set-title">检查与更新</div>
            <div class="ac-set-row" style="align-items: flex-start">
              <div class="ac-set-lbl"><b>最新可用版本</b><span>点击「检查更新」查询</span></div>
              <div style="display: flex; flex-direction: column; gap: 8px; align-items: flex-start">
                <div style="display: flex; gap: 8px; align-items: center">
                  <button class="btn" :disabled="updChecking" @click="checkUpdate">{{ updChecking ? '检查中…' : '检查更新' }}</button>
                  <button v-if="updInfo.hasUpdate" class="btn primary" :disabled="updBusy" @click="startUpdate">
                    <AppIcon name="download" :size="14" />{{ updBusy ? '更新中…' : '下载并更新' }}
                  </button>
                  <span v-if="updInfo.checkedAt && !updInfo.hasUpdate && !updBusy" style="font-size: 12px; color: #7ee2a8">已是最新版本</span>
                </div>
                <div v-if="updInfo.latest" style="font-size: 12.5px; color: var(--text-2)">
                  最新 <b>{{ updInfo.latest }}</b>（当前 {{ updInfo.current }}）
                  <span v-if="updInfo.release?.size"> · {{ fmtBytes(updInfo.release.size) }}</span>
                </div>
                <div v-if="updInfo.release?.notes" class="upd-notes">{{ updInfo.release.notes.slice(0, 400) }}{{ updInfo.release.notes.length > 400 ? '…' : '' }}</div>
                <!-- 更新进度 -->
                <div v-if="updBusy" style="width: 340px">
                  <div style="height: 8px; border-radius: 4px; background: rgba(127,127,127,.18); overflow: hidden">
                    <div style="height: 100%; background: var(--theme-1, #3b91d8); transition: width 300ms"
                      :style="{ width: (updStatus.status === 'replacing' ? 100 : updStatus.progress) + '%' }"></div>
                  </div>
                  <div style="font-size: 12px; color: var(--text-3); margin-top: 5px">
                    {{ updMsgText }} · {{ fmtBytes(updStatus.received) }}{{ updStatus.total ? ' / ' + fmtBytes(updStatus.total) : '' }}
                    <span v-if="updStatus.speed"> · {{ fmtRate(updStatus.speed) }}</span>
                  </div>
                </div>
                <div v-if="updStatus.status === 'error'" style="font-size: 12.5px; color: #ff8a80">{{ updStatus.msg }}</div>
              </div>
            </div>
            <div class="ac-set-sep"></div>
            <div class="ac-set-title">更新记录</div>
            <table v-if="updHistory.length" class="file-list" style="width: 100%; border-collapse: collapse; font-size: 12.5px">
              <tr v-for="h in updHistory" :key="h.id">
                <td style="padding: 7px 10px">{{ new Date(h.createdAt).toLocaleString() }}</td>
                <td style="padding: 7px 10px">{{ h.from || '?' }} → <b>{{ h.to }}</b></td>
                <td style="padding: 7px 10px">{{ h.status === 'success' ? '✅ 成功' : '❌ ' + (h.error || '失败') }}</td>
              </tr>
            </table>
            <div v-else style="font-size: 12.5px; color: var(--text-3); padding: 8px 2px">暂无更新记录</div>
          </div>
        </template>

        <!-- 审计日志 -->
        <template v-else-if="tab === 'logs'">
          <div class="ac-head">
            <h2 class="ac-h2">审计日志</h2>
            <div style="display: flex; gap: 8px; align-items: center">
              <input class="input" v-model="logKw" style="width: 160px" placeholder="搜索用户/动作/详情" @keyup.enter="logPage = 1; loadLogs()" />
              <button class="btn" @click="logPage = 1; loadLogs()">查询</button>
              <button class="btn" @click="exportLogs"><AppIcon name="download" :size="14" />导出 CSV</button>
              <button class="btn" :disabled="logPage <= 1" @click="logPage--; loadLogs()">上一页</button>
              <span style="font-size: 12px; color: var(--text-3)">第 {{ logPage }} 页 / 共 {{ Math.ceil(logTotal / 20) || 1 }} 页</span>
              <button class="btn" :disabled="logPage >= Math.ceil(logTotal / 20)" @click="logPage++; loadLogs()">下一页</button>
            </div>
          </div>
          <div class="ac-card" style="padding: 4px 0">
            <table class="ac-table">
              <thead><tr><th>时间</th><th>用户</th><th>动作</th><th>详情</th><th>IP</th></tr></thead>
              <tbody>
                <tr v-for="l in logs" :key="l.id">
                  <td style="color: var(--text-3)">{{ new Date(l.createdAt).toLocaleString() }}</td>
                  <td class="ac-strong">{{ l.username }}</td>
                  <td><span class="ac-tag">{{ l.action }}</span></td>
                  <td style="color: var(--text-2)">{{ l.detail }}</td>
                  <td style="color: var(--text-3)">{{ l.ip }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

        <!-- 通知记录（含软清除，审计用） -->
        <template v-else-if="tab === 'notify'">
          <div class="ac-head">
            <h2 class="ac-h2">通知记录</h2>
            <div style="display: flex; gap: 8px; align-items: center">
              <input class="input" v-model="notifKw" style="width: 180px" placeholder="搜索用户/类型/标题/内容" @keyup.enter="notifPage = 1; loadNotif()" />
              <button class="btn" @click="notifPage = 1; loadNotif()">查询</button>
              <label style="font-size: 12.5px; color: var(--text-2); display: flex; align-items: center; gap: 5px; cursor: pointer">
                <input type="checkbox" v-model="notifClearedOnly" @change="notifPage = 1; loadNotif()" />只看已清除
              </label>
              <button class="btn" :disabled="notifPage <= 1" @click="notifPage--; loadNotif()">上一页</button>
              <span style="font-size: 12px; color: var(--text-3)">第 {{ notifPage }} 页 / 共 {{ Math.ceil(notifTotal / 20) || 1 }} 页</span>
              <button class="btn" :disabled="notifPage >= Math.ceil(notifTotal / 20)" @click="notifPage++; loadNotif()">下一页</button>
            </div>
          </div>
          <div class="ac-card" style="padding: 4px 0">
            <table class="ac-table">
              <thead><tr><th>时间</th><th>用户</th><th>类型</th><th>标题</th><th>内容</th><th>状态</th></tr></thead>
              <tbody>
                <tr v-for="n in notifList" :key="n.id">
                  <td style="color: var(--text-3)">{{ new Date(n.createdAt).toLocaleString() }}</td>
                  <td class="ac-strong">{{ n.username || n.userId }}</td>
                  <td><span class="ac-tag">{{ notifTypeName(n.type) }}</span></td>
                  <td>{{ n.title }}</td>
                  <td style="color: var(--text-2); max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="n.content">{{ n.content }}</td>
                  <td>
                    <span v-if="n.cleared" class="ac-tag" style="color: var(--danger, #e05252)">已清除</span>
                    <span v-else-if="n.read" class="ac-tag">已读</span>
                    <span v-else class="ac-tag" style="color: var(--theme-2)">未读</span>
                  </td>
                </tr>
                <tr v-if="!notifList.length"><td colspan="6" style="text-align: center; color: var(--text-3); padding: 24px">暂无通知记录</td></tr>
              </tbody>
            </table>
          </div>
        </template>
      </div>
    </div>

    <!-- 存储策略对话框 -->
    <div class="dialog-mask" v-if="policyShow" @click.self="closePolicy">
      <div class="dialog" style="width: 480px">
        <h3>{{ editPolicy.id ? '编辑' : '挂载' }}存储</h3>
        <div class="row">
          <label>类型</label>
          <select class="input" v-model="editPolicy.type" style="width: 100%">
            <option value="local">本地磁盘目录</option>
            <option value="pan123">123云盘（官方开放平台）</option>
            <option value="aliyun">阿里云盘（开放平台）</option>
            <option value="baidu">百度网盘（官方 API，默认只读）</option>
            <option value="tianyi">天翼云盘（实验性，粘贴 Cookie）</option>
          </select>
        </div>
        <div class="row"><label>显示名称</label><input class="input" v-model="editPolicy.name" placeholder="如：电影 / 我的云盘" style="width: 100%" /></div>
        <div class="row" v-if="editPolicy.type === 'local'">
          <label>本机目录</label>
          <input class="input" v-model="editPolicy.rootPath" placeholder="如 E:\媒体\电影（也可在文件管理器里点「挂载文件夹」用选择器挑）" style="width: 100%" />
        </div>
        <template v-if="editPolicy.type === 'pan123'">
          <div class="row"><label>ClientID（123 开放平台）</label><input class="input" v-model="pOpts.client_id" style="width: 100%" /></div>
          <div class="row"><label>ClientSecret</label><input class="input" v-model="pOpts.client_secret" style="width: 100%" /></div>
        </template>
        <template v-else-if="editPolicy.type === 'aliyun'">
          <div class="row"><label>ClientID（开放平台应用 ID）</label><input class="input" v-model="pOpts.client_id" style="width: 100%" /></div>
          <div class="row"><label>ClientSecret</label><input class="input" v-model="pOpts.client_secret" style="width: 100%" /></div>
          <div class="row">
            <label>refresh_token</label>
            <div style="display: flex; gap: 8px"><input class="input" v-model="pOpts.refresh_token" placeholder="扫码授权后自动填入，也可手动粘贴" style="flex: 1" /><button class="btn" @click="startAuth()">扫码授权</button></div>
          </div>
        </template>
        <template v-else-if="editPolicy.type === 'baidu'">
          <div class="row"><label>AppKey</label><input class="input" v-model="pOpts.client_id" style="width: 100%" /></div>
          <div class="row"><label>SecretKey</label><input class="input" v-model="pOpts.client_secret" style="width: 100%" /></div>
          <div class="row">
            <label>access_token</label>
            <div style="display: flex; gap: 8px"><input class="input" v-model="pOpts.access_token" placeholder="扫码授权后自动填入，也可手动粘贴" style="flex: 1" /><button class="btn" @click="startAuth()">扫码授权</button></div>
          </div>
        </template>
        <template v-else-if="editPolicy.type === 'tianyi'">
          <div class="row"><label>网页版 Cookie（实验性）</label><input class="input" v-model="pOpts.cookie" style="width: 100%" /></div>
        </template>
        <!-- 扫码授权面板：二维码 = 厂商授权页地址（带签名 state），手机扫码→授权→厂商重定向回 /api/cloud/callback 自动完成 -->
        <div v-if="qrImg" class="qr-panel">
          <img :src="qrImg" alt="授权二维码" />
          <div class="qr-actions">
            <a :href="qrUrl" target="_blank" rel="noopener">手机不便？点此在电脑浏览器打开授权页</a>
            <button class="btn" style="padding: 2px 10px" @click="stopQr">取消</button>
          </div>
          <div class="qr-actions" style="justify-content: center">
            <a href="#" @click.prevent="startAuth(true)">手机无法访问本站？改用「粘贴授权码」模式</a>
          </div>
        </div>
        <div v-if="authMsg" style="font-size: 12px; color: var(--theme-2); margin-bottom: 10px; word-break: break-all">{{ authMsg }}</div>
        <div class="ac-guide">
          <template v-if="editPolicy.type === 'pan123'">
            <b>123云盘开通步骤：</b>
            ① 访问 <a href="https://www.123pan.com/developer" target="_blank">123 开放平台</a> 注册开发者
            ② 创建应用获取 ClientID / ClientSecret 填入上方
            ③ 挂载后点「测通」验证连通
          </template>
          <template v-else-if="editPolicy.type === 'aliyun'">
            <b>阿里云盘挂载教程：</b>
            ① 前往 <a href="https://open.alipan.com" target="_blank">阿里云盘开放平台</a> 注册开发者并创建应用，获得 ClientID / ClientSecret 填入上方
            ② 确认「站点设置 → 公开地址」为手机可访问的地址（纯内网部署填内网地址即可，手机需与服务器同网段）
            ③ 点「扫码授权」→ 用阿里云盘 App / 手机浏览器扫二维码 → 登录并确认授权 → 手机显示绑定成功，本页 token 自动填入
            ④ 保存后点列表「测通」验证连通
          </template>
          <template v-else-if="editPolicy.type === 'baidu'">
            <b>百度网盘挂载教程：</b>
            ① 前往 <a href="https://console.bce.baidu.com" target="_blank">百度智能云控制台</a>（开放平台）创建应用，获得 AppKey / SecretKey 填入上方
            ② 在应用的「回调地址」里登记 <code>本站公开地址 + /api/cloud/callback</code>（与站点设置一致，百度要求回调地址预先登记）
            ③ 点「扫码授权」→ 手机扫码登录百度账号并确认授权 → 绑定成功自动回填
            ④ 上传/写操作需申请接口白名单，未获批前为只读挂载（查看/下载不受限）
          </template>
          <template v-else-if="editPolicy.type === 'tianyi'">
            <b>天翼云盘（实验性）：</b>
            电脑浏览器登录天翼云盘网页版 → F12 复制 Cookie 整串填入上方。接口为社区逆向，随时可能失效。
          </template>
        </div>
        <div class="actions">
          <button class="btn" @click="closePolicy">取消</button>
          <button class="btn primary" @click="createPolicy">{{ editPolicy.id ? '保存' : '挂载' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useSession } from '../stores/session'
import { useAppState } from '../stores/appstate'
import { useUiDialog, useToast } from '../stores/dialog'
import { adminApi } from '../api/modules'
import AppIcon from '../components/AppIcon.vue'
import QRCode from 'qrcode'

const session = useSession()
const uiDlg = useUiDialog()
const toast = useToast()
const tab = ref('dash')
const tabs = computed(() => [
  { id: 'dash', name: '仪表盘', icon: 'info' },
  { id: 'policies', name: '存储策略', icon: 'drive' },
  { id: 'shares', name: '分享审计', icon: 'share' },
  { id: 'tasks', name: '任务监控', icon: 'list' },
  { id: 'settings', name: '站点设置', icon: 'settings' },
  { id: 'update', name: '系统更新', icon: 'refresh' },
  { id: 'logs', name: '审计日志', icon: 'edit' },
  { id: 'notify', name: '通知记录', icon: 'bell' }
])

const dash = ref<any>({})
// 系统资源监控（3s 轮询，仅仪表盘页激活时）
const appstate = useAppState()
const sys = ref<any>(null)
const sysMsg = ref('')
let sysTimer: number | undefined
const sysOn = computed(() => appstate.isOn('system_monitor'))
async function loadSys() {
  if (!sysOn.value) { sys.value = null; sysMsg.value = '系统监控已被管理员停用（应用中心可开启）'; return }
  try {
    sys.value = await adminApi.system()
    sysMsg.value = ''
  } catch (e: any) {
    if (e?.code === 403 || /停用/.test(e?.message || '')) { sys.value = null; sysMsg.value = '系统监控已被管理员停用' }
    else sys.value = null
  }
}
function startSysPoll() {
  stopSysPoll()
  loadSys()
  sysTimer = window.setInterval(loadSys, 3000)
}
function stopSysPoll() {
  if (sysTimer) { clearInterval(sysTimer); sysTimer = undefined }
}
function fmtRate(n: number) {
  if (!n) return '0 B/s'
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB/s'
  if (n > 1024) return (n / 1024).toFixed(1) + ' KB/s'
  return n + ' B/s'
}
function fmtUptime(s: number) {
  if (!s) return '-'
  const d = Math.floor(s / 86400), h = Math.floor(s % 86400 / 3600), m = Math.floor(s % 3600 / 60)
  return (d ? d + '天' : '') + h + '时' + m + '分'
}
function fmtBytes(n: number) {
  if (!n) return '0 B'
  if (n > 1 << 30) return (n / (1 << 30)).toFixed(2) + ' GB'
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  if (n > 1024) return (n / 1024).toFixed(1) + ' KB'
  return n + ' B'
}

// ---- 系统更新（检查/下载/自重启；更新中 2s 轮询状态）----
const updStatus = ref<any>({ current: '', status: 'idle', progress: 0, received: 0, total: 0, speed: 0, msg: '' })
const updInfo = ref<any>({})
const updHistory = ref<any[]>([])
const updChecking = ref(false)
let updTimer: number | undefined
const updBusy = computed(() => ['downloading', 'replacing'].includes(updStatus.value.status))
const updMsgText = computed(() => {
  const m = updStatus.value.msg || ''
  if (updStatus.value.status === 'downloading') return m || '下载中…'
  if (updStatus.value.status === 'replacing') return '重启中，页面几秒后自动恢复…'
  return m
})
const updMode = computed({
  get: () => (settings.value.update_url ? 'direct' : 'github'),
  set: (m: string) => {
    if (m === 'direct' && !settings.value.update_url) settings.value.update_url = ''
    if (m === 'github') settings.value.update_url = ''
  }
})
async function loadUpdStatus() {
  try { updStatus.value = await adminApi.updateStatus() } catch { /* ignore */ }
}
async function loadUpdHistory() {
  try { updHistory.value = await adminApi.updateHistory() } catch { /* ignore */ }
}
async function checkUpdate() {
  updChecking.value = true
  try {
    updInfo.value = await adminApi.updateCheck()
  } catch (e: any) {
    toast.error('检查更新失败：' + (e.message || ''))
  } finally {
    updChecking.value = false
  }
}
async function startUpdate() {
  if (!confirm('将下载并替换当前运行的二进制（自动备份旧版本后重启）。继续？')) return
  try {
    await adminApi.updateStart()
    startUpdPoll()
  } catch (e: any) {
    toast.error('启动更新失败：' + (e.message || ''))
  }
}
function startUpdPoll() {
  stopUpdPoll()
  loadUpdStatus()
  updTimer = window.setInterval(async () => {
    await loadUpdStatus()
    // 重启完成（状态回到 idle 且版本号变化）或出错 → 停止轮询
    if (['idle', 'error'].includes(updStatus.value.status)) {
      stopUpdPoll()
      loadUpdHistory()
      if (updStatus.value.status === 'idle') checkUpdate()
    }
  }, 2000)
}
function stopUpdPoll() {
  if (updTimer) { clearInterval(updTimer); updTimer = undefined }
}
async function saveUpdSource() {
  try {
    await adminApi.settingsSet({
      update_repo: settings.value.update_repo || 'johngko/cloudpan',
      update_proxy: settings.value.update_proxy || '',
      update_url: settings.value.update_url || '',
      update_version: settings.value.update_version || ''
    })
    toast.success('更新来源已保存')
  } catch (e: any) {
    toast.error('保存失败：' + (e.message || ''))
  }
}
const policies = ref<any[]>([])
const allShares = ref<any[]>([])
const tasks = ref<any[]>([])
const logs = ref<any[]>([])
const logPage = ref(1)
const logTotal = ref(0)
const logKw = ref('')
const notifList = ref<any[]>([])
const notifPage = ref(1)
const notifTotal = ref(0)
const notifKw = ref('')
const notifClearedOnly = ref(false)
const settings = ref<Record<string, string>>({})

const policyShow = ref(false)
const editPolicy = ref<any>({})
const emptyPolicy = { type: 'local', name: '', letter: '', rootPath: '' }
const pOpts = ref<Record<string, string>>({})
const authMsg = ref('')

function switchTab(id: string) {
  tab.value = id
  if (id === 'tasks') loadTasks()
  if (id === 'notify') loadNotif()
  if (id === 'update') {
    loadUpdStatus(); loadUpdHistory()
    loadUpdStatus().then(() => { if (['downloading', 'replacing'].includes(updStatus.value.status)) startUpdPoll() })
  } else stopUpdPoll()
  if (id === 'dash') startSysPoll(); else stopSysPoll()
}

onMounted(() => { loadAll(); startSysPoll() })
onUnmounted(() => { stopSysPoll(); stopAuthPoll(); stopUpdPoll() })
async function loadAll() {
  try {
    dash.value = await adminApi.dashboard()
    policies.value = await adminApi.policies()
    settings.value = await adminApi.settings()
    allShares.value = (await adminApi.shares(1, 100)).items
    tasks.value = await adminApi.tasks()
    await loadLogs()
  } catch (e: any) { console.warn(e) }
}
async function loadTasks() { try { tasks.value = await adminApi.tasks() } catch {} }
async function loadLogs() {
  try {
    const d = await adminApi.logs(logPage.value, 20, logKw.value.trim())
    logs.value = d.items
    logTotal.value = d.total
  } catch {}
}
async function loadNotif() {
  try {
    const d = await adminApi.notifications(notifPage.value, 20, notifKw.value.trim(), notifClearedOnly.value)
    notifList.value = d.items
    notifTotal.value = d.total
  } catch {}
}
function exportLogs() {
  window.open(adminApi.logsExport(logKw.value.trim()), '_blank')
}

function typeName(t: string) {
  return ({ local: '本地目录', pan123: '123云盘', aliyun: '阿里云盘', baidu: '百度网盘', tianyi: '天翼云盘' } as any)[t] || t
}
function taskTypeName(t: string) { return ({ offline: 'HTTP 离线', bt: 'BT/磁力链', compress: '压缩', decompress: '解压', transfer: '转存' } as any)[t] || t }
function taskStatusName(s: string) { return ({ queued: '排队中', processing: '执行中', finished: '完成', error: '失败', canceled: '已取消' } as any)[s] || s }
function notifTypeName(t: string) { return ({ task: '任务', quota: '配额', system: '系统', offline: '离线下载' } as any)[t] || t }
function fmt(n: number) {
  if (!n) return '-'
  if (n > 1 << 30) return (n / (1 << 30)).toFixed(2) + ' GB'
  return (n / (1 << 20)).toFixed(1) + ' MB'
}
function fmtGB(n: number) {
  if (!n) return '0'
  return (n / (1 << 30)).toFixed(1) + ' GB'
}

async function editPolicyRow(p: any) {
  editPolicy.value = { id: p.id, name: p.name, letter: p.letter, type: p.type, rootPath: p.rootPath }
  pOpts.value = { ...(p.options || {}) }
  stopQr()
  policyShow.value = true
}
function newPolicy() {
  editPolicy.value = { ...emptyPolicy }
  pOpts.value = {}
  stopQr()
  policyShow.value = true
}
async function checkPolicy(p: any) {
  try {
    const { get } = await import('../api/http')
    const st = await get<{ ok: boolean; msg?: string; used?: number; total?: number }>(`/cloud/status?policyId=${p.id}`)
    if (st.ok) {
      // 本地策略无配额信息：直接展示 msg（"本地存储"），避免出现"已用 -"
      const detail = st.used || st.total
        ? `已用 ${fmt(st.used || 0)}${st.total ? ' / 总量 ' + fmt(st.total) : ''}`
        : (st.msg || '正常')
      await uiDlg.alert('测通结果', `连通正常！${detail}`)
    } else await uiDlg.alert('测通失败', st.msg || '未知错误')
  } catch (e: any) { toast.error(e.message) }
}
async function togglePolicy(p: any) {
  try { await adminApi.policyToggle(p.id, p.status === 'disabled' ? 'active' : 'disabled'); loadAll() } catch (e: any) { toast.error(e.message) }
}
async function delPolicy(p: any) {
  if (!confirm(`卸载存储 ${p.name}？（不会删除磁盘上的文件）`)) return
  try { await adminApi.policyDelete(p.id); loadAll() } catch (e: any) { toast.error(e.message) }
}
async function savePolicy(silent = false) {
  const d: any = { ...editPolicy.value, options: pOpts.value }
  if (editPolicy.value.id) await adminApi.policyUpdate(editPolicy.value.id, d)
  else {
    const p = await adminApi.policyCreate(d)
    editPolicy.value.id = p.id
  }
  if (!silent) { closePolicy(); loadAll() }
}
async function createPolicy() {
  try { await savePolicy() } catch (e: any) { toast.error(e.message) }
}
// ---- 云盘扫码绑定：渲染厂商授权页二维码（带签名 state），轮询 /cloud/status 直到回调完成 ----
const qrUrl = ref('')
const qrImg = ref('')
let authPoll: ReturnType<typeof setInterval> | null = null

async function startAuth(codeMode = false) {
  authMsg.value = ''
  qrImg.value = ''
  stopAuthPoll()
  try {
    const { get, post } = await import('../api/http')
    await savePolicy(true)
    // codeMode：手机完全不可达本站时的回退——oob 授权页直接显示授权码，人工粘贴
    const r = await get<{ url: string; mode: string }>(`/cloud/auth-url?policyId=${editPolicy.value.id}${codeMode ? '&mode=code' : ''}`)
    if (!r.url) { authMsg.value = '该类型无需跳转授权'; return }
    if (r.mode === 'qr' && !codeMode) {
      // 扫码绑定：二维码即厂商授权页（redirect 回 /api/cloud/callback），手机扫码→授权→自动回填
      qrUrl.value = r.url
      qrImg.value = await QRCode.toDataURL(r.url, { width: 180, margin: 1, errorCorrectionLevel: 'M' })
      authMsg.value = '正在等待扫码授权… 用云盘 App / 手机浏览器扫描二维码并确认授权（手机需能访问本站「公开地址」）。'
      pollAuth()
    } else {
      // 回退：授权码粘贴模式（站点公开地址不可达时使用）
      window.open(r.url, '_blank')
      const code = prompt('完成授权登录后，把页面上的授权码(code)粘贴到这里：')
      if (!code) return
      await post('/cloud/exchange', { policyId: editPolicy.value.id, type: editPolicy.value.type, code })
      authMsg.value = '授权成功！'
      loadAll()
    }
  } catch (e: any) { authMsg.value = e.message }
}
function pollAuth() {
  stopAuthPoll()
  authPoll = setInterval(async () => {
    try {
      const { get } = await import('../api/http')
      const st = await get<{ ok: boolean; msg?: string }>(`/cloud/status?policyId=${editPolicy.value.id}`)
      if (!st.ok) return // 尚未绑定（缺 token 时 status 报"需要 refresh_token…"）
      stopAuthPoll()
      qrImg.value = ''
      authMsg.value = '授权成功！'
      // 拉取最新 options 回填 token 字段
      const list = await get<any[]>('/admin/policies')
      const p = (list || []).find((x: any) => x.id === editPolicy.value.id)
      if (p) pOpts.value = { ...(p.options || {}) }
      toast.success('云盘绑定成功')
      loadAll()
    } catch { /* 轮询中的网络抖动忽略 */ }
  }, 2500)
}
function stopAuthPoll() { if (authPoll) { clearInterval(authPoll); authPoll = null } }
function stopQr() { stopAuthPoll(); qrImg.value = ''; authMsg.value = '' }
function closePolicy() { stopQr(); policyShow.value = false }
async function saveSettings() {
  try {
    await adminApi.settingsSet(settings.value)
    await session.loadSite()
    toast.success('设置已保存')
  } catch (e: any) { toast.error(e.message) }
}
async function adminDelShare(s: any) {
  if (!(await uiDlg.confirm('取消分享', `取消分享「${s.name}」？外链将立即失效。`, { danger: true, okText: '取消分享' }))) return
  try { await adminApi.shareDelete(s.id); loadAll() } catch (e: any) { toast.error(e.message) }
}
</script>

<style scoped>
.ac-side {
  width: 230px; flex: none; border-right: 1px solid var(--stroke-b);
  padding: 14px 10px; overflow: auto; display: flex; flex-direction: column; gap: 10px;
}
.ac-user {
  display: flex; align-items: center; gap: 10px; padding: 10px 12px;
  border-radius: var(--radius-2xl); background: var(--card);
}
.mini-avatar {
  width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center;
  background: linear-gradient(135deg, var(--theme-1), var(--theme-2)); color: #fff; font-size: 15px; flex: none;
}
.ac-nav {
  display: flex; align-items: center; gap: 11px; padding: 9px 12px; border-radius: var(--radius-sm);
  font-size: 13.5px; cursor: pointer; color: var(--text-2); margin-bottom: 1px; position: relative;
}
.ac-nav:hover { background: var(--hover-b); color: var(--text); }
.ac-nav.active { background: #3b91d822; color: var(--text); font-weight: 600; }
.ac-nav.active::before {
  content: ""; position: absolute; left: 0; top: 50%; transform: translateY(-50%);
  width: 3px; height: 16px; border-radius: 2px; background: linear-gradient(180deg, var(--theme-1), var(--theme-2));
}
.ac-badge {
  background: linear-gradient(135deg, var(--theme-1), var(--theme-2));
  color: #fff; border-radius: 10px; padding: 1px 7px; font-size: 10.5px;
}
.ac-content { flex: 1; overflow: auto; padding: 24px 30px; }
.ac-h2 { font-size: 20px; font-weight: 600; margin-bottom: 18px; }
.ac-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 18px; }
.ac-head .ac-h2 { margin-bottom: 0; }
.ac-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 12px; }
.ac-card {
  background: var(--card); border: 1px solid var(--stroke-b); border-radius: var(--radius);
}
.ac-stat { padding: 18px 20px; }
.ac-stat-num { font-size: 26px; font-weight: 300; }
.ac-stat-lbl { font-size: 12px; color: var(--text-3); margin-top: 4px; }
.ac-row-title { font-size: 14px; font-weight: 600; }
.ac-progress { height: 5px; border-radius: 3px; background: rgba(125,125,135,0.25); overflow: hidden; }
.ac-progress-fill { height: 100%; border-radius: 3px; background: linear-gradient(90deg, var(--theme-1), var(--theme-2)); transition: width 0.3s; }
.ac-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.ac-table th {
  text-align: left; padding: 10px 16px; font-weight: 600; color: var(--text-3); font-size: 12px;
  border-bottom: 1px solid var(--stroke-b);
}
.ac-table td { padding: 9px 16px; border-bottom: 1px solid var(--hover-b); }
.ac-table tr:hover td { background: var(--hover-b); }
.ac-strong { font-weight: 600; }
.ac-tag {
  background: #3b91d820; color: var(--theme-2); padding: 2px 9px; border-radius: 10px; font-size: 11.5px;
}
.ac-tag.admin { background: #8a6ef025; color: #8a6ef0; }
.ac-dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; background: #3fbf6f; margin-right: 6px; }
.ac-dot.off { background: #c4c9d0; }
.ac-status { font-size: 11.5px; color: var(--text-3); margin-top: 2px; }
.ac-kv { display: flex; justify-content: space-between; font-size: 12.5px; padding: 4px 0; color: var(--text-2); }
.ac-kv b { font-weight: 600; color: var(--text); }
.ac-set-title { font-size: 13px; font-weight: 600; color: var(--theme-2); margin: 6px 0 4px; }
.ac-set-row {
  display: flex; align-items: center; justify-content: space-between; gap: 20px;
  padding: 13px 0; border-bottom: 1px solid var(--hover-b);
}
.ac-set-lbl { display: flex; flex-direction: column; gap: 3px; }
.ac-set-lbl b { font-size: 13.5px; }
.ac-set-lbl span { font-size: 11.5px; color: var(--text-3); }
.ac-set-sep { height: 12px; }
/* 开关 */
.ac-switch { position: relative; width: 44px; height: 22px; flex: none; cursor: pointer; }
.ac-switch input { opacity: 0; width: 0; height: 0; }
.ac-slider {
  position: absolute; inset: 0; border-radius: 11px; background: rgba(125,125,135,0.4); transition: 0.2s;
}
.ac-slider::before {
  content: ""; position: absolute; width: 16px; height: 16px; border-radius: 50%;
  left: 3px; top: 3px; background: #fff; transition: 0.2s; box-shadow: 0 1px 3px rgba(0,0,0,0.3);
}
.ac-switch input:checked + .ac-slider { background: linear-gradient(90deg, var(--theme-1), var(--theme-2)); }
.ac-switch input:checked + .ac-slider::before { transform: translateX(22px); }
.ac-guide {
  font-size: 12px; color: var(--text-2); line-height: 1.8;
  background: #3b91d812; border-radius: 8px; padding: 10px 14px; margin-bottom: 10px;
}
.ac-guide b { color: var(--theme-2); display: block; margin-bottom: 2px; }
.ac-guide a { color: var(--theme-2); }
.qr-panel {
  display: flex; flex-direction: column; align-items: center; gap: 8px;
  padding: 14px; margin-bottom: 10px; border-radius: 10px;
  background: var(--card, #fff); border: 1px solid var(--stroke-b, #e5e7eb);
}
.qr-panel img { border-radius: 8px; }
.qr-panel .qr-actions { display: flex; align-items: center; gap: 10px; font-size: 12px; }
.qr-panel .qr-actions a { color: var(--theme-2); }

/* 系统资源监控 */
.sys-card .ac-row-title { display: flex; align-items: center; gap: 8px; }
.sys-live {
  font-size: 10.5px; font-weight: 500; color: #34c759; background: rgba(52,199,89,0.12);
  padding: 1px 8px; border-radius: 8px; letter-spacing: 0.5px;
}
.sys-live::before { content: '●'; margin-right: 4px; animation: sys-blink 2s infinite; }
.sys-live.off { color: var(--text-3); background: rgba(125,125,135,0.15); }
.sys-live.off::before { content: '○'; animation: none; }
@keyframes sys-blink { 0%, 100% { opacity: 1 } 50% { opacity: 0.35 } }
.sys-muted { color: var(--text-3); font-size: 12.5px; padding: 10px 0; }
.sys-grid { display: grid; grid-template-columns: 1fr 1.1fr; gap: 24px; margin-top: 6px; }
.sys-col { display: flex; flex-direction: column; gap: 18px; }
.sys-block {}
.sys-line { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 6px; }
.sys-name { font-size: 12.5px; font-weight: 600; color: var(--text-2); }
.sys-val { font-size: 12px; color: var(--text-2); font-variant-numeric: tabular-nums; }
.sys-sub { font-size: 11px; color: var(--text-3); margin-top: 5px; }
.sys-cores { display: flex; align-items: flex-end; gap: 3px; height: 26px; margin-top: 8px; }
.sys-core {
  flex: 1; max-width: 18px; height: 100%; border-radius: 2px; overflow: hidden;
  background: rgba(125,125,135,0.2); display: flex; align-items: flex-end;
}
.sys-core-fill { width: 100%; border-radius: 2px; background: linear-gradient(180deg, var(--theme-2), var(--theme-1)); transition: height 0.5s; }
.sys-core-more { font-size: 10px; color: var(--text-3); margin-left: 4px; }
.sys-disk { margin: 8px 0; }
.sys-disk-head { display: flex; justify-content: space-between; font-size: 12px; margin-bottom: 4px; }
.sys-disk-name { font-weight: 600; }
.sys-disk-num { color: var(--text-3); font-variant-numeric: tabular-nums; }
.ac-progress-fill.warn { background: linear-gradient(90deg, #e8a33d, #e86a3d); }
.sys-net { display: flex; align-items: center; gap: 10px; padding: 5px 0; border-bottom: 1px solid var(--stroke-b); }
.sys-net:last-child { border-bottom: none; }
.sys-net-name { flex: 1; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sys-rx, .sys-tx { font-size: 11.5px; font-variant-numeric: tabular-nums; color: var(--text-2); min-width: 86px; text-align: right; }
.sys-rx { color: var(--theme-2); }
/* 应用权限三态控件 */
.perm-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; margin-top: 6px; }
.perm-item {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 6px 10px; border: 1px solid var(--stroke-b); border-radius: 8px; font-size: 13px;
  transition: border-color .15s, background .15s;
}
.perm-item.allow { border-color: var(--theme-2); background: color-mix(in srgb, var(--theme-2) 8%, transparent); }
.perm-item.deny { border-color: var(--danger, #e05252); background: color-mix(in srgb, var(--danger, #e05252) 8%, transparent); }
.perm-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.perm-seg { display: flex; gap: 2px; background: rgba(125,125,135,.12); border-radius: 6px; padding: 2px; flex: none; }
.perm-btn {
  border: 0; background: transparent; font-size: 12px; padding: 3px 9px;
  border-radius: 5px; cursor: pointer; color: var(--text-2);
}
.perm-btn.on { background: linear-gradient(135deg, var(--theme-1), var(--theme-2)); color: #fff; }
.perm-item.deny .perm-btn.on { background: linear-gradient(135deg, #e86a3d, var(--danger, #e05252)); }
</style>
