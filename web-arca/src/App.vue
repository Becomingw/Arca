<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ArrowUpRight, Bell, BookOpen, Box, ChartNoAxesCombined, CircleUserRound, KeyRound, Layers3, LogOut, MessageSquare, Moon, Sun, Terminal } from 'lucide-vue-next'
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { api, exportOpenCode, restoreSession, signOut, viewer } from './api.js'

const entries = [
  { id: 'overview', title: '概览', icon: Box },
  { id: 'models', title: '模型', icon: Layers3 },
  { id: 'keys', title: '我的 API Keys', icon: KeyRound },
  { id: 'feedback', title: '讨论与反馈', icon: MessageSquare },
  { id: 'activity', title: '团队活动', icon: ChartNoAxesCombined },
  { id: 'profile', title: '个人设置', icon: CircleUserRound },
  { id: 'agent', title: 'Agent 协作', icon: Terminal },
]
const adminEntry = { id: 'entitlements', title: '组织与权益', icon: Layers3 }
const navigation = computed(() => viewer.value?.role >= 100 ? [...entries, adminEntry] : entries)
const section = ref('overview')
const issueID = ref('')
const issue = ref(null)
const topics = ref([])
const topicOffset = ref(0)
const topicTotal = ref(0)
const eventAfter = ref(0)
const eventHistory = ref([])
const topicPageSize = 35
const models = ref([])
const keys = ref([])
const profile = ref(null)
const heatmap = ref(null)
const leaderboard = ref(null)
const entitlement = ref(null)
const entitlementError = ref('')
const policyText = ref('')
const policyVersion = ref(null)
const previewUser = ref('')
const previewKey = ref('')
const policyPreview = ref(null)
const editingTopic = ref(null)
const editingReply = ref(null)
let pendingCommand
const notices = ref([])
const unread = ref(0)
const busy = ref(false)
const restoring = ref(true)
const message = ref('')
const avatarHandles = ref({})
const noticesOpen = ref(false)
const theme = ref(localStorage.getItem('arca-console-theme') || 'light')
const topicDraft = ref({ title: '', description: '', category: 'bug', request_id: '', include_diagnostics: false })
const diagnostic = ref(null)
const reply = ref('')
const operationID = ref('')
const relayKey = ref('')
const activityDays = ref(7)
const myAvatar = computed(() => profile.value?.avatar || '')
const tokenSum = computed(() => heatmap.value?.days?.reduce((sum, item) => sum + item.tokens, 0) || 0)
const greatest = computed(() => Math.max(1, ...(heatmap.value?.days?.map(item => item.tokens) || [])))
const activeTitle = computed(() => navigation.value.find(entry => entry.id === section.value)?.title || '概览')
let noticeTimer
let loadVersion = 0

watch(theme, next => {
  document.documentElement.dataset.theme = next
  localStorage.setItem('arca-console-theme', next)
}, { immediate: true })

function readLocation() {
  const [name, id] = location.hash.slice(1).split('/')
  section.value = navigation.value.some(entry => entry.id === name) ? name : 'overview'
  issueID.value = section.value === 'feedback' && /^[a-f\d-]{36}$/i.test(id || '') ? id : ''
}

function go(name, id) {
  location.hash = id ? `${name}/${id}` : name
  readLocation()
}

async function populateAvatars(detail, items, version = loadVersion) {
  const ids = items.map(item => item.owner_id).concat(detail ? [detail.issue.owner_id, ...detail.events.map(event => event.actor_id)] : [])
  const unique = [...new Set(ids.filter(id => /^\d+$/.test(id || '')))].slice(0, 100)
  const handles = unique.length ? await api('/api/arca/avatars?ids=' + unique.join(',')) : {}
  if (version === loadVersion) avatarHandles.value = { ...avatarHandles.value, ...handles }
}

async function refreshNotices() {
  const userID = viewer.value?.id
  if (!userID) return
  const response = await api('/api/arca/feedback/notifications?limit=20')
  if (viewer.value?.id !== userID) return
  notices.value = response.items
  unread.value = response.unread_count
  await populateAvatars(null, response.items.map(item => ({ owner_id: item.actor_id })))
}

async function loadPage() {
  if (!viewer.value) return
  const version = ++loadVersion
  const current = section.value
  const selectedIssue = issueID.value
  try {
    busy.value = true
    message.value = ''
    switch (current) {
    case 'overview': {
      entitlementError.value = ''
      const [available, chart, benefits] = await Promise.all([
        api('/api/user/models'),
        api('/api/arca/profile/heatmap'),
        api('/api/arca/entitlements/me').catch(() => null),
      ])
      if (version !== loadVersion) return
      models.value = Array.isArray(available) ? available : []
      heatmap.value = chart
      entitlement.value = benefits
      if (!benefits) entitlementError.value = '暂时无法获取权益信息。'
      break
    }
    case 'models': {
      const available = await api('/api/user/models')
      if (version === loadVersion) models.value = Array.isArray(available) ? available : []
      break
    }
    case 'keys': {
      // The native New API key list omits the secret.
      const available = await api('/api/token/?p=1&page_size=50')
      if (version === loadVersion) keys.value = available.items || []
      break
    }
    case 'feedback': {
      const [page, detail] = await Promise.all([
        api('/api/arca/feedback?limit=' + topicPageSize + '&skip=' + topicOffset.value),
        selectedIssue ? api('/api/arca/feedback/' + selectedIssue + '?limit=50&after_event=' + eventAfter.value) : null,
      ])
      if (version !== loadVersion) return
      topics.value = page.items
      topicTotal.value = page.total
      issue.value = detail
      await populateAvatars(detail, page.items, version)
      break
    }
    case 'activity': {
      const report = await api('/api/arca/team-activity?days=' + activityDays.value)
      if (version === loadVersion) leaderboard.value = report
      break
    }
    case 'entitlements': {
      const record = await api('/api/arca/admin/entitlements')
      if (version === loadVersion) {
        policyText.value = JSON.stringify(record.policy, null, 2)
        policyVersion.value = record.version
        policyPreview.value = null
      }
      break
    }
    case 'profile': {
      const [preferences, chart] = await Promise.all([
        api('/api/arca/profile'), api('/api/arca/profile/heatmap'),
      ])
      if (version !== loadVersion) return
      profile.value = preferences
      heatmap.value = chart
      break
    }
    }
  } catch (error) {
    if (version === loadVersion) message.value = error.message
  } finally {
    if (version === loadVersion) busy.value = false
  }
}

function handleFocus() {
  if (!viewer.value) restoreSession().catch(error => { message.value = error.message })
}

onMounted(async () => {
  readLocation()
  window.addEventListener('hashchange', readLocation)
  window.addEventListener('focus', handleFocus)
  try {
    await restoreSession()
    readLocation()
  } catch (error) {
    message.value = error.message
  } finally {
    restoring.value = false
  }
  noticeTimer = setInterval(() => refreshNotices().catch(() => {}), 60000)
})

onUnmounted(() => {
  clearInterval(noticeTimer)
  window.removeEventListener('hashchange', readLocation)
  window.removeEventListener('focus', handleFocus)
})

watch(issueID, () => {
  eventAfter.value = 0
  eventHistory.value = []
  editingTopic.value = null
  editingReply.value = null
  reply.value = ''
  pendingCommand = undefined
})
watch([section, issueID, activityDays, topicOffset, eventAfter], loadPage)

function nextEvents() {
  if (!issue.value?.has_more) return
  eventHistory.value.push(eventAfter.value)
  eventAfter.value = issue.value.next_event_cursor
}
function previousEvents() {
  if (eventHistory.value.length) eventAfter.value = eventHistory.value.pop()
}
watch(() => viewer.value?.id, (id, previous) => {
  if (id === previous) return
  loadVersion++
  notices.value = []
  unread.value = 0
  profile.value = null
  avatarHandles.value = {}
  issue.value = null
  topics.value = []
  keys.value = []
  models.value = []
  policyText.value = ''
  policyPreview.value = null
  policyVersion.value = null
  entitlement.value = null
  heatmap.value = null
  topicDraft.value = { title: '', description: '', category: 'bug', request_id: '', include_diagnostics: false }
  editingTopic.value = null
  editingReply.value = null
  reply.value = ''
  pendingCommand = undefined
  if (id) {
    Promise.all([api('/api/arca/profile'), refreshNotices()])
      .then(([current]) => { if (viewer.value?.id === id) profile.value = current })
      .catch(() => {})
    readLocation()
    loadPage()
  }
})
watch(() => topicDraft.value.request_id, () => {
  diagnostic.value = null
  topicDraft.value.include_diagnostics = false
})
watch(() => [topicDraft.value.title, topicDraft.value.description, topicDraft.value.category, topicDraft.value.request_id, topicDraft.value.include_diagnostics], () => { operationID.value = '' })

function avatarURL(handle) {
  return handle ? '/api/arca/avatars/' + encodeURIComponent(handle) : ''
}

function markdown(text) {
  return DOMPurify.sanitize(marked.parse(text || '', { async: false, breaks: true }), {
    USE_PROFILES: { html: true }, FORBID_ATTR: ['style'], FORBID_TAGS: ['img', 'form', 'style', 'video', 'audio', 'source', 'iframe', 'input', 'button', 'textarea', 'select'],
  })
}

function formatDate(time) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(time * 1000))
}

function statusLabel(status) {
  return ({ open: '处理中', resolved: '已解决', closed: '已关闭' })[status] || status
}

async function action(work) {
  busy.value = true
  message.value = ''
  try {
    await work()
  } catch (error) {
    message.value = error.message
  } finally {
    busy.value = false
  }
}

async function previewDiagnostic() {
  const requested = topicDraft.value.request_id.trim()
  diagnostic.value = null
  await action(async () => {
    const summary = await api('/api/arca/requests/' + encodeURIComponent(requested) + '/diagnostics')
    if (topicDraft.value.request_id.trim() === requested) diagnostic.value = summary
  })
}

async function submitTopic() {
  await action(async () => {
    if (!operationID.value) operationID.value = crypto.randomUUID()
    const { request_id, ...draft } = topicDraft.value
    const result = await api('/api/arca/feedback', {
      method: 'POST', body: { ...draft, request_id: request_id.trim() || undefined, client_request_id: operationID.value },
    })
    topicDraft.value = { title: '', description: '', category: 'bug', request_id: '', include_diagnostics: false }
    diagnostic.value = null
    go('feedback', result.issue.id)
  })
}

async function command(data) {
  if (!issue.value) return
  const current = issue.value.issue
  await action(async () => {
    const fingerprint = JSON.stringify([current.id, current.version, data])
    if (pendingCommand?.fingerprint !== fingerprint) {
      pendingCommand = { fingerprint, id: crypto.randomUUID() }
    }
    const result = await api('/api/arca/feedback/' + current.id + '/commands', {
      method: 'POST', body: { ...data, expected_version: current.version, client_request_id: pendingCommand.id },
    })
    if (issueID.value !== current.id) return
    pendingCommand = undefined
    if (data.action === 'reply' && (issue.value.has_more || eventAfter.value > 0)) {
      eventHistory.value.push(eventAfter.value)
      eventAfter.value = Math.max(0, result.issue.activity_seq - 1)
    }
    editingTopic.value = null
    editingReply.value = null
    if (data.action === 'delete_issue') go('feedback')
    if (data.action === 'reply' || data.action === 'set_status') reply.value = ''
    await loadPage()
  })
}

function confirmDelete(data) {
  if (window.confirm('确定要删除这条内容吗？')) return command(data)
}

function previewPolicy() {
  return action(async () => {
    policyPreview.value = await api('/api/arca/admin/entitlements/preview', { method: 'POST', body: {
      user_id: Number(previewUser.value), api_key_id: Number(previewKey.value) || undefined,
      config: JSON.parse(policyText.value),
    } })
  })
}

function savePolicy() {
  return action(async () => {
    const result = await api('/api/arca/admin/entitlements', { method: 'PUT', body: {
      expected_version: policyVersion.value, policy: JSON.parse(policyText.value),
    } })
    policyVersion.value = result.version
    policyText.value = JSON.stringify(result.policy, null, 2)
    policyPreview.value = null
  })
}

async function openNotice(item) {
  noticesOpen.value = false
  go('feedback', item.issue_id)
  if (!item.read_at) {
    await action(async () => {
      await api('/api/arca/feedback/notifications/read', { method: 'POST', body: { ids: [item.id] } })
      await refreshNotices()
    })
  }
}

async function changeAvatar(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  await action(async () => {
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type) || file.size > 10 * 1024 * 1024) throw new Error('请选择 10 MB 以内的 PNG、JPEG 或 WebP 图片')
    const bitmap = await createImageBitmap(file)
    try {
      const canvas = document.createElement('canvas')
      canvas.width = canvas.height = 256
      const side = Math.min(bitmap.width, bitmap.height)
      canvas.getContext('2d').drawImage(bitmap, (bitmap.width - side) / 2, (bitmap.height - side) / 2, side, side, 0, 0, 256, 256)
      const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/webp', 0.85))
      if (!blob) throw new Error('图片处理失败')
      profile.value = { ...profile.value, ...await api('/api/arca/profile/avatar', { method: 'PUT', body: blob }) }
    } finally {
      bitmap.close()
    }
  })
}

function downloadConfig() {
  const selected = relayKey.value.trim()
  relayKey.value = ''
  return action(async () => {
    const config = await exportOpenCode(selected)
    const url = URL.createObjectURL(new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' }))
    const link = document.createElement('a')
    link.href = url
    link.download = 'opencode.json'
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  })
}

async function logout() {
  await action(async () => {
    await signOut()
    issue.value = null
    notices.value = []
    unread.value = 0
    profile.value = null
    go('overview')
  })
}
</script>

<template>
  <main v-if="restoring || !viewer" class="landing">
    <header class="landing-header"><a class="brand" href="/arca/"><Box :size="24" /> Arca</a><a href="/about">New API <ArrowUpRight :size="15" /></a></header>
    <section class="hero">
      <div class="hero-mark" aria-hidden="true"><Box :size="180" :stroke-width="0.7" /></div>
      <p class="hero-brand">Arca</p>
      <h1>让团队在同一个地方使用 AI。</h1>
      <p>管理模型与使用权益，查看请求，和团队一起解决问题。</p>
      <a v-if="!restoring" class="primary" href="/sign-in?redirect=%2Farca%2F">使用 New API 账号登录 <ArrowUpRight :size="18" /></a>
      <p v-else role="status">正在恢复登录状态…</p>
      <p v-if="message" role="alert" class="error">{{ message }}</p>
    </section>
  </main>

  <div v-else class="workspace">
    <aside class="sidebar">
      <a class="brand" href="#overview"><Box :size="23" /> Arca</a>
      <nav aria-label="控制台">
        <a v-for="entry in navigation" :key="entry.id" :href="'#' + entry.id" :aria-current="section === entry.id ? 'page' : undefined">
          <component :is="entry.icon" :size="18" /> <span>{{ entry.title }}</span>
        </a>
      </nav>
      <div class="sidebar-bottom">
        <a href="/dashboard"><ArrowUpRight :size="17" /> New API 控制台</a>
        <button type="button" @click="logout"><LogOut :size="17" /> 退出登录</button>
      </div>
    </aside>
    <div class="main-panel">
      <header class="topbar">
        <span>{{ activeTitle }}</span>
        <div class="top-controls">
          <button type="button" class="icon" :aria-label="theme === 'dark' ? '浅色' : '深色'" @click="theme = theme === 'dark' ? 'light' : 'dark'">
            <component :is="theme === 'dark' ? Sun : Moon" :size="19" />
          </button>
          <div class="notice-area">
            <button type="button" class="icon" aria-label="站内通知" :aria-expanded="noticesOpen" @click="noticesOpen = !noticesOpen; if (noticesOpen) refreshNotices().catch(error => message = error.message)">
              <Bell :size="19" /><span v-if="unread" class="unread">{{ unread }}</span>
            </button>
            <section v-if="noticesOpen" class="popover">
              <div class="heading-row"><strong>站内通知</strong><button v-if="unread" @click="action(async () => { await api('/api/arca/feedback/notifications/read', { method: 'POST', body: { all: true } }); await refreshNotices() })">全部标为已读</button></div>
              <button v-for="item in notices" :key="item.id" class="notice" @click="openNotice(item)"><strong>{{ item.issue_title }}</strong><span class="author"><img v-if="avatarHandles[item.actor_id]" class="avatar" :src="avatarURL(avatarHandles[item.actor_id])" alt="" />{{ item.actor_username || '成员' }} · {{ item.snippet }}</span></button>
              <p v-if="!notices.length" class="muted">暂无通知。</p>
            </section>
          </div>
          <a href="#profile" class="avatar" :aria-label="viewer.username">
            <img v-if="myAvatar" :src="avatarURL(myAvatar)" alt="" />
            <span v-else>{{ viewer.username.slice(0, 1).toUpperCase() }}</span>
          </a>
        </div>
      </header>

      <div class="page">
        <p v-if="message" class="error" role="alert">{{ message }}</p>
        <p v-if="busy" role="status" class="loading">处理中…</p>
        <template v-if="section === 'overview'">
          <h1>{{ viewer.display_name || viewer.username }}</h1>
          <div class="cards">
            <a class="card stat" href="#models"><span>可用模型</span><strong>{{ models.length }}</strong></a>
            <a class="card stat" href="#profile"><span>统计周期 Token</span><strong>{{ heatmap?.available ? tokenSum.toLocaleString('zh-CN') : '—' }}</strong></a>
            <a class="card stat" href="#feedback"><span>未读通知</span><strong>{{ unread }}</strong></a>
          </div>
          <section class="card"><h2>使用权益</h2><p v-if="entitlementError" class="muted">{{ entitlementError }}</p><pre v-else-if="entitlement">{{ JSON.stringify(entitlement, null, 2) }}</pre></section>
        </template>

        <template v-else-if="section === 'models'">
          <h1>模型</h1>
          <p v-if="!models.length" class="muted">暂无可用模型。</p>
          <div class="cards"><article v-for="item in models" :key="item" class="card model"><Layers3 :size="20" />{{ item }}</article></div>
        </template>

        <template v-else-if="section === 'keys'">
          <div class="heading-row"><h1>我的 API Keys</h1><a class="secondary" href="/keys">管理 API Keys <ArrowUpRight :size="16" /></a></div>
          <ul class="card plain-list"><li v-for="key in keys" :key="key.id"><KeyRound :size="17" />{{ key.name }}</li><li v-if="!keys.length" class="muted">暂无 API Key。</li></ul>
          <form class="card form" @submit.prevent="downloadConfig">
            <h2>导出 OpenCode 配置</h2>
            <label>推理 API Key<input v-model="relayKey" type="password" autocomplete="off" required spellcheck="false" /></label>
            <button class="primary" :disabled="busy">下载 opencode.json</button>
            <a class="secondary" href="/api/opencode/setup">下载 Linux 同步工具</a>
          </form>
        </template>

        <template v-else-if="section === 'feedback'">
          <div class="heading-row"><h1>讨论与反馈</h1><a v-if="issueID" class="secondary" href="#feedback">回到主题列表</a></div>
          <div v-if="issueID && issue?.issue.id === issueID" class="discussion card">
            <div class="heading-row"><h2>{{ issue.issue.title }}</h2><span class="tag">{{ statusLabel(issue.issue.status) }}</span></div>
            <div class="author"><img v-if="avatarHandles[issue.issue.owner_id]" class="avatar" :src="avatarURL(avatarHandles[issue.issue.owner_id])" alt="" />{{ issue.issue.owner_username || '成员' }} · {{ formatDate(issue.issue.created_at) }}</div>
            <div class="markdown" v-html="markdown(issue.issue.description)" />
            <div class="controls">
              <button v-if="viewer.role >= 10 || String(viewer.id) === issue.issue.owner_id" @click="editingTopic = { title: issue.issue.title, description: issue.issue.description }">编辑主题</button>
              <button v-if="viewer.role >= 10 || String(viewer.id) === issue.issue.owner_id" @click="confirmDelete({ action: 'delete_issue' })">删除主题</button>
              <button v-if="viewer.role >= 10" @click="command({ action: 'set_pinned', pinned: !issue.issue.pinned_at })">{{ issue.issue.pinned_at ? '取消置顶' : '置顶' }}</button>
            </div>
            <form v-if="editingTopic" class="form" @submit.prevent="command({ action: 'edit_issue', ...editingTopic })">
              <label>标题<input v-model="editingTopic.title" required maxlength="200" /></label>
              <label>内容<textarea v-model="editingTopic.description" required rows="4" /></label>
              <div class="controls"><button class="primary" :disabled="busy">保存</button><button type="button" @click="editingTopic = null">取消</button></div>
            </form>
            <article v-for="event in issue.events.filter(event => event.kind !== 'created')" :key="event.seq" class="reply">
              <div class="author"><img v-if="avatarHandles[event.actor_id]" class="avatar" :src="avatarURL(avatarHandles[event.actor_id])" alt="" />{{ event.actor_username || '成员' }} · {{ formatDate(event.created_at) }}</div>
              <div v-if="event.content" class="markdown" v-html="markdown(event.content)" />
              <p v-if="event.to_status" class="muted">状态更新为{{ statusLabel(event.to_status) }}</p>
              <div v-if="event.kind === 'reply' && !event.deleted_at && (viewer.role >= 10 || String(viewer.id) === event.actor_id)" class="controls">
                <button v-if="String(viewer.id) === event.actor_id" @click="editingReply = { seq: event.seq, content: event.content }">编辑回复</button>
                <button @click="confirmDelete({ action: 'delete_reply', seq: event.seq })">删除回复</button>
              </div>
              <form v-if="editingReply?.seq === event.seq" class="form" @submit.prevent="command({ action: 'edit_reply', ...editingReply })">
                <label>回复<textarea v-model="editingReply.content" required rows="3" /></label>
                <div class="controls"><button class="primary" :disabled="busy">保存</button><button type="button" @click="editingReply = null">取消</button></div>
              </form>
            </article>
            <div v-if="eventHistory.length || issue.has_more" class="controls">
              <button type="button" :disabled="busy || !eventHistory.length" @click="previousEvents">上一页回复</button>
              <button type="button" :disabled="busy || !issue.has_more" @click="nextEvents">下一页回复</button>
            </div>
            <form class="form" @submit.prevent="command({ action: 'reply', content: reply })">
              <label>回复<textarea v-model="reply" rows="4" required placeholder="输入 @用户名 通知成员" /></label>
              <div class="heading-row">
                <button class="primary" :disabled="busy">发送回复</button>
                <button v-if="String(viewer.id) === issue.issue.owner_id && issue.issue.status === 'open'" type="button" :disabled="busy || !reply.trim()" @click="command({ action: 'set_status', status: 'resolved', comment: reply })">标记已解决</button>
                <button v-if="String(viewer.id) === issue.issue.owner_id && issue.issue.status !== 'open'" type="button" :disabled="busy" @click="command({ action: 'set_status', status: 'open' })">重新打开</button>
                <button v-if="viewer.role >= 10 && issue.issue.status !== 'closed'" type="button" :disabled="busy" @click="command({ action: 'set_status', status: 'closed' })">关闭主题</button>
              </div>
            </form>
          </div>
          <div v-else-if="!issueID" class="discussion-grid">
            <section class="card"><h2>主题</h2><ul class="plain-list topics"><li v-for="topic in topics" :key="topic.id"><a :href="'#feedback/' + topic.id"><div class="heading-row"><strong>{{ topic.title }}</strong><span class="tag">{{ statusLabel(topic.status) }}</span></div><span class="author"><img v-if="avatarHandles[topic.owner_id]" class="avatar" :src="avatarURL(avatarHandles[topic.owner_id])" alt="" />{{ topic.owner_username || '成员' }} · {{ formatDate(topic.updated_at) }}</span></a></li><li v-if="!topics.length" class="muted">暂无讨论。</li></ul>
              <div v-if="topicTotal > topicPageSize" class="controls">
                <button type="button" :disabled="busy || topicOffset === 0" @click="topicOffset = Math.max(0, topicOffset - topicPageSize)">上一页</button>
                <button type="button" :disabled="busy || topicOffset + topicPageSize >= topicTotal" @click="topicOffset += topicPageSize">下一页</button>
              </div>
            </section>
            <form class="card form" @submit.prevent="submitTopic">
              <h2>新建主题</h2>
              <label>标题<input v-model="topicDraft.title" required maxlength="200" /></label>
              <label>分类<select v-model="topicDraft.category"><option value="bug">问题反馈</option><option value="model">模型</option><option value="access">权益</option><option value="suggestion">建议</option><option value="other">其他</option></select></label>
              <label>内容<textarea v-model="topicDraft.description" rows="5" required /></label>
              <label>请求 ID<input v-model="topicDraft.request_id" maxlength="64" /></label>
              <button v-if="topicDraft.request_id.trim()" type="button" @click="previewDiagnostic" :disabled="busy">预览请求诊断</button>
              <pre v-if="diagnostic">{{ diagnostic.summary }}</pre>
              <label v-if="diagnostic" class="check"><input v-model="topicDraft.include_diagnostics" type="checkbox" />附上脱敏摘要，其他成员可见</label>
              <button class="primary" :disabled="busy">发布主题</button>
            </form>
          </div>
        </template>

        <template v-else-if="section === 'activity'">
          <div class="heading-row"><h1>团队活动</h1><select v-model.number="activityDays" aria-label="统计周期"><option :value="7">最近 7 天</option><option :value="30">最近 30 天</option></select></div>
          <section class="card">
            <h2>估算活动时间</h2><p class="muted">按 UTC 划分，每有一个调用成功的五分钟时段，就计入五分钟。</p>
            <p v-if="leaderboard?.suppressed" class="muted">统计周期内的活跃成员少于三人。</p>
            <ol v-else class="plain-list ranks"><li v-for="row in leaderboard?.members" :key="row.id"><span>{{ row.rank }}</span><strong>{{ row.label }}</strong><span>{{ row.estimated_minutes }} 分钟</span></li></ol>
          </section>
        </template>

        <template v-else-if="section === 'profile'">
          <h1>个人设置</h1>
          <section class="card form">
            <h2>头像</h2><div class="avatar avatar-large"><img v-if="myAvatar" :src="avatarURL(myAvatar)" alt="" /><span v-else>{{ viewer.username.slice(0, 1).toUpperCase() }}</span></div>
            <label>更新头像<input type="file" accept="image/png,image/jpeg,image/webp" @change="changeAvatar" /></label>
            <button v-if="myAvatar" :disabled="busy" @click="action(async () => { profile = { ...profile, ...await api('/api/arca/profile/avatar', { method: 'DELETE' }) } })">删除头像</button>
            <label v-if="profile" class="check"><input type="checkbox" :checked="profile.leaderboard_anonymous" :disabled="busy" @change="event => action(async () => { profile = { ...profile, ...await api('/api/arca/profile/privacy', { method: 'PUT', body: { leaderboard_anonymous: event.target.checked } }) } })" />在团队活动榜匿名显示</label>
          </section>
          <section class="card"><h2>Token 使用量</h2><p v-if="heatmap?.available" class="muted">按 UTC 统计已记录的用量。</p><p v-else class="muted">本站暂未提供用量统计。</p>
            <div v-if="heatmap?.available" class="heatmap" aria-label="Token 使用量热力图">
              <span v-for="day in heatmap.days" :key="day.date" :title="`${day.date} · ${day.tokens.toLocaleString('zh-CN')}`" :style="{ '--level': day.tokens ? 0.2 + 0.8 * Math.sqrt(day.tokens / greatest) : 0 }" />
            </div>
          </section>
        </template>

        <template v-else-if="section === 'entitlements'">
          <h1>组织与权益</h1>
          <form v-if="policyVersion !== null" class="card form" @submit.prevent="savePolicy">
            <h2>权益策略</h2>
            <label>组织、模型家族和预算<textarea v-model="policyText" rows="16" spellcheck="false" /></label>
            <div class="controls">
              <label>预览用户 ID<input v-model="previewUser" inputmode="numeric" /></label>
              <label>API Key ID<input v-model="previewKey" inputmode="numeric" /></label>
            </div>
            <pre v-if="policyPreview">{{ JSON.stringify(policyPreview, null, 2) }}</pre>
            <div class="controls">
              <button type="button" :disabled="busy || Number(previewUser) <= 0" @click="previewPolicy">预览权益</button>
              <button class="primary" :disabled="busy">保存版本 {{ policyVersion }}</button>
              <button type="button" :disabled="busy" @click="loadPage">重新加载</button>
            </div>
          </form>
        </template>

        <template v-else>
          <h1>Agent 协作</h1>
          <section v-if="viewer.role >= 10" class="card"><h2>Arca MCP</h2><p>使用 New API 的细粒度管理令牌，授权 Agent 查看反馈和请求诊断。</p><pre>/mcp</pre>
            <a href="/profile" class="secondary"><BookOpen :size="17" /> 管理访问令牌</a>
          </section>
          <p v-else class="muted">管理员可以为获授权的 Agent 配置 MCP 工具。</p>
        </template>
      </div>
    </div>
  </div>
</template>
