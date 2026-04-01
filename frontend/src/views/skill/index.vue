<template>
  <div class="skill-page">
    <div class="page-header">
      <h3>{{ t('skill.title') }}</h3>
      <div class="header-actions">
        <t-input v-model="searchQuery" :placeholder="t('skill.searchPlaceholder')" clearable size="small" style="width: 200px">
          <template #prefix-icon><t-icon name="search" /></template>
        </t-input>
        <t-button variant="outline" size="small" @click="loadSkills(true)">
          <template #icon><t-icon name="refresh" /></template>
        </t-button>
        <t-button theme="primary" size="small" @click="showAddDialog = true">
          <template #icon><t-icon name="add" /></template>
          {{ t('skill.install') }}
        </t-button>
      </div>
    </div>

    <div class="skill-list">
      <t-card v-for="s in filteredSkills" :key="s.name" class="skill-card" @click="toggleDetail(s.name)">
        <div class="skill-header">
          <div class="skill-info">
            <div class="skill-title-row">
              <span class="skill-name">{{ s.displayName || s.name }}</span>
              <t-tag v-if="s.builtin" theme="primary" variant="light" size="small">{{ t('skill.builtin') }}</t-tag>
              <t-tag v-else theme="default" variant="light" size="small">{{ t('skill.custom') }}</t-tag>
              <t-tag v-if="s.type === 'mcp'" theme="warning" variant="light" size="small">MCP</t-tag>
              <t-tag v-else-if="!s.builtin" theme="success" variant="outline" size="small">Agent Skill</t-tag>
              <t-tag :theme="s.enabled ? 'success' : 'default'" variant="light" size="small">
                {{ s.enabled ? t('skill.enabled') : t('skill.disabled') }}
              </t-tag>
              <t-tooltip v-if="!s.builtin && s.securityLevel === 'caution'" :content="s.securityNote || t('skill.securityCaution')">
                <t-tag theme="warning" variant="light" size="small">{{ t('skill.securityCaution') }}</t-tag>
              </t-tooltip>
              <t-tag v-else-if="!s.builtin && s.securityLevel === 'safe'" theme="success" variant="light" size="small">{{ t('skill.securitySafe') }}</t-tag>
              <t-tag v-else-if="!s.builtin && !s.securityLevel" theme="default" variant="light" size="small">{{ t('skill.securityUnchecked') }}</t-tag>
            </div>
            <div class="skill-desc">{{ s.description }}</div>
            <div class="skill-meta">
              <span class="skill-version">v{{ s.version }}</span>
              <span v-if="s.type === 'mcp' && s.mcp" class="skill-transport">
                {{ s.mcp.transport === 'stdio' ? s.mcp.command : s.mcp.url }}
              </span>
            </div>
          </div>
          <div class="skill-actions" @click.stop>
            <t-switch :value="s.enabled" size="small" @change="(val: boolean) => handleToggle(s.name, val)" />
            <t-button v-if="!s.builtin" variant="text" theme="danger" size="small" @click="handleUninstall(s.name)">
              {{ t('skill.uninstall') }}
            </t-button>
          </div>
        </div>

        <div v-if="expandedSkill === s.name" class="skill-detail">
          <!-- SKILL.md 格式：渲染 markdown -->
          <div v-if="s.format === 'skillmd' && s.content" class="skillmd-content" v-html="renderMarkdown(s.content)" />
          <!-- skill.yaml 格式：显示工具列表 -->
          <div v-else class="tool-list">
            <div class="tool-list-title">{{ t('skill.toolCount', { count: s.tools?.length || 0 }) }}</div>
            <div v-for="tool in s.tools" :key="tool.name" class="tool-item">
              <div class="tool-name"><t-icon name="tools" size="14px" /> {{ tool.name }}</div>
              <div class="tool-desc">{{ tool.description }}</div>
              <div class="tool-params">
                <span class="param-label">{{ t('skill.param') }}：</span>
                <t-tag v-for="(prop, key) in tool.parameters?.properties" :key="key" size="small" variant="outline">
                  {{ key }}: {{ prop.type }}
                </t-tag>
              </div>
              <div v-if="tool.execute?.type === 'command'" class="tool-command">
                <span class="param-label">{{ t('skill.command') }}：</span>
                <code>{{ tool.execute.command }}</code>
              </div>
            </div>
          </div>
        </div>
      </t-card>

      <t-card v-if="filteredSkills.length === 0" class="skill-card">
        <t-empty />
      </t-card>
    </div>

    <t-drawer
      v-model:visible="showAddDialog"
      :header="t('skill.installTitle')"
      size="600px"
      :footer="addMode === 'mcp'"
      @close="showAddDialog = false"
    >
      <t-tabs v-model="addMode">
        <t-tab-panel value="agent" label="Agent Skill">
          <div class="install-form">
            <div class="field-row">
              <label>{{ t('skill.provider') }}</label>
              <t-select v-model="agentProvider" @change="onProviderChange">
                <t-option value="clawhub" label="ClawHub (Default)" />
                <t-option value="skillhub" label="SkillHub (Tencent)" />
              </t-select>
            </div>
            <div class="field-row">
              <label>{{ t('skill.searchSkill') }}</label>
              <t-select
                v-model="selectedSkillId"
                filterable
                :filter="() => true"
                :loading="searching"
                :options="searchOptions"
                :placeholder="t('skill.searchPlaceholder')"
                @search="handleSearch"
                @change="handleSelectSkill"
              />
            </div>
            <div v-if="selectedSkillId" class="selected-skill-actions">
              <t-button theme="primary" block :loading="installingAgent" @click="handleInstallAgent">
                {{ t('skill.installSkill', { name: selectedDisplayName }) }}
              </t-button>
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="mcp" label="MCP Server">
          <div class="install-form">
            <div class="mcp-hint">{{ t('skill.mcpHint') }}</div>
            <div class="field-row">
              <label>{{ t('skill.skillName') }}</label>
              <t-input v-model="mcpForm.name" :placeholder="t('skill.skillNamePlaceholder')" />
            </div>
            <div class="field-row">
              <label>{{ t('skill.displayName') }}</label>
              <t-input v-model="mcpForm.displayName" :placeholder="t('skill.displayNamePlaceholder')" />
            </div>
            <div class="field-row">
              <label>{{ t('skill.description') }}</label>
              <t-input v-model="mcpForm.description" :placeholder="t('skill.descPlaceholder')" />
            </div>
            <div class="field-row">
              <label>{{ t('skill.mcpTransport') }}</label>
              <t-radio-group v-model="mcpForm.transport" variant="default-filled">
                <t-radio-button value="stdio">Stdio</t-radio-button>
                <t-radio-button value="sse">SSE</t-radio-button>
              </t-radio-group>
            </div>

            <template v-if="mcpForm.transport === 'stdio'">
              <div class="field-row">
                <label>{{ t('skill.mcpCommand') }}</label>
                <t-input v-model="mcpForm.command" :placeholder="t('skill.mcpCommandPlaceholder')" />
              </div>
              <div class="field-row">
                <label>{{ t('skill.mcpArgs') }}</label>
                <t-input v-model="mcpForm.argsStr" :placeholder="t('skill.mcpArgsPlaceholder')" />
              </div>
            </template>

            <template v-if="mcpForm.transport === 'sse'">
              <div class="field-row">
                <label>{{ t('skill.mcpURL') }}</label>
                <t-input v-model="mcpForm.url" :placeholder="t('skill.mcpURLPlaceholder')" />
              </div>
            </template>

            <div class="field-row">
              <label>{{ t('skill.mcpEnv') }}</label>
              <div class="env-editor">
                <div v-for="(env, idx) in mcpForm.envList" :key="idx" class="env-row">
                  <t-input v-model="env.key" placeholder="KEY" style="width: 150px" />
                  <span class="env-eq">=</span>
                  <t-input v-model="env.value" placeholder="VALUE" style="flex: 1" type="password" />
                  <t-button variant="text" theme="danger" size="small" @click="mcpForm.envList.splice(idx, 1)">
                    <t-icon name="delete" />
                  </t-button>
                </div>
                <t-button variant="dashed" size="small" block @click="mcpForm.envList.push({ key: '', value: '' })">
                  {{ t('skill.mcpAddEnv') }}
                </t-button>
              </div>
            </div>
          </div>
        </t-tab-panel>
      </t-tabs>
      <template #footer>
        <t-space>
          <t-button @click="showAddDialog = false">{{ t('common.cancel') }}</t-button>
          <t-button theme="primary" :loading="installingMcp" @click="handleInstallMcp">{{ t('skill.install') }}</t-button>
        </t-space>
      </template>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import * as Runtime from '../../../wailsjs/runtime/runtime'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import {
  GetSkills, RefreshSkills, InstallMCPSkill,
  SearchSkillHubSkills, InstallSkillHubSkill,
  SearchClawHubSkills, InstallClawHubSkill,
  UninstallSkill, SetSkillEnabled,
} from '../../../wailsjs/go/agent/App'

const { t } = useI18n()

interface PropDef { type: string; description: string }
interface ToolExecute { type: string; command: string }
interface MCPConfig { transport: string; command: string; args: string[]; url: string; env: Record<string, string> }
interface ToolDef { name: string; description: string; parameters: { type: string; properties: Record<string, PropDef>; required: string[] }; execute: ToolExecute }
interface SkillData { name: string; displayName: string; description: string; version: string; enabled: boolean; builtin: boolean; type?: string; format?: string; content?: string; mcp?: MCPConfig; tools: ToolDef[]; securityLevel?: string; securityNote?: string }
interface MCPForm { name: string; displayName: string; description: string; transport: string; command: string; argsStr: string; url: string; envList: { key: string; value: string }[] }
interface SkillHubResult { name: string; slug: string; description: string; description_zh: string; version: string; score: number; downloads: number }
interface ClawHubResult { name: string; href: string; desc: string }

const skills = ref<SkillData[]>([])
const searchQuery = ref('')
const filteredSkills = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return skills.value
  return skills.value.filter(s =>
    (s.displayName || s.name).toLowerCase().includes(q) ||
    (s.description || '').toLowerCase().includes(q)
  )
})
const expandedSkill = ref('')
const showAddDialog = ref(false)
const addMode = ref('agent')

// Agent Skill search
const agentProvider = ref('clawhub')
const selectedSkillId = ref('')
const selectedDisplayName = ref('')
const searching = ref(false)
const skillHubResults = ref<SkillHubResult[]>([])
const clawHubResults = ref<ClawHubResult[]>([])
const installingAgent = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

// MCP form
const installingMcp = ref(false)
const mcpForm = ref<MCPForm>({ name: '', displayName: '', description: '', transport: 'stdio', command: '', argsStr: '', url: '', envList: [] })

const loadSkills = async (refresh = false) => {
  try { skills.value = refresh ? await RefreshSkills() : await GetSkills() }
  catch (e: any) { MessagePlugin.error(t('skill.toggleFailed') + ': ' + e) }
}

const handleToggle = async (name: string, enabled: boolean) => {
  try { await SetSkillEnabled(name, enabled); await loadSkills() }
  catch (e: any) { MessagePlugin.error(t('skill.toggleFailed') + ': ' + e) }
}

const handleUninstall = async (name: string) => {
  try { await UninstallSkill(name); MessagePlugin.success(t('skill.uninstallSuccess')); await loadSkills() }
  catch (e: any) { MessagePlugin.error(t('skill.uninstallFailed') + ': ' + e) }
}

const toggleDetail = (name: string) => { expandedSkill.value = expandedSkill.value === name ? '' : name }
const renderMarkdown = (md: string) => marked(md) as string

const searchOptions = computed(() => {
  if (agentProvider.value === 'skillhub') {
    return skillHubResults.value.map(item => ({
      value: item.name,
      label: `${item.name}  —  ${item.description_zh || item.description}`,
    }))
  }
  return clawHubResults.value.map(item => ({
    value: item.href,
    label: `${item.name}  —  ${item.desc}`,
  }))
})

const onProviderChange = () => {
  selectedSkillId.value = ''
  selectedDisplayName.value = ''
  skillHubResults.value = []
  clawHubResults.value = []
}

// === 搜索 ===
const handleSearch = (query: string) => {
  if (searchTimer) clearTimeout(searchTimer)
  if (!query || query.length < 2) {
    skillHubResults.value = []
    clawHubResults.value = []
    return
  }
  searchTimer = setTimeout(async () => {
    searching.value = true
    try {
      if (agentProvider.value === 'skillhub') {
        skillHubResults.value = (await SearchSkillHubSkills(query)) || []
      } else {
        clawHubResults.value = (await SearchClawHubSkills(query)) || []
      }
    } catch {
      skillHubResults.value = []
      clawHubResults.value = []
    } finally {
      searching.value = false
    }
  }, 400)
}

const handleSelectSkill = (val: string) => {
  selectedSkillId.value = val
  // 记录显示名称
  if (agentProvider.value === 'skillhub') {
    const item = skillHubResults.value.find(i => i.name === val)
    selectedDisplayName.value = item?.name || val
  } else {
    const item = clawHubResults.value.find(i => i.href === val)
    selectedDisplayName.value = item?.name || val
  }
}

// === Agent Skill 安装 ===
const handleInstallAgent = async () => {
  if (!selectedSkillId.value) return
  installingAgent.value = true
  try {
    if (agentProvider.value === 'skillhub') {
      await InstallSkillHubSkill(selectedSkillId.value)
    } else {
      await InstallClawHubSkill(selectedSkillId.value)
    }
    MessagePlugin.success(t('skill.installSuccess'))
    showAddDialog.value = false
    selectedSkillId.value = ''
    selectedDisplayName.value = ''
    skillHubResults.value = []
    clawHubResults.value = []
    await loadSkills()
  } catch (e: any) {
    MessagePlugin.error(t('skill.installFailed') + ': ' + e)
  } finally {
    installingAgent.value = false
  }
}

// === MCP 安装 ===
const handleInstallMcp = async () => {
  installingMcp.value = true
  try {
    const f = mcpForm.value
    if (!f.name) { MessagePlugin.warning(t('skill.nameRequired')); return }
    if (f.transport === 'stdio' && !f.command) { MessagePlugin.warning(t('skill.mcpCommandRequired')); return }
    if (f.transport === 'sse' && !f.url) { MessagePlugin.warning(t('skill.mcpURLRequired')); return }

    const env: Record<string, string> = {}
    for (const e of f.envList) { if (e.key) env[e.key] = e.value }
    const args = f.argsStr ? f.argsStr.split(/\s+/).filter(Boolean) : []

    await InstallMCPSkill({
      name: f.name, displayName: f.displayName, description: f.description,
      version: '1.0.0', enabled: true, builtin: false, type: 'mcp',
      mcp: { transport: f.transport, command: f.command, args, url: f.url, env },
      tools: [],
    } as any)
    MessagePlugin.success(t('skill.installSuccess'))
    showAddDialog.value = false
    mcpForm.value = { name: '', displayName: '', description: '', transport: 'stdio', command: '', argsStr: '', url: '', envList: [] }
    await loadSkills()
  } catch (e: any) { MessagePlugin.error(t('skill.installFailed') + ': ' + e) }
  finally { installingMcp.value = false }
}

onMounted(() => {
  loadSkills()
  Runtime.EventsOn('skill:installed', () => loadSkills())
})
onUnmounted(() => {
  Runtime.EventsOff('skill:installed')
})
</script>

<style lang="less" scoped>
.skill-page { padding: 20px; height: 100%; overflow-y: auto; box-sizing: border-box; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; h3 { margin: 0; font-size: 18px; } }
.header-actions { display: flex; align-items: center; gap: 8px; }
.skill-list { display: flex; flex-direction: column; gap: 12px; }
.skill-card { :deep(.t-card__body) { padding: 16px; } cursor: pointer; }
.skill-header { display: flex; justify-content: space-between; align-items: flex-start; }
.skill-info { flex: 1; }
.skill-title-row { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
.skill-name { font-weight: 600; font-size: 15px; }
.skill-desc { color: var(--td-text-color-secondary); font-size: 13px; margin-bottom: 2px; }
.skill-meta { display: flex; align-items: center; gap: 12px; }
.skill-version { color: var(--td-text-color-placeholder); font-size: 12px; }
.skill-transport { color: var(--td-text-color-placeholder); font-size: 12px; font-family: monospace; }
.skill-actions { display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
.skill-detail { margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--td-border-level-1-color); }
.skillmd-content {
  font-size: 13px; line-height: 1.7; color: var(--td-text-color-primary);
  :deep(h1) { font-size: 18px; margin: 0 0 8px; }
  :deep(h2) { font-size: 15px; margin: 12px 0 6px; color: var(--td-text-color-secondary); }
  :deep(h3) { font-size: 14px; margin: 10px 0 4px; }
  :deep(p) { margin: 4px 0; }
  :deep(ul), :deep(ol) { padding-left: 20px; margin: 4px 0; }
  :deep(code) { background: var(--td-bg-color-container); padding: 1px 5px; border-radius: 3px; font-family: monospace; font-size: 12px; }
  :deep(pre) { background: var(--td-bg-color-container); padding: 10px 12px; border-radius: 6px; overflow-x: auto; margin: 6px 0; }
  :deep(pre code) { background: none; padding: 0; font-size: 12px; }
  :deep(a) { color: var(--td-brand-color); text-decoration: none; }
}
.tool-list { margin-top: 0; }
.tool-list-title { font-size: 13px; color: var(--td-text-color-secondary); margin-bottom: 8px; }
.tool-item { padding: 10px 12px; background: var(--td-bg-color-page); border-radius: 6px; margin-bottom: 8px; }
.tool-name { display: flex; align-items: center; gap: 4px; font-weight: 500; font-size: 13px; margin-bottom: 4px; }
.tool-desc { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 4px; }
.tool-params { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; font-size: 12px; }
.tool-command { margin-top: 4px; font-size: 12px; code { background: var(--td-bg-color-container); padding: 2px 6px; border-radius: 3px; font-family: monospace; } }
.param-label { color: var(--td-text-color-placeholder); }
.install-form { margin-top: 8px; }
.field-row { margin-bottom: 12px; label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 4px; color: var(--td-text-color-secondary); } }
.selected-skill-actions { margin-top: 16px; }
.mcp-hint { color: var(--td-text-color-secondary); font-size: 13px; margin-bottom: 12px; padding: 8px 12px; background: var(--td-bg-color-page); border-radius: 6px; }
.env-editor { width: 100%; }
.env-row { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.env-eq { color: var(--td-text-color-placeholder); font-family: monospace; }
</style>

<style lang="less">
/* 全局样式：修复技能搜索下拉选项布局 */
.t-select__list .t-select-option {
  height: auto !important;
  min-height: 36px;
}
</style>
