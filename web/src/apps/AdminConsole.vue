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
                <span style="color: var(--text-3)" :title="p.rootPath">{{ fmt(p.usageBytes) }} 已用</span>
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
            <button class="btn primary" @click="newPolicy"><AppIcon name="plus" :size="15" />挂载本机目录</button>
          </div>
          <div class="ac-card" style="padding: 4px 0">
            <table class="ac-table">
              <thead><tr><th>名称</th><th>类型</th><th>根目录 / 状态</th><th>用量</th><th style="width: 240px">操作</th></tr></thead>
              <tbody>
                <tr v-for="p in policies" :key="p.id">
                  <td class="ac-strong">{{ p.name }}</td>
                  <!-- 单用户私有部署只有「本地目录」；遗留的云盘策略标注出来，好让用户去卸载它 -->
                  <td>
                    <span class="ac-tag" :class="{ admin: p.type !== 'local' }">{{ typeName(p.type) }}</span>
                  </td>
                  <td>
                    <div style="font-size: 12px; max-width: 240px; overflow: hidden; text-overflow: ellipsis" :title="p.rootPath || ''">{{ p.rootPath || p.statusMsg || '-' }}</div>
                    <div class="ac-status" :class="p.status"><span class="ac-dot" :class="{ off: p.status !== 'active' }"></span>{{ p.status === 'active' ? '正常' : p.status === 'disabled' ? '已停用' : '异常' }}</div>
                    <div v-if="p.davPath" class="ac-status" style="color: var(--text-3); cursor: pointer"
                         :title="'点击选择本机地址并复制：' + p.davPath"
                         @click="copyDav(p.davPath)">WebDAV {{ p.davPath }}</div>
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
            <div v-if="!policies.length" style="padding: 24px; text-align: center; color: var(--text-3); font-size: 13px">点击右上角「挂载本机目录」把本机文件夹加进网盘（也可在文件管理器里点「挂载文件夹」）</div>
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
              <thead><tr><th>ID</th><th>类型</th><th>状态</th><th>进度</th><th>时间</th><th>说明</th></tr></thead>
              <tbody>
                <tr v-for="t in tasks" :key="t.id">
                  <td>#{{ t.id }}</td>
                  <td><span class="ac-tag">{{ taskTypeName(t.type) }}</span></td>
                  <td><span class="ac-status" :class="t.status === 'finished' ? 'active' : t.status"><span class="ac-dot" :class="{ off: t.status === 'error' || t.status === 'canceled' }"></span>{{ taskStatusName(t.status) }}</span></td>
                  <td style="width: 160px"><div class="ac-progress"><div class="ac-progress-fill" :style="{ width: t.progress + '%' }"></div></div></td>
                  <td style="color: var(--text-3)">{{ new Date(t.createdAt).toLocaleString() }}</td>
                  <!-- 失败原因只在失败时显示；运行中的阶段提示（msg）单独一列，避免混在一起看不出是"报错"还是"进度" -->
                  <td style="font-size: 12px; max-width: 220px; overflow: hidden; text-overflow: ellipsis">
                    <span v-if="t.status === 'error'" :title="t.error" style="color: var(--danger)">{{ t.error }}</span>
                    <span v-else-if="t.msg" :title="t.msg" style="color: var(--text-3)">{{ t.msg }}</span>
                  </td>
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
            <div class="ac-set-title">离线下载</div>
            <div class="ac-set-row">
              <div class="ac-set-lbl"><b>允许访问内网地址</b><span>关闭时（默认）服务器代下载会拒绝内网 / 保留网段（SSRF 防护）。单机私有部署下，若想把 NAS、内网媒体服务器、本机其它服务上的文件或 m3u8 拉进网盘，需要打开它。</span></div>
              <label class="ac-switch"><input type="checkbox" :checked="settings.offline_allow_private === 'true'" @change="settings.offline_allow_private = ($event.target as HTMLInputElement).checked ? 'true' : 'false'" /><span class="ac-slider"></span></label>
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

    <!-- 存储策略对话框（单用户私有部署：只挂本机磁盘目录，云盘挂载整体移除） -->
    <div class="dialog-mask" v-if="policyShow" @click.self="closePolicy">
      <div class="dialog" style="width: 480px">
        <h3>{{ editPolicy.id ? '编辑' : '挂载' }}本机目录</h3>
        <div class="row">
          <label>类型</label>
          <input class="input" value="本地磁盘目录" disabled style="width: 100%" />
        </div>
        <div class="row"><label>显示名称</label><input class="input" v-model="editPolicy.name" placeholder="如：电影 / 我的云盘" style="width: 100%" /></div>
        <div class="row">
          <label>本机目录</label>
          <input class="input" v-model="editPolicy.rootPath" placeholder="如 E:\媒体\电影（也可在文件管理器里点「挂载文件夹」用选择器挑）" style="width: 100%" />
        </div>
        <div class="ac-guide">
          <b>挂载本机目录：</b>
          ① 填一个<b>已存在</b>的绝对路径（也可在文件管理器中进入该目录后点「挂载文件夹」用选择器挑）；<br>
          ② 保存后点列表里的「测通」验证目录是否可读；<br>
          ③ 手机/其他电脑走 WebDAV 访问时，地址用列表里那行 <code>WebDAV /dav/…</code>（点一下会让你选本机地址）。
        </div>
        <div class="actions">
          <button class="btn" @click="closePolicy">取消</button>
          <button class="btn primary" @click="createPolicy">{{ editPolicy.id ? '保存' : '挂载' }}</button>
        </div>
      </div>
    </div>

    <!-- 点击某台挂载的 WebDAV 路径时弹出：多网卡机器上让用户挑一个对方能访问的地址 -->
    <AddressPicker v-model:show="davPickerShow" title="复制 WebDAV 地址" :path="davPickerPath" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useSession } from '../stores/session'
import { useAppState } from '../stores/appstate'
import { useUiDialog, useToast } from '../stores/dialog'
import { adminApi } from '../api/modules'
import AppIcon from '../components/AppIcon.vue'
import AddressPicker from '../components/AddressPicker.vue'

const session = useSession()
const uiDlg = useUiDialog()
const toast = useToast()
const tab = ref('dash')
// WebDAV 统一入口：挂载路径段由后端下发（前端不重复实现命名规则），
// 主机部分则交给地址选择器 —— 多网卡机器上不能拿 location.origin 猜。
const davPickerShow = ref(false)
const davPickerPath = ref('')
function copyDav(path: string) {
  davPickerPath.value = path
  davPickerShow.value = true
}
const tabs = computed(() => [
  { id: 'dash', name: '仪表盘', icon: 'info' },
  { id: 'policies', name: '存储策略', icon: 'drive' },
  { id: 'shares', name: '分享审计', icon: 'share' },
  { id: 'tasks', name: '任务监控', icon: 'list' },
  { id: 'settings', name: '站点设置', icon: 'settings' },
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
// 单用户私有部署：类型固定为「本地磁盘目录」，云盘挂载与其授权流程已整体移除
const editPolicy = ref<any>({})
const emptyPolicy = { type: 'local', name: '', letter: '', rootPath: '' }

function switchTab(id: string) {
  tab.value = id
  if (id === 'tasks') loadTasks()
  if (id === 'notify') loadNotif()
  if (id === 'dash') startSysPoll(); else stopSysPoll()
}

// 挂载列表指纹：保存上一次挂载项（id/名称）快照，用于避免无变化时反复广播
let lastPolicySig = ''
onMounted(() => { loadAll(); startSysPoll() })
onUnmounted(() => { stopSysPoll() })
async function loadAll() {
  try {
    dash.value = await adminApi.dashboard()
    const ps = await adminApi.policies()
    policies.value = ps
    // 挂载列表（id/名称）变化时广播：已打开的文件管理器即时刷新挂载点，无需关闭页面重开
    const sig = JSON.stringify(ps.map((p: any) => [p.id, p.name]))
    if (sig !== lastPolicySig) {
      lastPolicySig = sig
      window.dispatchEvent(new CustomEvent('cp-policies-changed'))
    }
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
  // 只保留本地目录；更早版本遗留的云盘策略仍能在列表里看出类型（便于用户卸载它）
  return ({ local: '本地目录', pan123: '123云盘（已不支持）', aliyun: '阿里云盘（已不支持）', baidu: '百度网盘（已不支持）', tianyi: '天翼云盘（已不支持）' } as any)[t] || t
}
function taskTypeName(t: string) { return ({ offline: 'HTTP 离线', m3u8: 'm3u8 视频', bt: 'BT/磁力链', compress: '压缩', decompress: '解压', transfer: '转存' } as any)[t] || t }
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
  editPolicy.value = { id: p.id, name: p.name, letter: p.letter, type: 'local', rootPath: p.rootPath }
  policyShow.value = true
}
function newPolicy() {
  editPolicy.value = { ...emptyPolicy }
  policyShow.value = true
}
async function checkPolicy(p: any) {
  try {
    const { get } = await import('../api/http')
    const st = await get<{ ok: boolean; msg?: string; used?: number; total?: number }>(`/policies/status?policyId=${p.id}`)
    if (st.ok) {
      // 本地目录的「连通」= 目录真的读得出来（能读到多少项也一并回报）
      const detail = st.used || st.total
        ? `已用 ${fmt(st.used || 0)}${st.total ? ' / 总量 ' + fmt(st.total) : ''}`
        : (st.msg || '正常')
      await uiDlg.alert('测通结果', `挂载正常！${detail}`)
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
  const d: any = { ...editPolicy.value }
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
function closePolicy() { policyShow.value = false }
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
