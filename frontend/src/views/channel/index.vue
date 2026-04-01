<template>
  <div class="channel-page">
    <div class="page-header">
      <h3>{{ t('channel.title') }}</h3>
      <div class="header-actions">
        <t-input v-model="searchQuery" :placeholder="t('skill.searchPlaceholder')" clearable size="small" style="width: 200px">
          <template #prefix-icon><t-icon name="search" /></template>
        </t-input>
        <t-button variant="outline" size="small" @click="loadChannels">
          <template #icon><t-icon name="refresh" /></template>
        </t-button>
        <t-button theme="primary" size="small" @click="openAddDialog">
          <template #icon><t-icon name="add" /></template>
          {{ t('channel.add') }}
        </t-button>
      </div>
    </div>

    <div class="channel-list">
      <t-card v-for="ch in filteredChannels" :key="ch.id" class="channel-card">
        <div class="channel-header">
          <div class="channel-info">
            <div class="channel-title-row">
              <span class="channel-name">{{ ch.name }}</span>
              <t-tag :theme="ch.type === 'feishu' ? 'primary' : ch.type === 'dingtalk' ? 'warning' : 'success'" variant="light" size="small">
                {{ ch.type === 'feishu' ? t('channel.feishu') : ch.type === 'dingtalk' ? t('channel.dingtalk') : t('channel.wecom') }}
              </t-tag>
              <t-tag :theme="statusMap[ch.id] ? 'success' : 'default'" variant="light" size="small">
                {{ statusMap[ch.id] ? t('channel.connected') : t('channel.disconnected') }}
              </t-tag>
            </div>
            <div class="channel-bot" v-if="ch.botId">{{ t('channel.bindBot') }}: {{ ch.botId }}</div>
          </div>
          <div class="channel-actions">
            <t-button v-if="!statusMap[ch.id]" variant="outline" size="small" @click="handleConnect(ch.id)">
              {{ t('channel.connect') }}
            </t-button>
            <t-button v-else variant="outline" theme="warning" size="small" @click="handleDisconnect(ch.id)">
              {{ t('channel.disconnect') }}
            </t-button>
            <t-button variant="text" size="small" @click="openEditDialog(ch)">
              <t-icon name="edit" />
            </t-button>
            <t-button variant="text" theme="danger" size="small" @click="handleDelete(ch.id)">
              <t-icon name="delete" />
            </t-button>
          </div>
        </div>
      </t-card>

      <t-card v-if="filteredChannels.length === 0" class="channel-card">
        <t-empty />
      </t-card>
    </div>

    <!-- 添加/编辑对话框 -->
    <t-dialog v-model:visible="showDialog" :header="editingId ? t('channel.name') : t('channel.add')" :footer="false" width="520px">
      <t-form :data="form" label-width="120px">
        <t-form-item :label="t('channel.name')">
          <t-input v-model="form.name" />
        </t-form-item>
        <t-form-item :label="t('channel.type')">
          <t-radio-group v-model="form.type" :disabled="!!editingId">
            <t-radio value="feishu">{{ t('channel.feishu') }}</t-radio>
            <t-radio value="wecom">{{ t('channel.wecom') }}</t-radio>
            <t-radio value="dingtalk">{{ t('channel.dingtalk') }}</t-radio>
          </t-radio-group>
        </t-form-item>
        <!-- 飞书配置 -->
        <template v-if="form.type === 'feishu'">
          <t-form-item :label="t('channel.appId')">
            <t-input v-model="form.feishu.appId" />
          </t-form-item>
          <t-form-item :label="t('channel.appSecret')">
            <t-input v-model="form.feishu.appSecret" type="password" />
          </t-form-item>
        </template>

        <!-- 企业微信配置 -->
        <template v-if="form.type === 'wecom'">
          <t-form-item label="Bot ID">
            <t-input v-model="form.wecom.botId" />
          </t-form-item>
          <t-form-item label="Bot Secret">
            <t-input v-model="form.wecom.secret" type="password" />
          </t-form-item>
        </template>

        <!-- 钉钉配置 -->
        <template v-if="form.type === 'dingtalk'">
          <t-form-item label="Client ID (AppKey)">
            <t-input v-model="form.dingtalk.clientId" />
          </t-form-item>
          <t-form-item label="Client Secret (AppSecret)">
            <t-input v-model="form.dingtalk.clientSecret" type="password" />
          </t-form-item>
        </template>

        <t-form-item>
          <t-button theme="primary" @click="handleSave">{{ t('channel.save') }}</t-button>
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  GetChannels, SaveChannel, DeleteChannel,
  ConnectChannel, DisconnectChannel, GetChannelStatus,
  CreateBot,
} from '../../../wailsjs/go/agent/App'

const { t } = useI18n()

interface FeishuConfig { appId: string; appSecret: string }
interface WecomConfig { botId: string; secret: string }
interface DingtalkConfig { clientId: string; clientSecret: string }
interface ChannelData { id: string; type: string; name: string; enabled: boolean; botId: string; feishu?: FeishuConfig; wecom?: WecomConfig; dingtalk?: DingtalkConfig }

const channelList = ref<ChannelData[]>([])
const searchQuery = ref('')
const filteredChannels = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return channelList.value
  return channelList.value.filter(ch =>
    ch.name.toLowerCase().includes(q) || ch.type.toLowerCase().includes(q)
  )
})
const statusMap = ref<Record<string, boolean>>({})
const showDialog = ref(false)
const editingId = ref('')

const defaultForm = (): { name: string; type: string; feishu: FeishuConfig; wecom: WecomConfig; dingtalk: DingtalkConfig } => ({
  name: '', type: 'feishu',
  feishu: { appId: '', appSecret: '' },
  wecom: { botId: '', secret: '' },
  dingtalk: { clientId: '', clientSecret: '' },
})
const form = reactive(defaultForm())

const loadChannels = async () => {
  try {
    channelList.value = (await GetChannels()) || []
    // 查询各渠道连接状态
    const map: Record<string, boolean> = {}
    for (const ch of channelList.value) {
      map[ch.id] = await GetChannelStatus(ch.id)
    }
    statusMap.value = map
  } catch (e: any) {
    MessagePlugin.error(e.toString())
  }
}

const openAddDialog = () => {
  editingId.value = ''
  Object.assign(form, defaultForm())
  showDialog.value = true
}

const openEditDialog = (ch: ChannelData) => {
  editingId.value = ch.id
  form.name = ch.name
  form.type = ch.type
  if (ch.feishu) form.feishu = { ...ch.feishu }
  if (ch.wecom) form.wecom = { ...ch.wecom }
  if (ch.dingtalk) form.dingtalk = { ...ch.dingtalk }
  showDialog.value = true
}

const handleSave = async () => {
  const isNew = !editingId.value
  const id = editingId.value || `${form.type}_${Date.now()}`
  const typeNameMap: Record<string, string> = { feishu: t('channel.feishu'), wecom: t('channel.wecom'), dingtalk: t('channel.dingtalk') }
  const channelName = form.name || typeNameMap[form.type] || form.type

  let botId = ''

  if (isNew) {
    // 新增渠道时自动创建同名助手
    try {
      const avatarMap: Record<string, string> = { feishu: '🐦', wecom: '💬', dingtalk: '⚡' }
      const avatar = avatarMap[form.type] || '💬'
      const bot = await CreateBot({ name: channelName, avatar, description: `${channelName} channel bot`, systemPrompt: '', providerId: '', model: '' } as any)
      botId = (bot as any).id
    } catch (e: any) {
      MessagePlugin.error('创建助手失败: ' + e)
      return
    }
  } else {
    // 编辑时保留原有绑定
    const existing = channelList.value.find(c => c.id === id)
    botId = existing?.botId || ''
  }

  const data: ChannelData = {
    id,
    type: form.type,
    name: channelName,
    enabled: true,
    botId,
    feishu: form.type === 'feishu' ? { ...form.feishu } : undefined,
    wecom: form.type === 'wecom' ? { ...form.wecom } : undefined,
    dingtalk: form.type === 'dingtalk' ? { ...form.dingtalk } : undefined,
  }
  try {
    await SaveChannel(data as any)
    MessagePlugin.success(t('channel.saveSuccess'))
    showDialog.value = false
    await loadChannels()
  } catch (e: any) { MessagePlugin.error(e.toString()) }
}

const handleDelete = async (id: string) => {
  try {
    await DeleteChannel(id)
    MessagePlugin.success(t('channel.deleteSuccess'))
    await loadChannels()
  } catch (e: any) { MessagePlugin.error(e.toString()) }
}

const handleConnect = async (id: string) => {
  try {
    await ConnectChannel(id)
    MessagePlugin.success(t('channel.connectSuccess'))
    statusMap.value[id] = true
  } catch (e: any) { MessagePlugin.error(t('channel.connectFailed') + ': ' + e) }
}

const handleDisconnect = async (id: string) => {
  try {
    await DisconnectChannel(id)
    MessagePlugin.success(t('channel.disconnectSuccess'))
    statusMap.value[id] = false
  } catch (e: any) { MessagePlugin.error(e.toString()) }
}

onMounted(() => {
  loadChannels()
})
</script>

<style lang="less" scoped>
.channel-page { padding: 20px; height: 100%; overflow-y: auto; box-sizing: border-box; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; h3 { margin: 0; font-size: 18px; } }
.header-actions { display: flex; align-items: center; gap: 8px; }
.channel-list { display: flex; flex-direction: column; gap: 12px; }
.channel-card { :deep(.t-card__body) { padding: 16px; } }
.channel-header { display: flex; justify-content: space-between; align-items: center; }
.channel-info { flex: 1; }
.channel-title-row { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
.channel-name { font-weight: 600; font-size: 15px; }
.channel-bot { font-size: 12px; color: var(--td-text-color-placeholder); }
.channel-actions { display: flex; align-items: center; gap: 4px; }
</style>
