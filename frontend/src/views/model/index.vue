<template>
  <div class="model-page">
    <div class="page-header">
      <h3>{{ t('model.title') }}</h3>
    </div>

    <t-card :title="t('model.currentModel')" class="section-card">
      <div class="active-model">
        <t-select
          v-model="activeProviderID"
          :label="t('model.provider') + '：'"
          :placeholder="t('model.selectProvider')"
          @change="handleProviderChange"
          style="width: 200px"
        >
          <t-option v-for="p in providers" :key="p.id" :value="p.id" :label="p.name" />
        </t-select>
        <t-select
          v-model="activeModelName"
          :label="t('model.selectModel') + '：'"
          :placeholder="t('model.selectModel')"
          @change="handleModelChange"
          style="width: 240px; margin-left: 16px"
        >
          <t-option v-for="m in activeProviderModels" :key="m" :value="m" :label="m" />
        </t-select>
      </div>
    </t-card>

    <t-card :title="t('model.providerConfig')" class="section-card">
      <div v-for="(provider, idx) in providers" :key="provider.id" class="provider-card">
        <div class="provider-header">
          <span class="provider-name">{{ provider.name }}</span>
          <t-tag v-if="provider.apiKey" theme="success" variant="light" size="small">{{ t('model.configured') }}</t-tag>
          <t-tag v-else theme="warning" variant="light" size="small">{{ t('model.notConfigured') }}</t-tag>
        </div>
        <t-form label-width="80px" class="provider-form">
          <t-form-item :label="t('model.name')">
            <t-input v-model="provider.name" />
          </t-form-item>
          <t-form-item :label="t('model.apiUrl')">
            <t-input v-model="provider.baseUrl" :placeholder="t('model.apiUrl')" />
          </t-form-item>
          <t-form-item :label="t('model.apiKey')">
            <t-input v-model="provider.apiKey" type="password" />
          </t-form-item>
          <t-form-item :label="t('model.modelList')">
            <t-select
              v-model="provider.models"
              multiple
              filterable
              creatable
              :loading="fetchingModels[provider.id]"
              :placeholder="t('model.modelListPlaceholder')"
              @focus="handleSelectFocus(provider)"
            >
              <t-option
                v-for="m in mergedModels(provider)"
                :key="m"
                :value="m"
                :label="m"
              />
            </t-select>
          </t-form-item>
          <t-form-item>
            <t-space>
              <t-button theme="primary" @click="handleSave">{{ t('model.save') }}</t-button>
              <t-popconfirm :content="t('model.confirmDelete')" @confirm="handleDeleteProvider(idx)" v-if="providers.length > 1">
                <t-button theme="danger" variant="outline">{{ t('model.delete') }}</t-button>
              </t-popconfirm>
            </t-space>
          </t-form-item>
        </t-form>
      </div>
      <t-button variant="dashed" block @click="handleAddProvider" style="margin-top: 12px">
        <template #icon><t-icon name="add" /></template>
        {{ t('model.addProvider') }}
      </t-button>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { GetModelConfig, SaveModelProviders, SetActiveModel, FetchProviderModels } from '../../../wailsjs/go/agent/App'

const { t } = useI18n()

interface ModelProvider { id: string; name: string; baseUrl: string; apiKey: string; models: string[] }

const providers = ref<ModelProvider[]>([])
const activeProviderID = ref('')
const activeModelName = ref('')
const fetchingModels = ref<Record<string, boolean>>({})
const availableModels = ref<Record<string, string[]>>({})

const activeProviderModels = computed(() => {
  const p = providers.value.find((p) => p.id === activeProviderID.value)
  return p?.models || []
})

const loadConfig = async () => {
  try {
    const cfg = await GetModelConfig()
    providers.value = cfg.providers || []
    activeProviderID.value = cfg.activeModel?.providerId || ''
    activeModelName.value = cfg.activeModel?.model || ''
  } catch (e: any) { MessagePlugin.error(t('model.loadFailed') + ': ' + e) }
}

const handleProviderChange = async (val: string | number) => {
  const p = providers.value.find((p) => p.id === String(val))
  if (p && p.models.length > 0) activeModelName.value = p.models[0] ?? ''
  try { await SetActiveModel(activeProviderID.value, activeModelName.value); MessagePlugin.success(t('model.switchSuccess')) }
  catch (e: any) { MessagePlugin.error(t('model.switchFailed') + ': ' + e) }
}

const handleModelChange = async () => {
  try { await SetActiveModel(activeProviderID.value, activeModelName.value); MessagePlugin.success(t('model.switchSuccess')) }
  catch (e: any) { MessagePlugin.error(t('model.switchFailed') + ': ' + e) }
}

const handleSave = async () => {
  try { await SaveModelProviders(providers.value); MessagePlugin.success(t('model.saveSuccess')) }
  catch (e: any) { MessagePlugin.error(t('model.saveFailed') + ': ' + e) }
}

// 合并已选模型和远程拉取的模型作为下拉选项（去重）
const mergedModels = (provider: ModelProvider): string[] => {
  const remote = availableModels.value[provider.id] || []
  const set = new Set([...provider.models, ...remote])
  return [...set]
}

const handleSelectFocus = async (provider: ModelProvider) => {
  if (availableModels.value[provider.id] || !provider.apiKey) return
  fetchingModels.value[provider.id] = true
  try {
    const models = await FetchProviderModels(provider.id)
    availableModels.value[provider.id] = models || []
  } catch { /* 静默失败 */ }
  finally { fetchingModels.value[provider.id] = false }
}

const handleAddProvider = () => {
  providers.value.push({ id: 'custom-' + Date.now(), name: t('skill.custom'), baseUrl: '', apiKey: '', models: [] })
}

const handleDeleteProvider = async (idx: number) => {
  providers.value.splice(idx, 1)
  try { await SaveModelProviders(providers.value); MessagePlugin.success(t('model.deleteSuccess')) }
  catch (e: any) { MessagePlugin.error(e.toString()) }
}

onMounted(() => loadConfig())
</script>

<style lang="less" scoped>
.model-page { padding: 20px; overflow-y: auto; height: 100%; box-sizing: border-box; }
.page-header { margin-bottom: 16px; h3 { margin: 0; font-size: 18px; } }
.section-card { margin-bottom: 16px; }
.active-model { display: flex; align-items: center; }
.provider-card { padding: 16px; border: 1px solid var(--td-border-level-1-color); border-radius: 8px; margin-bottom: 12px; }
.provider-header { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; }
.provider-name { font-weight: 600; font-size: 15px; }
.provider-form { :deep(.t-form__item) { margin-bottom: 12px; } }
</style>
