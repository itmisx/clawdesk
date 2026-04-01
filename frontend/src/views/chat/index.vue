<template>
  <div class="chat-wrapper">
    <!-- 助手列表 -->
    <div class="bot-sidebar">
      <div class="bot-sidebar-header">
        <div class="create-bot-btn" @click="showCreateDialog = true">
          <t-icon name="add-circle" size="16px" />
          <span>{{ t('bot.createTitle') }}</span>
        </div>
      </div>
      <div class="bot-list">
        <div
          v-for="(bot, index) in sessions"
          :key="bot.id"
          class="bot-card"
          :class="{ active: currentSessionId === bot.id, 'drag-over': dragOverIndex === index }"
          draggable="true"
          @dragstart="onDragStart(index, $event)"
          @dragover.prevent="onDragOver(index)"
          @dragleave="onDragLeave"
          @drop="onDrop(index)"
          @dragend="onDragEnd"
          @click="switchSession(bot.id)"
        >
          <div class="bot-card-top">
            <div class="bot-avatar-wrap">
              <img v-if="isImageAvatar(bot.avatar)" :src="bot.avatar" class="bot-avatar-img" />
              <span v-else class="bot-avatar-emoji">{{ bot.avatar || '🤖' }}</span>
            </div>
            <div class="bot-card-body">
              <span class="bot-card-name">{{ bot.name }}</span>
              <span class="bot-card-desc">{{ bot.description || t('bot.defaultDesc') }}</span>
              <span class="bot-card-model">{{ bot.model || t('bot.useGlobal') }}</span>
            </div>
            <div class="bot-card-more" @click.stop="openEditDialog(bot)">
              <t-icon name="ellipsis" size="14px" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 聊天区域 -->
    <div class="chat-area">
      <div v-if="!currentSessionId" class="empty-state">
        <t-icon name="chat-message" size="48px" style="color: var(--td-text-color-placeholder)" />
        <p>{{ t('chat.selectOrCreate') }}</p>
        <t-button theme="primary" variant="outline" @click="showCreateDialog = true">
          <template #icon><t-icon name="add" /></template>
          {{ t('bot.createTitle') }}
        </t-button>
      </div>
      <template v-else>
        <div class="chat-container">
          <!-- 无消息时的欢迎页 -->
          <div v-if="chatListData.length === 0" class="welcome-wrapper">
            <div class="welcome-msg">
              <img v-if="isImageAvatar(currentBot?.avatar)" :src="currentBot?.avatar" class="welcome-avatar welcome-avatar-img" />
              <span v-else class="welcome-avatar">{{ currentBot?.avatar || '🤖' }}</span>
              <h3>{{ currentBot?.name }}</h3>
              <p>{{ currentBot?.description || t('chat.welcome') }}</p>
            </div>
          </div>

          <!-- 消息列表 -->
          <ChatList
            v-else
            ref="chatListRef"
            :data="chatListData"
            :auto-scroll="true"
            :show-scroll-button="true"
            :clear-history="false"
            layout="both"
          >
            <template #actions="{ item }">
              <t-button variant="text" shape="square" size="small" @click="copyMessage(item)">
                <template #icon><t-icon name="file-copy" size="14px" /></template>
              </t-button>
            </template>
          </ChatList>

          <!-- 查看执行过程按钮 -->
          <div v-if="chatMessages.length > 0 && !streaming" class="trace-bar">
            <t-button variant="text" size="small" @click="openTrace">
              <template #icon><t-icon name="view-module" size="14px" /></template>
              查看执行过程
            </t-button>
          </div>

          <!-- 附件预览 -->
          <div v-if="attachments.length > 0" class="attachments-bar">
            <div v-for="(att, i) in attachments" :key="i" class="attachment-item">
              <img v-if="att.type === 'image'" :src="att.preview" class="attachment-thumb" />
              <t-icon v-else name="file" size="16px" />
              <span class="attachment-name">{{ att.name }}</span>
              <span class="attachment-size">{{ formatSize(att.size) }}</span>
              <t-icon name="close" size="14px" class="attachment-remove" @click="attachments.splice(i, 1)" />
            </div>
          </div>

          <!-- 输入区域 -->
          <div class="input-area" @keydown="handleKeydown" @compositionstart="onCompositionStart" @compositionend="onCompositionEnd">
            <input ref="fileInputRef" type="file" multiple hidden @change="handleFileSelect" />
            <ChatSender
              v-model="inputValue"
              :placeholder="t('chat.inputPlaceholder', { shortcut: isMac ? '⌘' : 'Ctrl' })"
              :loading="streaming"
              :disabled="streaming"
              @send="handleChatSend"
              @stop="handleStop"
            >
              <template #footerPrefix>
                <t-button variant="text" shape="square" size="small" @click="fileInputRef?.click()" :disabled="streaming">
                  <template #icon><t-icon name="attach" size="18px" /></template>
                </t-button>
              </template>
            </ChatSender>
          </div>
        </div>
      </template>
    </div>

    <!-- 助手设置抽屉（基本信息 + 定时任务） -->
    <t-drawer
      v-model:visible="showCreateDialog"
      :header="editingBot ? t('bot.editTitle') : t('bot.createTitle')"
      size="520px"
      :footer="true"
      @close="resetBotForm"
    >
      <t-tabs v-model="botDrawerTab">
        <!-- 基本信息 -->
        <t-tab-panel value="info" :label="t('bot.basicInfo')">
          <div class="bot-form" style="margin-top: 8px">
            <div class="form-row">
              <label>{{ t('bot.avatar') }}</label>
              <div class="avatar-picker">
                <span
                  v-for="emoji in avatarList"
                  :key="emoji"
                  class="avatar-option"
                  :class="{ selected: botForm.avatar === emoji }"
                  @click="botForm.avatar = emoji"
                >{{ emoji }}</span>
                <span class="avatar-option avatar-upload-btn" :class="{ selected: isImageAvatar(botForm.avatar) }" @click="triggerAvatarUpload">
                  <img v-if="isImageAvatar(botForm.avatar)" :src="botForm.avatar" class="avatar-upload-preview" />
                  <t-icon v-else name="upload" />
                </span>
                <input ref="avatarFileInput" type="file" accept="image/*" style="display:none" @change="handleAvatarUpload" />
              </div>
            </div>
            <div class="form-row">
              <label>{{ t('bot.name') }}</label>
              <t-input v-model="botForm.name" :placeholder="t('bot.namePlaceholder')" />
            </div>
            <div class="form-row">
              <label>{{ t('bot.description') }}</label>
              <t-input v-model="botForm.description" :placeholder="t('bot.descPlaceholder')" />
            </div>
            <div class="form-row">
              <label>{{ t('bot.bindModel') }}</label>
              <div class="model-bind">
                <t-select v-model="botForm.providerId" :placeholder="t('bot.useGlobal')" clearable style="width: 180px">
                  <t-option v-for="p in providers" :key="p.id" :value="p.id" :label="p.name" />
                </t-select>
                <t-select v-model="botForm.model" :placeholder="t('model.selectModel')" clearable style="flex: 1" v-if="botForm.providerId">
                  <t-option v-for="m in boundProviderModels" :key="m" :value="m" :label="m" />
                </t-select>
              </div>
            </div>
            <div class="form-row">
              <label>{{ t('bot.systemPrompt') }}</label>
              <t-textarea
                v-model="botForm.systemPrompt"
                :placeholder="t('bot.systemPromptPlaceholder')"
                :autosize="{ minRows: 3, maxRows: 8 }"
              />
            </div>
          </div>
        </t-tab-panel>

        <!-- 定时任务（仅编辑模式显示） -->
        <t-tab-panel v-if="editingBot" value="schedule" :label="t('bot.schedule')">
          <div style="margin-top: 8px">
            <!-- 任务列表 -->
            <div v-if="!showAddTask">
              <t-button theme="primary" block variant="dashed" style="margin-bottom: 12px" @click="showAddTask = true">
                <template #icon><t-icon name="add" /></template>
                {{ t('bot.addTask') }}
              </t-button>

              <div v-if="scheduleTasks.length === 0" style="text-align: center; color: var(--td-text-color-placeholder); padding: 40px 0;">
                {{ t('bot.noTasks') }}
              </div>

              <div v-for="task in scheduleTasks" :key="task.id" class="sched-task-card">
                <div class="sched-task-header">
                  <span class="sched-task-name">{{ task.name }}</span>
                  <t-switch :value="task.enabled" size="small" @change="(val: boolean) => handleToggleTask(task.id, val)" />
                </div>
                <div class="sched-task-prompt">{{ task.prompt }}</div>
                <div class="sched-task-meta">
                  <t-tag size="small" variant="light">
                    {{ task.schedule.type === 'interval' ? `${task.schedule.interval} min` : task.schedule.dailyAt }}
                  </t-tag>
                  <t-tag size="small" variant="light" v-if="task.schedule.repeatType === 'count'">
                    {{ task.runCount }}/{{ task.schedule.repeatCount }}
                  </t-tag>
                  <t-tag size="small" variant="light" v-else-if="task.schedule.repeatType === 'days'">
                    {{ task.schedule.repeatDays }}d
                  </t-tag>
                  <t-tag size="small" variant="light" theme="success" v-if="task.notify.enabled">
                    {{ task.notify.type === 'wecom' ? t('bot.wecom') : t('bot.feishu') }}
                  </t-tag>
                </div>
                <div v-if="task.lastRunAt" class="sched-task-last">
                  {{ t('bot.lastRun') }}: {{ task.runCount }}x
                </div>
                <div class="sched-task-actions">
                  <t-button variant="text" size="small" @click="openEditTask(task)">{{ t('bot.edit') }}</t-button>
                  <t-button variant="text" size="small" theme="danger" @click="handleDeleteTask(task.id)">{{ t('bot.delete') }}</t-button>
                </div>
              </div>
            </div>

            <!-- 添加/编辑任务表单 -->
            <div v-else class="sched-form">
              <div class="sched-form-header">
                <span>{{ editingTask ? t('bot.editTask') : t('bot.addTask') }}</span>
                <t-button variant="text" size="small" @click="resetTaskForm">{{ t('bot.backToList') }}</t-button>
              </div>

              <div class="form-row">
                <label>{{ t('bot.taskName') }}</label>
                <t-input v-model="taskForm.name" :placeholder="t('bot.taskNamePlaceholder')" />
              </div>
              <div class="form-row">
                <label>{{ t('bot.taskPrompt') }}</label>
                <t-textarea v-model="taskForm.prompt" :placeholder="t('bot.taskPromptPlaceholder')" :autosize="{ minRows: 2, maxRows: 5 }" />
              </div>

              <div class="form-row">
                <label>{{ t('bot.scheduleType') }}</label>
                <t-radio-group v-model="taskForm.scheduleType" variant="default-filled">
                  <t-radio-button value="interval">{{ t('bot.interval') }}</t-radio-button>
                  <t-radio-button value="daily">{{ t('bot.daily') }}</t-radio-button>
                </t-radio-group>
              </div>
              <div class="form-row" v-if="taskForm.scheduleType === 'interval'">
                <label>{{ t('bot.intervalMinutes') }}</label>
                <t-input-number v-model="taskForm.interval" :min="1" :max="10080" theme="normal" />
              </div>
              <div class="form-row" v-if="taskForm.scheduleType === 'daily'">
                <label>{{ t('bot.dailyAt') }}</label>
                <t-input v-model="taskForm.dailyAt" placeholder="HH:MM" />
              </div>

              <div class="form-row">
                <label>{{ t('bot.repeat') }}</label>
                <t-radio-group v-model="taskForm.repeatType" variant="default-filled">
                  <t-radio-button value="forever">{{ t('bot.forever') }}</t-radio-button>
                  <t-radio-button value="days">{{ t('bot.byDays') }}</t-radio-button>
                  <t-radio-button value="count">{{ t('bot.byCount') }}</t-radio-button>
                </t-radio-group>
              </div>
              <div class="form-row" v-if="taskForm.repeatType === 'days'">
                <label>{{ t('bot.repeatDays') }}</label>
                <t-input-number v-model="taskForm.repeatDays" :min="1" :max="365" theme="normal" />
              </div>
              <div class="form-row" v-if="taskForm.repeatType === 'count'">
                <label>{{ t('bot.repeatCount') }}</label>
                <t-input-number v-model="taskForm.repeatCount" :min="1" :max="10000" theme="normal" />
              </div>

              <div class="form-row">
                <label>{{ t('bot.notify') }}</label>
                <t-switch v-model="taskForm.notifyEnabled" />
              </div>
              <template v-if="taskForm.notifyEnabled">
                <div class="form-row">
                  <label>{{ t('bot.notifyChannel') }}</label>
                  <t-radio-group v-model="taskForm.notifyType" variant="default-filled">
                    <t-radio-button value="wecom">{{ t('bot.wecom') }}</t-radio-button>
                    <t-radio-button value="feishu">{{ t('bot.feishu') }}</t-radio-button>
                  </t-radio-group>
                </div>
                <div class="form-row">
                  <label>Webhook URL</label>
                  <t-input v-model="taskForm.webhook" placeholder="https://..." />
                </div>
              </template>

              <t-button theme="primary" block style="margin-top: 16px" @click="handleSaveTask">
                {{ editingTask ? t('bot.save') : t('bot.create') }}
              </t-button>
            </div>
          </div>
        </t-tab-panel>
      </t-tabs>

      <template #footer>
        <div style="display: flex; justify-content: space-between; width: 100%;">
          <t-button v-if="editingBot" theme="danger" variant="text" @click="handleDeleteFromDrawer">
            {{ t('bot.delete') }}
          </t-button>
          <span v-else />
          <t-space>
            <t-button @click="showCreateDialog = false">{{ t('common.cancel') }}</t-button>
            <t-button theme="primary" @click="handleSaveBot" v-if="botDrawerTab === 'info'">
              {{ editingBot ? t('bot.save') : t('bot.create') }}
            </t-button>
          </t-space>
        </div>
      </template>
    </t-drawer>

    <!-- 执行追踪抽屉 -->
    <t-drawer
      v-model:visible="showTraceDrawer"
      header="执行过程"
      size="520px"
    >
      <div v-if="currentTrace" class="trace-content">
        <div class="trace-summary">
          <div class="trace-query">
            <span class="trace-label">问题：</span>{{ currentTrace.plan?.query }}
          </div>
          <div class="trace-plan-info">
            <span class="trace-label">策略：</span>{{ currentTrace.plan?.summary }}
            <t-tag v-if="currentTrace.plan?.duration" size="small" variant="light">{{ currentTrace.plan?.duration }}ms</t-tag>
          </div>
        </div>

        <!-- 步骤列表 -->
        <div v-if="currentTrace.plan?.steps?.length" class="trace-steps">
          <div v-for="step in currentTrace.plan.steps" :key="step.id" class="trace-step" :class="'trace-step--' + step.status">
            <div class="trace-step-header">
              <span class="trace-step-icon">
                <t-icon v-if="step.status === 'done'" name="check-circle-filled" style="color: var(--td-success-color)" />
                <t-icon v-else-if="step.status === 'failed'" name="close-circle-filled" style="color: var(--td-error-color)" />
                <t-icon v-else name="time" style="color: var(--td-text-color-placeholder)" />
              </span>
              <span class="trace-step-name">{{ step.name }}</span>
              <t-tag size="small" variant="outline">{{ step.agentRole }}</t-tag>
              <span v-if="step.duration" class="trace-step-dur">{{ step.duration }}ms</span>
            </div>
            <div class="trace-step-desc">{{ step.description }}</div>

            <!-- 工具调用 -->
            <div v-if="step.toolCalls?.length" class="trace-tools">
              <div v-for="(tc, i) in step.toolCalls" :key="i" class="trace-tool">
                <span class="trace-tool-name">
                  <t-icon name="tools" size="12px" />
                  {{ tc.toolName }}
                </span>
                <t-tag :theme="tc.success ? 'success' : 'danger'" size="small" variant="light">
                  {{ tc.success ? '成功' : '失败' }}
                </t-tag>
                <span v-if="tc.duration" class="trace-tool-dur">{{ tc.duration }}ms</span>
                <div v-if="tc.result" class="trace-tool-result">{{ tc.result.slice(0, 200) }}{{ tc.result.length > 200 ? '...' : '' }}</div>
              </div>
            </div>
          </div>
        </div>

        <!-- Mermaid 流程图 -->
        <div v-if="currentTrace.plan?.mermaid" class="trace-mermaid">
          <div class="trace-label">执行流程图</div>
          <div class="mermaid-chart" v-html="renderedMermaid" />
        </div>
      </div>
      <div v-else class="trace-empty">
        <t-icon name="info-circle" size="32px" style="color: var(--td-text-color-placeholder)" />
        <p>该问答为直接回答，无多步执行过程</p>
      </div>
    </t-drawer>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch, triggerRef } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { ChatList, ChatSender } from '@tdesign-vue-next/chat'
import '@tdesign-vue-next/chat/es/style/index.css'
import { useI18n } from 'vue-i18n'
import * as Runtime from '../../../wailsjs/runtime/runtime'
import { marked } from 'marked'
import mermaid from 'mermaid'
import { GetSessions, CreateBot, UpdateBot, DeleteSession, ReorderSessions, GetSessionHistory, SendMessage, StopGenerate, GetModelConfig, GetExecutionTrace, GetScheduledTasks, AddScheduledTask, UpdateScheduledTask, DeleteScheduledTask, SetScheduledTaskEnabled } from '../../../wailsjs/go/agent/App'

// 初始化 mermaid
mermaid.initialize({ startOnLoad: false, theme: 'default', securityLevel: 'loose' })

// 语言 → 文件扩展名映射
const langExtMap: Record<string, string> = {
  go: 'go', python: 'py', py: 'py', javascript: 'js', js: 'js', typescript: 'ts', ts: 'ts',
  java: 'java', rust: 'rs', c: 'c', cpp: 'cpp', html: 'html', css: 'css', json: 'json',
  yaml: 'yaml', yml: 'yaml', xml: 'xml', sql: 'sql', sh: 'sh', bash: 'sh', shell: 'sh',
  markdown: 'md', md: 'md', ruby: 'rb', php: 'php', swift: 'swift', kotlin: 'kt',
}

// 配置 marked：mermaid 图表 + 代码块复制/下载按钮
let mermaidCounter = 0
const renderer = new marked.Renderer()
renderer.code = function({ text, lang }: { text: string; lang?: string }) {
  if (lang === 'mermaid') {
    const id = `mermaid-${Date.now()}-${mermaidCounter++}`
    return `<div class="mermaid-wrapper"><div class="mermaid-chart" data-mermaid-id="${id}">${text}</div><details class="mermaid-source"><summary>Source</summary><pre><code>${text}</code></pre></details></div>`
  }
  const ext = langExtMap[lang || ''] || 'txt'
  const escaped = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return `<div class="code-block"><div class="code-block-header"><span class="code-lang">${lang || ''}</span><span class="code-actions"><button class="code-btn" onclick="navigator.clipboard.writeText(this.closest('.code-block').querySelector('code').textContent)">Copy</button><button class="code-btn" onclick="(function(el){var c=el.closest('.code-block').querySelector('code').textContent;var b=new Blob([c],{type:'text/plain'});var a=document.createElement('a');a.href=URL.createObjectURL(b);a.download='code.${ext}';a.click()})(this)">Download</button></span></div><pre><code>${escaped}</code></pre></div>`
}
marked.setOptions({ breaks: true, gfm: true, renderer })

const { t } = useI18n()
const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0

interface ChatMessage { role: string; content: string; timestamp?: string }
interface BotSession { id: string; name: string; avatar: string; description: string; providerId: string; model: string }
interface ModelProvider { id: string; name: string; models: string[] }

const sessions = ref<BotSession[]>([])
const currentSessionId = ref('')

// 拖拽排序
const dragIndex = ref(-1)
const dragOverIndex = ref(-1)
const onDragStart = (index: number, e: DragEvent) => {
  dragIndex.value = index
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
  }
}
const onDragOver = (index: number) => { dragOverIndex.value = index }
const onDragLeave = () => { dragOverIndex.value = -1 }
const onDrop = (targetIndex: number) => {
  const from = dragIndex.value
  if (from < 0 || from === targetIndex) return
  const [item] = sessions.value.splice(from, 1)
  if (!item) return
  sessions.value.splice(targetIndex, 0, item)
  ReorderSessions(sessions.value.map(s => s.id))
}
const onDragEnd = () => { dragIndex.value = -1; dragOverIndex.value = -1 }
const chatMessages = ref<ChatMessage[]>([])
const inputValue = ref('')
const isComposing = ref(false)
// per-session streaming 状态
interface SessionStreamState {
  content: string
  tokenBuffer: string
  tokenFlushTimer: ReturnType<typeof setTimeout> | null
  callingTool: string
  activeToolCount: number
  toolCallLogs: string[]
  timeout: ReturnType<typeof setTimeout> | null
}
const streamingSessions = ref<Map<string, SessionStreamState>>(new Map())
const streaming = computed(() => streamingSessions.value.has(currentSessionId.value))
const streamingContent = computed(() => streamingSessions.value.get(currentSessionId.value)?.content || '')
const callingTool = computed(() => streamingSessions.value.get(currentSessionId.value)?.callingTool || '')
const activeToolCount = computed(() => streamingSessions.value.get(currentSessionId.value)?.activeToolCount || 0)
const toolCallLogs = computed(() => streamingSessions.value.get(currentSessionId.value)?.toolCallLogs || [])
const chatListRef = ref()
const fileInputRef = ref<HTMLInputElement>()

// 附件
interface AttachmentItem {
  name: string; size: number; type: 'text' | 'image' | 'other'
  content: string; preview?: string
}
const attachments = ref<AttachmentItem[]>([])

// 执行追踪
const showTraceDrawer = ref(false)
const currentTrace = ref<any>(null)
const renderedMermaid = ref('')
const lastTraceTs = ref('') // 最新一次对话的 messageTs

const openTrace = async () => {
  if (!currentSessionId.value || !lastTraceTs.value) return
  try {
    const trace = await GetExecutionTrace(currentSessionId.value, lastTraceTs.value)
    currentTrace.value = trace
    if (trace?.plan?.mermaid) {
      try {
        const { svg } = await mermaid.render('trace-mermaid-' + Date.now(), trace.plan.mermaid)
        renderedMermaid.value = svg
      } catch { renderedMermaid.value = '' }
    }
    showTraceDrawer.value = true
  } catch { showTraceDrawer.value = true; currentTrace.value = null }
}

// 定时任务
interface SchedTask { id: string; sessionId: string; name: string; prompt: string; enabled: boolean; schedule: { type: string; interval: number; dailyAt: string; repeatType: string; repeatDays: number; repeatCount: number }; notify: { enabled: boolean; type: string; webhook: string }; runCount: number; lastRunAt: string; lastResult: string; lastError: string }
const showScheduleDrawer = ref(false)
const scheduleBot = ref<BotSession | null>(null)
const scheduleTasks = ref<SchedTask[]>([])
const showAddTask = ref(false)
const editingTask = ref<SchedTask | null>(null)
const taskForm = ref({
  name: '', prompt: '',
  scheduleType: 'interval', interval: 30, dailyAt: '09:00',
  repeatType: 'forever', repeatDays: 7, repeatCount: 10,
  notifyEnabled: false, notifyType: 'wecom', webhook: '',
})

const openScheduleDrawer = async (bot: BotSession) => {
  openEditDialog(bot)
  botDrawerTab.value = 'schedule'
  try { scheduleTasks.value = (await GetScheduledTasks(bot.id)) || [] } catch { scheduleTasks.value = [] }
}

const resetTaskForm = () => {
  editingTask.value = null
  showAddTask.value = false
  taskForm.value = { name: '', prompt: '', scheduleType: 'interval', interval: 30, dailyAt: '09:00', repeatType: 'forever', repeatDays: 7, repeatCount: 10, notifyEnabled: false, notifyType: 'wecom', webhook: '' }
}

const openEditTask = (task: SchedTask) => {
  editingTask.value = task
  showAddTask.value = true
  taskForm.value = {
    name: task.name, prompt: task.prompt,
    scheduleType: task.schedule.type, interval: task.schedule.interval, dailyAt: task.schedule.dailyAt,
    repeatType: task.schedule.repeatType, repeatDays: task.schedule.repeatDays, repeatCount: task.schedule.repeatCount,
    notifyEnabled: task.notify.enabled, notifyType: task.notify.type, webhook: task.notify.webhook,
  }
}

const handleSaveTask = async () => {
  if (!scheduleBot.value || !taskForm.value.name || !taskForm.value.prompt) return
  const f = taskForm.value
  const data: any = {
    sessionId: scheduleBot.value.id, name: f.name, prompt: f.prompt, enabled: true,
    schedule: { type: f.scheduleType, interval: f.interval, dailyAt: f.dailyAt, repeatType: f.repeatType, repeatDays: f.repeatDays, repeatCount: f.repeatCount },
    notify: { enabled: f.notifyEnabled, type: f.notifyType, webhook: f.webhook },
  }
  try {
    if (editingTask.value) { data.id = editingTask.value.id; await UpdateScheduledTask(data) }
    else { await AddScheduledTask(data) }
    resetTaskForm()
    scheduleTasks.value = (await GetScheduledTasks(scheduleBot.value.id)) || []
  } catch (e: any) { MessagePlugin.error('保存失败: ' + e) }
}

const handleDeleteTask = async (taskID: string) => {
  try {
    await DeleteScheduledTask(taskID)
    if (scheduleBot.value) scheduleTasks.value = (await GetScheduledTasks(scheduleBot.value.id)) || []
  } catch (e: any) { MessagePlugin.error('删除失败: ' + e) }
}

const handleToggleTask = async (taskID: string, enabled: boolean) => {
  try {
    await SetScheduledTaskEnabled(taskID, enabled)
    if (scheduleBot.value) scheduleTasks.value = (await GetScheduledTasks(scheduleBot.value.id)) || []
  } catch (e: any) { MessagePlugin.error('操作失败: ' + e) }
}

const TEXT_EXTS = ['.txt','.md','.json','.csv','.yaml','.yml','.xml','.log','.go','.py','.js','.ts','.jsx','.tsx','.html','.css','.sh','.sql','.java','.c','.cpp','.h','.rs','.rb','.php','.swift','.kt','.toml','.ini','.cfg','.env']
const IMAGE_EXTS = ['.png','.jpg','.jpeg','.gif','.webp','.bmp','.svg']

const handleFileSelect = (e: Event) => {
  const files = (e.target as HTMLInputElement).files
  if (!files) return
  for (const file of files) {
    const ext = '.' + file.name.split('.').pop()?.toLowerCase()
    if (IMAGE_EXTS.includes(ext)) {
      const reader = new FileReader()
      reader.onload = () => {
        attachments.value.push({ name: file.name, size: file.size, type: 'image', content: reader.result as string, preview: reader.result as string })
      }
      reader.readAsDataURL(file)
    } else if (TEXT_EXTS.includes(ext)) {
      const reader = new FileReader()
      reader.onload = () => {
        attachments.value.push({ name: file.name, size: file.size, type: 'text', content: reader.result as string })
      }
      reader.readAsText(file)
    } else {
      attachments.value.push({ name: file.name, size: file.size, type: 'other', content: '' })
    }
  }
  ;(e.target as HTMLInputElement).value = '' // 清空以便重复选择同文件
}

const formatSize = (bytes: number): string => {
  if (bytes < 1024) return bytes + 'B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + 'KB'
  return (bytes / 1024 / 1024).toFixed(1) + 'MB'
}

// 输入历史（类似 shell）
const inputHistory = ref<string[]>([])
const historyIndex = ref(-1) // -1 表示当前输入，0 是最近一条
const savedInput = ref('') // 按上键前保存当前未发送的输入

// 助手表单
const showCreateDialog = ref(false)
const editingBot = ref<BotSession | null>(null)
const botDrawerTab = ref('info')
const providers = ref<ModelProvider[]>([])
const botForm = ref({ name: '', avatar: '🤖', description: '', systemPrompt: '', providerId: '', model: '' })

const avatarList = ['🤖', '💻', '📝', '🔍', '🎨', '📊', '🌐', '🧠', '⚡', '🎯', '🛠️', '📚', '🎵', '🏥', '📸', '🧪', '🐦', '💬']

// 判断是否为自定义图片头像
const isImageAvatar = (avatar?: string) => avatar && avatar.startsWith('data:image/')

// 将 emoji 转为 SVG data URL 供 ChatItem avatar 使用
const emojiToAvatar = (emoji: string) => {
  if (isImageAvatar(emoji)) return emoji
  return `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"><rect width="40" height="40" rx="20" fill="transparent"/><text x="20" y="28" text-anchor="middle" font-size="26">${emoji}</text></svg>`)}`
}
const userAvatarUrl = emojiToAvatar('👤')
const botAvatarUrl = computed(() => emojiToAvatar(currentBot.value?.avatar || '🤖'))

// 自定义头像上传
const avatarFileInput = ref<HTMLInputElement | null>(null)
const triggerAvatarUpload = () => avatarFileInput.value?.click()
const handleAvatarUpload = (e: Event) => {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  if (file.size > 2 * 1024 * 1024) {
    MessagePlugin.warning(t('bot.avatarTooLarge'))
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    botForm.value.avatar = reader.result as string
  }
  reader.readAsDataURL(file)
  ;(e.target as HTMLInputElement).value = ''
}

const currentBot = computed(() => sessions.value.find(s => s.id === currentSessionId.value))

const boundProviderModels = computed(() => {
  const p = providers.value.find(p => p.id === botForm.value.providerId)
  return p?.models || []
})

// 构建 ChatList 内容项
const makeContent = (text: string, type: 'text' | 'markdown' = 'markdown') => {
  return [{ type, data: text }]
}

// ChatList 数据：合并历史消息 + 工具调用 + 流式内容
const nowTime = () => {
  const now = new Date()
  return now.getHours().toString().padStart(2, '0') + ':' + now.getMinutes().toString().padStart(2, '0')
}

// 历史消息（只在 chatMessages 变化时重建，流式输出期间不变）
const historyItems = computed(() => {
  const items: any[] = []
  const botName = currentBot.value?.name || 'AI'
  for (const msg of chatMessages.value) {
    items.push({
      role: msg.role,
      content: makeContent(msg.content),
      avatar: msg.role === 'user' ? userAvatarUrl : botAvatarUrl.value,
      name: msg.role === 'user' ? t('chat.you') : botName,
      datetime: msg.timestamp || '',
    })
  }
  return items
})

const chatListData = computed(() => {
  const items = [...historyItems.value]
  const botName = currentBot.value?.name || 'AI'

  // 工具调用记录
  for (const tc of toolCallLogs.value) {
    items.push({ role: 'system', content: makeContent(tc, 'text') })
  }

  // 正在调用工具
  if (callingTool.value) {
    items.push({ role: 'system', content: makeContent(t('chat.callingTool', { name: callingTool.value }), 'text') })
  }

  // 流式内容（用轻量文本渲染，避免每个 token 都触发 marked.parse）
  if (streaming.value && streamingContent.value) {
    items.push({
      role: 'assistant',
      content: makeContent(streamingContent.value, 'text'),
      avatar: botAvatarUrl.value,
      name: botName,
      datetime: nowTime(),
    })
  }

  // 思考中
  if (streaming.value && !streamingContent.value && !callingTool.value) {
    items.push({
      role: 'assistant',
      content: makeContent(t('chat.thinking') + '...', 'text'),
      avatar: botAvatarUrl.value,
      name: botName,
    })
  }

  return items
})

// 判断是否为流式渲染项（最后一个 assistant 消息且正在流式输出）
const isStreamingItem = (index: number) => {
  return streaming.value && streamingContent.value && index === chatListData.value.length - 1
}

// 判断是否为思考中项
const isThinkingItem = (index: number) => {
  return streaming.value && !streamingContent.value && !callingTool.value && index === chatListData.value.length - 1
}

const openEditDialog = (bot: BotSession) => {
  editingBot.value = bot
  scheduleBot.value = bot
  botDrawerTab.value = 'info'
  botForm.value = { name: bot.name, avatar: bot.avatar || '🤖', description: bot.description, systemPrompt: '', providerId: bot.providerId || '', model: bot.model || '' }
  showCreateDialog.value = true
  // 预加载定时任务
  GetScheduledTasks(bot.id).then(tasks => { scheduleTasks.value = tasks || [] }).catch(() => { scheduleTasks.value = [] })
}

const resetBotForm = () => {
  editingBot.value = null
  botDrawerTab.value = 'info'
  botForm.value = { name: '', avatar: '🤖', description: '', systemPrompt: '', providerId: '', model: '' }
  resetTaskForm()
}

const handleSaveBot = async () => {
  if (!botForm.value.name.trim()) { MessagePlugin.warning(t('bot.namePlaceholder')); return }
  try {
    const opts = {
      name: botForm.value.name.trim(),
      avatar: botForm.value.avatar,
      description: botForm.value.description.trim(),
      systemPrompt: botForm.value.systemPrompt.trim(),
      providerId: botForm.value.providerId,
      model: botForm.value.model,
    }
    if (editingBot.value) {
      await UpdateBot(editingBot.value.id, opts as any)
    } else {
      const s = await CreateBot(opts as any)
      await loadSessions()
      switchSession(s.id)
    }
    showCreateDialog.value = false
    resetBotForm()
    await loadSessions()
  } catch (e: any) { MessagePlugin.error(t('chat.createFailed') + ': ' + e) }
}

const loadSessions = async () => {
  try { sessions.value = await GetSessions() }
  catch (e: any) { MessagePlugin.error(t('chat.loadFailed') + ': ' + e) }
}

const loadProviders = async () => {
  try { const cfg = await GetModelConfig(); providers.value = cfg.providers || [] }
  catch { /* ignore */ }
}

const switchSession = async (id: string) => {
  currentSessionId.value = id
  localStorage.setItem('clawdesk_last_session', id)
  historyIndex.value = -1
  savedInput.value = ''
  try {
    const history = await GetSessionHistory(id)
    chatMessages.value = history || []
    await nextTick()
    renderMermaidCharts()
  } catch (e: any) { MessagePlugin.error(t('chat.historyFailed') + ': ' + e) }
}

const handleDeleteBot = async (id: string) => {
  try {
    await DeleteSession(id)
    if (currentSessionId.value === id) { currentSessionId.value = ''; chatMessages.value = [] }
    await loadSessions()
  } catch (e: any) { MessagePlugin.error(t('chat.deleteFailed') + ': ' + e) }
}

const handleDeleteFromDrawer = () => {
  if (!editingBot.value) return
  const id = editingBot.value.id
  showCreateDialog.value = false
  resetBotForm()
  handleDeleteBot(id)
}

const onCompositionStart = () => { isComposing.value = true }
const onCompositionEnd = () => { setTimeout(() => { isComposing.value = false }, 100) }

const handleKeydown = (e: KeyboardEvent) => {
  const hasNewline = inputValue.value.includes('\n')

  // 上键：单行时切换历史
  if (e.key === 'ArrowUp' && !hasNewline) {
    if (inputHistory.value.length === 0) return
    e.preventDefault()
    if (historyIndex.value === -1) {
      savedInput.value = inputValue.value
      historyIndex.value = 0
    } else if (historyIndex.value < inputHistory.value.length - 1) {
      historyIndex.value++
    } else {
      return
    }
    inputValue.value = inputHistory.value[inputHistory.value.length - 1 - historyIndex.value] ?? ''
    return
  }

  // 下键：单行时切换历史
  if (e.key === 'ArrowDown' && !hasNewline && historyIndex.value >= 0) {
    e.preventDefault()
    historyIndex.value--
    if (historyIndex.value < 0) {
      inputValue.value = savedInput.value
      historyIndex.value = -1
    } else {
      inputValue.value = inputHistory.value[inputHistory.value.length - 1 - historyIndex.value] ?? ''
    }
    return
  }
}

// ChatSender @send 事件处理
const handleChatSend = async () => {
  if (isComposing.value) return
  const text = inputValue.value.trim()
  if ((!text && attachments.value.length === 0) || streaming.value || !currentSessionId.value) return

  // 存入输入历史
  if (text) {
    const last = inputHistory.value[inputHistory.value.length - 1]
    if (text !== last) {
      inputHistory.value.push(text)
      if (inputHistory.value.length > 50) inputHistory.value.shift()
    }
  }
  historyIndex.value = -1
  savedInput.value = ''

  // 显示用户消息（含附件摘要）
  let displayText = text
  if (attachments.value.length > 0) {
    const names = attachments.value.map(a => `📎 ${a.name}`).join(', ')
    displayText = text ? `${text}\n${names}` : names
  }
  chatMessages.value.push({ role: 'user', content: displayText })

  // 构建附件数据
  const atts = attachments.value.map(a => ({ name: a.name, type: a.type, content: a.content }))

  inputValue.value = ''
  attachments.value = []

  const sid = currentSessionId.value
  const state: SessionStreamState = {
    content: '', tokenBuffer: '', tokenFlushTimer: null,
    callingTool: '', activeToolCount: 0, toolCallLogs: [],
    timeout: setTimeout(() => { streamingSessions.value.delete(sid); triggerRef(streamingSessions) }, 5 * 60 * 1000),
  }
  streamingSessions.value.set(sid, state)
  triggerRef(streamingSessions)

  try { await SendMessage(sid, text, atts as any) }
  catch (e: any) {
    MessagePlugin.error(t('chat.sendFailed') + ': ' + e)
    streamingSessions.value.delete(sid); triggerRef(streamingSessions)
  }
}

const handleStop = async () => { try { await StopGenerate(currentSessionId.value) } catch { /* */ } }

const copyMessage = (item: any) => {
  const text = item.content?.[0]?.data || ''
  if (text) {
    navigator.clipboard.writeText(text).then(() => {
      MessagePlugin.success(t('chat.copied'))
    }).catch(() => {
      MessagePlugin.error('复制失败')
    })
  }
}

// 流式输出用轻量渲染（markdown 可能不完整，不用 marked 避免崩溃）
const renderStreamContent = (text: string): string => {
  if (!text) return ''
  let html = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>')
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>')
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\n/g, '<br/>')
  return html
}

// 完整消息用 marked 渲染
const renderMarkdown = (text: string): string => {
  if (!text) return ''
  try {
    return marked.parse(text) as string
  } catch {
    return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br/>')
  }
}

const renderMermaidCharts = async () => {
  try {
    await nextTick()
    const el = chatListRef.value?.$el || chatListRef.value
    if (!el) return
    const charts = el.querySelectorAll('.mermaid-chart:not([data-processed])')
    for (let i = 0; i < charts.length; i++) {
      const chart = charts[i]!
      const code = chart.textContent || ''
      if (!code.trim()) continue
      chart.setAttribute('data-processed', 'true')
      const id = 'mc' + Date.now().toString(36) + i
      try {
        const { svg } = await mermaid.render(id, code.trim())
        chart.innerHTML = svg
      } catch {
        const escaped = code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        chart.innerHTML = `<pre style="font-size:12px;margin:0"><code>${escaped}</code></pre>`
      }
    }
  } catch {
    // 忽略整体错误
  }
}

// 消息变化后渲染 mermaid（防抖 300ms，避免批量消息时重复渲染）
let mermaidTimer: ReturnType<typeof setTimeout> | null = null
const debouncedRenderMermaid = () => {
  if (mermaidTimer) clearTimeout(mermaidTimer)
  mermaidTimer = setTimeout(() => renderMermaidCharts(), 300)
}
watch([() => chatMessages.value.length, () => streaming.value], () => {
  if (!streaming.value) {
    nextTick(() => debouncedRenderMermaid())
  }
})

const toolNameMap = computed(() => ({
  execute_command: t('tool.execute_command'), read_file: t('tool.read_file'),
  write_file: t('tool.write_file'), list_directory: t('tool.list_directory'),
}))

onMounted(async () => {
  await loadSessions()
  await loadProviders()

  // 恢复上次的助手
  const lastId = localStorage.getItem('clawdesk_last_session')
  if (lastId && sessions.value.some(s => s.id === lastId)) {
    switchSession(lastId)
  }

  Runtime.EventsOn('llm:toolcall', (sessionId: string, toolName: string, toolArgs: string) => {
    const state = streamingSessions.value.get(sessionId)
    if (!state) return
    const displayName = (toolNameMap.value as any)[toolName] || toolName
    state.callingTool = displayName
    state.activeToolCount++
    try { const args = JSON.parse(toolArgs); state.toolCallLogs.push(`${displayName}: ${Object.values(args).join(', ')}`) }
    catch { state.toolCallLogs.push(displayName) }
    triggerRef(streamingSessions)
  })
  Runtime.EventsOn('llm:toolresult', (sessionId: string) => {
    const state = streamingSessions.value.get(sessionId)
    if (!state) return
    state.activeToolCount = Math.max(0, state.activeToolCount - 1)
    if (state.activeToolCount === 0) state.callingTool = ''
    triggerRef(streamingSessions)
  })
  Runtime.EventsOn('llm:token', (sessionId: string, token: string) => {
    const state = streamingSessions.value.get(sessionId)
    if (!state) return
    state.callingTool = ''; state.activeToolCount = 0
    state.tokenBuffer += token
    if (!state.tokenFlushTimer) {
      state.tokenFlushTimer = setTimeout(() => {
        state.content += state.tokenBuffer
        state.tokenBuffer = ''
        state.tokenFlushTimer = null
        triggerRef(streamingSessions)
      }, 100)
    }
  })
  Runtime.EventsOn('llm:done', (sessionId: string) => {
    const state = streamingSessions.value.get(sessionId)
    if (!state) return
    // 刷新残余 token
    if (state.tokenBuffer) { state.content += state.tokenBuffer; state.tokenBuffer = '' }
    if (state.tokenFlushTimer) { clearTimeout(state.tokenFlushTimer); state.tokenFlushTimer = null }
    if (state.timeout) { clearTimeout(state.timeout); state.timeout = null }
    // 如果是当前会话，将消息推入 chatMessages
    if (sessionId === currentSessionId.value && state.content) {
      chatMessages.value.push({ role: 'assistant', content: state.content })
    }
    streamingSessions.value.delete(sessionId)
    triggerRef(streamingSessions)
  })
  Runtime.EventsOn('llm:error', (sessionId: string, error: string) => {
    const state = streamingSessions.value.get(sessionId)
    if (state?.timeout) { clearTimeout(state.timeout) }
    if (state?.tokenFlushTimer) { clearTimeout(state.tokenFlushTimer) }
    streamingSessions.value.delete(sessionId)
    triggerRef(streamingSessions)
    if (sessionId === currentSessionId.value) {
      MessagePlugin.error(t('chat.aiError') + ': ' + error)
    }
  })

  // 监听助手创建事件（LLM 通过工具创建的）
  Runtime.EventsOn('bot:created', () => {
    loadSessions()
  })

  // 监听执行追踪完成事件 → 保存最新的 messageTs
  Runtime.EventsOn('llm:trace', (_sessionID: string, messageTs: string) => {
    lastTraceTs.value = messageTs
  })

  // 监听定时任务完成事件 → 刷新当前会话的聊天消息
  Runtime.EventsOn('schedule:done', (sessionID: string) => {
    if (sessionID === currentSessionId.value) {
      GetSessionHistory(sessionID).then(history => {
        chatMessages.value = history || []
        nextTick(() => renderMermaidCharts())
      })
    }
  })
})

onUnmounted(() => {
  Runtime.EventsOff('llm:token'); Runtime.EventsOff('llm:done')
  Runtime.EventsOff('llm:error'); Runtime.EventsOff('llm:toolcall'); Runtime.EventsOff('llm:toolresult')
  Runtime.EventsOff('bot:created'); Runtime.EventsOff('schedule:done'); Runtime.EventsOff('llm:trace')
})
</script>

<style lang="less" scoped>
.chat-wrapper { display: flex; height: 100%; overflow: hidden; }

// ===== 助手侧边栏 =====
.bot-sidebar {
  width: 272px; min-width: 272px;
  border-right: 1px solid var(--td-border-level-1-color);
  display: flex; flex-direction: column;
  background: var(--td-bg-color-secondarycontainer);
}
.bot-sidebar-header { padding: 12px 14px 6px; }
.create-bot-btn {
  display: flex; align-items: center; gap: 6px; justify-content: center;
  padding: 9px 0; border-radius: 10px;
  cursor: pointer; font-size: 13px; font-weight: 500;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
  border: 1px dashed var(--td-brand-color-light-hover);
  transition: all 0.2s;
  &:hover { background: var(--td-brand-color-light-hover); border-style: solid; }
}
.bot-list {
  flex: 1; overflow-y: auto; padding: 4px 10px 10px;
  &::-webkit-scrollbar { width: 3px; }
  &::-webkit-scrollbar-thumb { background: transparent; border-radius: 3px; }
  &:hover::-webkit-scrollbar-thumb { background: var(--td-scrollbar-color); }
}

.bot-card {
  padding: 10px 12px; border-radius: 12px;
  cursor: pointer; margin-bottom: 2px;
  border: 1.5px solid transparent;
  transition: all 0.18s ease;
  position: relative;
  &:hover {
    background: var(--td-bg-color-container);
    .bot-card-more { opacity: 1; }
  }
  &.active {
    background: var(--td-bg-color-container);
    border-color: var(--td-brand-color-light);
    box-shadow: 0 1px 6px rgba(0, 0, 0, 0.04);
    .bot-card-name { color: var(--td-brand-color); }
    .bot-avatar-wrap { background: var(--td-brand-color-light); }
  }
  &.drag-over {
    &::before {
      content: ''; position: absolute; top: -2px; left: 16px; right: 16px;
      height: 2px; border-radius: 1px; background: var(--td-brand-color);
    }
  }
}
.bot-card-top { display: flex; align-items: center; gap: 10px; }
.bot-avatar-wrap {
  width: 38px; height: 38px; flex-shrink: 0;
  border-radius: 10px; display: flex; align-items: center; justify-content: center;
  background: var(--td-bg-color-container); transition: background 0.2s;
  overflow: hidden;
}
.bot-avatar-emoji { font-size: 20px; line-height: 1; }
.bot-avatar-img { width: 100%; height: 100%; object-fit: cover; }
.bot-card-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.bot-card-name {
  font-size: 13px; font-weight: 600; line-height: 1.4;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  transition: color 0.2s;
}
.bot-card-desc {
  font-size: 11px; color: var(--td-text-color-placeholder); line-height: 1.3;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.bot-card-model {
  display: inline-flex; align-items: center;
  font-size: 10px; color: var(--td-text-color-disabled); line-height: 1;
  padding: 1.5px 5px; margin-top: 2px;
  background: var(--td-bg-color-container-hover); border-radius: 4px;
  max-width: fit-content;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.bot-card-more {
  opacity: 0; flex-shrink: 0; width: 24px; height: 24px; border-radius: 6px;
  display: flex; align-items: center; justify-content: center;
  color: var(--td-text-color-placeholder); cursor: pointer;
  transition: all 0.15s;
  &:hover { background: var(--td-bg-color-container-hover); color: var(--td-text-color-secondary); }
}
.bot-card.active .bot-card-more { opacity: 1; }

// ===== 聊天区域 =====
.chat-area {
  flex: 1; display: flex; flex-direction: column; overflow: hidden;
  background: var(--td-bg-color-container);
}
.empty-state {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  height: 100%; color: var(--td-text-color-placeholder); gap: 16px;
}
.chat-container { display: flex; flex-direction: column; height: 100%; }

.welcome-wrapper { flex: 1; display: flex; align-items: center; justify-content: center; }
.welcome-msg {
  text-align: center; color: var(--td-text-color-placeholder);
  h3 { margin: 10px 0 6px; color: var(--td-text-color-primary); font-size: 18px; font-weight: 600; }
  p { font-size: 13px; }
}
.welcome-avatar {
  font-size: 44px; display: flex; align-items: center; justify-content: center;
  width: 72px; height: 72px; margin: 0 auto;
  background: var(--td-bg-color-secondarycontainer); border-radius: 18px;
}
.welcome-avatar-img {
  width: 72px; height: 72px; border-radius: 18px; object-fit: cover;
  margin: 0 auto; display: block;
}

// ===== ChatList 占位 =====
:deep(.t-chat) { flex: 1; overflow: hidden; }

// 输入区域
.input-area {
  padding: 12px 20px 16px;
  background: var(--td-bg-color-container);
}

// 附件预览
.attachments-bar {
  display: flex; flex-wrap: wrap; gap: 6px; padding: 0 20px 8px;
  background: var(--td-bg-color-container);
}
.attachment-item {
  display: flex; align-items: center; gap: 4px;
  padding: 4px 8px; border-radius: 8px;
  background: var(--td-bg-color-page); border: 1px solid var(--td-border-level-1-color);
  font-size: 12px; color: var(--td-text-color-secondary);
}
.attachment-thumb { width: 32px; height: 32px; object-fit: cover; border-radius: 6px; }
.attachment-name { max-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.attachment-size { color: var(--td-text-color-placeholder); }
.attachment-remove { cursor: pointer; color: var(--td-text-color-placeholder); &:hover { color: var(--td-error-color); } }

// ===== 创建/编辑对话框 =====
.bot-form { display: flex; flex-direction: column; gap: 16px; }
.form-row {
  label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 6px; color: var(--td-text-color-secondary); }
}
.avatar-picker { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.avatar-option {
  width: 36px; height: 36px; font-size: 20px; display: flex; align-items: center; justify-content: center;
  border-radius: 8px; cursor: pointer; border: 2px solid transparent;
  transition: all 0.15s; overflow: hidden;
  &:hover { background: var(--td-bg-color-container-hover); }
  &.selected { border-color: var(--td-brand-color); background: var(--td-brand-color-light); }
}
.avatar-upload-btn { color: var(--td-text-color-placeholder); font-size: 16px; }
.avatar-upload-preview { width: 100%; height: 100%; object-fit: cover; }
.model-bind { display: flex; gap: 8px; }

// ===== 执行追踪 =====
.trace-bar {
  display: flex; justify-content: center; padding: 4px 0;
  .t-button { color: var(--td-text-color-placeholder); font-size: 12px; }
}
.trace-content { padding: 0 4px; }
.trace-summary {
  margin-bottom: 16px; padding: 12px; background: var(--td-bg-color-page); border-radius: 8px;
}
.trace-query { font-size: 14px; font-weight: 500; margin-bottom: 6px; }
.trace-plan-info { font-size: 13px; color: var(--td-text-color-secondary); display: flex; align-items: center; gap: 8px; }
.trace-label { color: var(--td-text-color-placeholder); font-size: 12px; }
.trace-steps { display: flex; flex-direction: column; gap: 10px; }
.trace-step {
  border: 1px solid var(--td-border-level-1-color); border-radius: 8px; padding: 10px 12px;
  &--done { border-left: 3px solid var(--td-success-color); }
  &--failed { border-left: 3px solid var(--td-error-color); }
  &--running { border-left: 3px solid var(--td-warning-color); }
}
.trace-step-header { display: flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; }
.trace-step-icon { display: flex; }
.trace-step-name { flex: 1; }
.trace-step-dur { font-size: 11px; color: var(--td-text-color-placeholder); }
.trace-step-desc { font-size: 12px; color: var(--td-text-color-secondary); margin: 4px 0 0 22px; }
.trace-tools { margin: 8px 0 0 22px; display: flex; flex-direction: column; gap: 6px; }
.trace-tool {
  display: flex; align-items: center; gap: 6px; font-size: 12px; flex-wrap: wrap;
  padding: 6px 8px; background: var(--td-bg-color-page); border-radius: 4px;
}
.trace-tool-name { display: flex; align-items: center; gap: 3px; font-weight: 500; }
.trace-tool-dur { font-size: 11px; color: var(--td-text-color-placeholder); }
.trace-tool-result { width: 100%; font-size: 11px; color: var(--td-text-color-secondary); margin-top: 4px; font-family: monospace; white-space: pre-wrap; word-break: break-all; }
.trace-mermaid { margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--td-border-level-1-color); }
.trace-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; padding: 40px 0; color: var(--td-text-color-placeholder); }

// ===== 定时任务 =====
.sched-task-card {
  border: 1px solid var(--td-border-level-1-color); border-radius: 8px; padding: 12px; margin-bottom: 10px;
}
.sched-task-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
.sched-task-name { font-weight: 500; font-size: 14px; }
.sched-task-prompt { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 6px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sched-task-meta { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 4px; }
.sched-task-last { font-size: 11px; color: var(--td-text-color-placeholder); margin-bottom: 4px; }
.sched-task-actions { display: flex; gap: 4px; }
.sched-form-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; font-weight: 500; font-size: 15px; }
.sched-form .form-row { margin-bottom: 12px; label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 4px; color: var(--td-text-color-secondary); } }
</style>

<style lang="less">
/* ===== Chat 全局样式覆写（Shadow DOM ::part 穿透）===== */

/* 气泡通用 */
t-chat-item::part(t-chat__item__content) {
  border-radius: 12px !important;
  font-size: 14px !important;
  line-height: 1.7 !important;
  border: none !important;
  box-shadow: none !important;
  padding: 10px 20px !important;
}

/* assistant / user 气泡：浅灰底 */
t-chat-item[role="assistant"]::part(t-chat__item__content),
t-chat-item[role="user"]::part(t-chat__item__content) {
  background: #f4f4f5 !important;
}

/* system 消息 */
t-chat-item[role="system"]::part(t-chat__item__content) {
  background: transparent !important;
  font-size: 12px !important;
  color: var(--td-text-color-placeholder) !important;
}
t-chat-item[role="system"]::part(t-chat__item__inner) {
  justify-content: center !important;
}

/* 消息间距 */
t-chat-item::part(t-chat__item__inner) {
  padding: 6px 16px !important;
}

/* header：名字 + 时间，一行显示 */
t-chat-item::part(t-chat__item__header) {
  display: flex !important;
  align-items: center !important;
  gap: 8px !important;
  margin-bottom: 4px !important;
}
t-chat-item::part(t-chat__item__name) {
  font-size: 13px !important;
  font-weight: 500 !important;
  color: var(--td-text-color-primary) !important;
}
t-chat-item::part(t-chat__item__time) {
  font-size: 12px !important;
  color: var(--td-text-color-placeholder) !important;
}

/* 去掉输入框多余边框 */
.chat-area .t-chat-sender {
  border: none !important;
  box-shadow: none !important;
}

/* 深色模式适配 */
[theme-mode="dark"] .chat-area,
[theme-mode="dark"] .chat-area .input-area {
  background: #1e1e1e !important;
}
[theme-mode="dark"] t-chat-item[role="assistant"]::part(t-chat__item__content),
[theme-mode="dark"] t-chat-item[role="user"]::part(t-chat__item__content) {
  background: #2c2c2e !important;
}
</style>
