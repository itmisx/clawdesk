<template>
  <div class="request-log-page">
    <div class="page-header">
      <h3>{{ t('requestLog.title') }}</h3>
      <div class="header-actions">
        <t-select v-model="selectedSession" size="small" style="width: 200px" @change="loadData" :placeholder="t('requestLog.selectSession')" filterable>
          <t-option v-for="s in sessions" :key="s.id" :value="s.id" :label="(s.avatar || '🤖') + ' ' + s.name" />
        </t-select>
        <t-button variant="outline" size="small" @click="loadData">
          <template #icon><t-icon name="refresh" /></template>
        </t-button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="stat-cards">
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(0, 82, 217, 0.1); color: #0052d9">
          <t-icon name="file-paste" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ records.length }}</div>
          <div class="stat-label">{{ t('requestLog.totalRecords') }}</div>
        </div>
      </div>
      <div class="stat-card" v-if="records.length > 0">
        <div class="stat-icon" style="background: rgba(0, 168, 112, 0.1); color: #00a870">
          <t-icon name="time" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ formatTime(records[records.length - 1]?.ts) }}</div>
          <div class="stat-label">{{ t('requestLog.latestTime') }}</div>
        </div>
      </div>
    </div>

    <!-- 记录列表 -->
    <t-card :title="t('requestLog.recentRecords')" class="section-card">
      <div v-if="records.length > 0" class="log-list">
        <div v-for="(record, idx) in [...records].reverse()" :key="idx" class="log-item">
          <div class="log-header" @click="expandedIdx = expandedIdx === idx ? -1 : idx">
            <t-tag variant="light" size="small">#{{ records.length - idx }}</t-tag>
            <span class="log-time">{{ new Date(record.ts).toLocaleString() }}</span>
            <span class="log-tools-count" v-if="record.tools?.length">{{ record.tools.length }} tools</span>
            <t-icon :name="expandedIdx === idx ? 'chevron-up' : 'chevron-down'" size="16px" style="margin-left: auto" />
          </div>
          <div v-if="expandedIdx === idx" class="log-detail">
            <!-- 系统提示词 -->
            <div class="detail-section">
              <div class="section-title">
                <t-icon name="root-list" size="14px" />
                <span>{{ t('requestLog.systemPrompt') }}</span>
                <t-button variant="text" size="small" @click="copyText(record.systemPrompt, 'sys-' + idx)" style="margin-left: auto">
                  <template #icon><t-icon name="file-copy" /></template>
                  {{ copiedKey === 'sys-' + idx ? t('requestLog.copied') : t('requestLog.copy') }}
                </t-button>
              </div>
              <pre class="log-pre">{{ record.systemPrompt || '-' }}</pre>
            </div>

            <!-- Function Calling -->
            <div class="detail-section">
              <div class="section-title">
                <t-icon name="call" size="14px" />
                <span>{{ t('requestLog.functionCalling') }}</span>
                <t-tag size="small" variant="light" style="margin-left: 4px">{{ record.tools?.length || 0 }}</t-tag>
                <t-button variant="text" size="small" @click="copyTools(record.tools, idx)" style="margin-left: auto">
                  <template #icon><t-icon name="file-copy" /></template>
                  {{ copiedKey === 'tools-' + idx ? t('requestLog.copied') : t('requestLog.copy') }}
                </t-button>
              </div>
              <div v-if="record.tools?.length" class="tools-list">
                <div v-for="(tool, ti) in record.tools" :key="Number(ti)" class="tool-card">
                  <div class="tool-header" @click.stop="toggleTool(idx, Number(ti))">
                    <span class="tool-name">{{ getToolName(tool) }}</span>
                    <span class="tool-desc">{{ getToolDesc(tool) }}</span>
                    <t-icon :name="isToolExpanded(idx, Number(ti)) ? 'chevron-up' : 'chevron-down'" size="14px" style="margin-left: auto; flex-shrink: 0" />
                  </div>
                  <div v-if="isToolExpanded(idx, Number(ti))" class="tool-params">
                    <div v-for="(param, pname) in getToolParams(tool)" :key="String(pname)" class="param-row">
                      <span class="param-name" :class="{ required: isRequired(tool, String(pname)) }">{{ pname }}</span>
                      <t-tag size="small" variant="outline">{{ param.type }}</t-tag>
                      <span class="param-desc">{{ param.description }}</span>
                    </div>
                    <div v-if="!Object.keys(getToolParams(tool)).length" class="param-empty">{{ t('requestLog.noParams') }}</div>
                  </div>
                </div>
              </div>
              <div v-else class="empty-tools">{{ t('requestLog.noTools') }}</div>
            </div>
          </div>
        </div>
      </div>
      <t-empty v-else :description="t('requestLog.noData')" />
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { GetSessions, GetRequestLogs } from '../../../wailsjs/go/agent/App'

const { t } = useI18n()

const sessions = ref<any[]>([])
const selectedSession = ref('')
const records = ref<any[]>([])
const expandedIdx = ref(-1)
const copiedKey = ref('')
const expandedTools = ref<Set<string>>(new Set())

const getToolFn = (tool: any) => tool?.function || {}
const getToolName = (tool: any) => getToolFn(tool).name || ''
const getToolDesc = (tool: any) => getToolFn(tool).description || ''
const getToolParams = (tool: any) => getToolFn(tool).parameters?.properties || {}
const isRequired = (tool: any, name: string) => (getToolFn(tool).parameters?.required || []).includes(name)

const toggleTool = (logIdx: number, toolIdx: number) => {
  const key = `${logIdx}-${toolIdx}`
  if (expandedTools.value.has(key)) {
    expandedTools.value.delete(key)
  } else {
    expandedTools.value.add(key)
  }
  expandedTools.value = new Set(expandedTools.value)
}
const isToolExpanded = (logIdx: number, toolIdx: number) => expandedTools.value.has(`${logIdx}-${toolIdx}`)

const copyText = async (text: string, key: string) => {
  try {
    await navigator.clipboard.writeText(text || '')
    copiedKey.value = key
    setTimeout(() => { copiedKey.value = '' }, 2000)
  } catch (e) {
    console.error(e)
  }
}

const copyTools = async (tools: any[], idx: number) => {
  await copyText(JSON.stringify(tools || [], null, 2), 'tools-' + idx)
}

const formatTime = (ts: string) => {
  if (!ts) return '-'
  return new Date(ts).toLocaleString()
}

const loadData = async () => {
  if (!selectedSession.value) {
    records.value = []
    return
  }
  expandedIdx.value = -1
  expandedTools.value = new Set()
  try {
    const result = await GetRequestLogs(selectedSession.value)
    records.value = result || []
  } catch (e) {
    console.error(e)
    records.value = []
  }
}

onMounted(async () => {
  try {
    sessions.value = await GetSessions()
  } catch (e) {
    console.error(e)
  }
})
</script>

<style lang="less" scoped>
.request-log-page { padding: 20px; height: 100%; overflow-y: auto; box-sizing: border-box; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; h3 { margin: 0; font-size: 18px; } }
.header-actions { display: flex; align-items: center; gap: 8px; }
.stat-cards { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; margin-bottom: 16px; }
.stat-card { display: flex; align-items: center; gap: 12px; padding: 16px; border-radius: var(--td-radius-large); background: var(--td-bg-color-container); border: 1px solid var(--td-border-level-1-color); }
.stat-icon { width: 40px; height: 40px; border-radius: var(--td-radius-medium); display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.stat-value { font-size: 22px; font-weight: 700; }
.stat-label { font-size: 12px; color: var(--td-text-color-secondary); margin-top: 2px; }
.section-card { margin-bottom: 16px; }
.log-list { display: flex; flex-direction: column; gap: 8px; }
.log-item {
  border: 1px solid var(--td-border-level-1-color);
  border-radius: var(--td-radius-medium);
  overflow: hidden;
  transition: border-color 0.2s;
  &:hover { border-color: var(--td-brand-color); }
}
.log-header {
  display: flex; align-items: center; gap: 8px; padding: 10px 12px;
  background: var(--td-bg-color-container); cursor: pointer;
}
.log-time { font-size: 13px; color: var(--td-text-color-secondary); }
.log-tools-count { font-size: 12px; color: var(--td-text-color-placeholder); }
.log-detail {
  border-top: 1px solid var(--td-border-level-1-color);
  background: var(--td-bg-color-page);
}
.detail-section {
  padding: 12px;
  &:not(:last-child) { border-bottom: 1px solid var(--td-border-level-1-color); }
}
.section-title {
  display: flex; align-items: center; gap: 6px;
  font-size: 13px; font-weight: 600; margin-bottom: 8px;
  color: var(--td-text-color-primary);
}
.log-pre {
  margin: 0; padding: 10px;
  white-space: pre-wrap; word-break: break-all;
  font-size: 12px; line-height: 1.6;
  max-height: 400px; overflow-y: auto;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
  border-radius: var(--td-radius-medium);
}
.tools-list { display: flex; flex-direction: column; gap: 6px; }
.tool-card {
  border: 1px solid var(--td-border-level-1-color);
  border-radius: var(--td-radius-medium);
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.tool-header {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 10px; cursor: pointer;
  font-size: 12px;
  &:hover { background: var(--td-bg-color-container-hover); }
}
.tool-name { font-weight: 600; color: var(--td-brand-color); white-space: nowrap; flex-shrink: 0; }
.tool-desc { color: var(--td-text-color-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tool-params {
  border-top: 1px solid var(--td-border-level-1-color);
  padding: 8px 10px;
  display: flex; flex-direction: column; gap: 4px;
  background: var(--td-bg-color-page);
}
.param-row {
  display: flex; align-items: center; gap: 6px;
  font-size: 12px; padding: 3px 0;
}
.param-name {
  font-weight: 500; color: var(--td-text-color-primary); white-space: nowrap;
  &.required::after { content: ' *'; color: var(--td-error-color); }
}
.param-desc { color: var(--td-text-color-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.param-empty { font-size: 12px; color: var(--td-text-color-placeholder); }
.empty-tools { font-size: 12px; color: var(--td-text-color-placeholder); padding: 8px 0; }
</style>
