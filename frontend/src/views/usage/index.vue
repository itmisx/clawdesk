<template>
  <div class="usage-page">
    <div class="page-header">
      <h3>{{ t('dashboard.title') }}</h3>
      <t-button variant="outline" size="small" @click="loadStats">
        <template #icon><t-icon name="refresh" /></template>
      </t-button>
    </div>

    <!-- 总览卡片 -->
    <div class="stat-cards">
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(0, 82, 217, 0.1); color: #0052d9">
          <t-icon name="cpu" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.modelCount }}</div>
          <div class="stat-label">{{ t('dashboard.modelCount') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(0, 168, 112, 0.1); color: #00a870">
          <t-icon name="extension" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.skillCount }}</div>
          <div class="stat-label">{{ t('dashboard.skillCount') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(237, 123, 47, 0.1); color: #ed7b2f">
          <t-icon name="chat-message" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.sessionCount }}</div>
          <div class="stat-label">{{ t('dashboard.sessionCount') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(179, 77, 211, 0.1); color: #b34dd3">
          <t-icon name="arrow-right-up" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.totalRequests }}</div>
          <div class="stat-label">{{ t('dashboard.totalRequests') }}</div>
        </div>
      </div>
    </div>

    <!-- Token 用量 -->
    <t-card :title="t('dashboard.tokenUsage')" class="section-card">
      <div v-if="stats.totalTokens > 0" class="token-overview">
        <div class="token-bar-wrapper">
          <div class="token-bar">
            <div
              class="token-bar-prompt"
              :style="{ width: promptPercent + '%' }"
            ></div>
            <div
              class="token-bar-completion"
              :style="{ width: completionPercent + '%' }"
            ></div>
          </div>
          <div class="token-legend">
            <span class="legend-item">
              <span class="legend-dot prompt"></span>
              {{ t('dashboard.promptTokens') }}: {{ formatNumber(stats.totalPromptTokens) }}
            </span>
            <span class="legend-item">
              <span class="legend-dot completion"></span>
              {{ t('dashboard.completionTokens') }}: {{ formatNumber(stats.totalCompletionTokens) }}
            </span>
            <span class="legend-item total">
              {{ t('dashboard.totalTokens') }}: {{ formatNumber(stats.totalTokens) }}
            </span>
          </div>
        </div>
      </div>
      <t-empty v-else :description="t('dashboard.noData')" />
    </t-card>

    <!-- 按厂商统计 -->
    <t-card :title="t('dashboard.byProvider')" class="section-card">
      <div v-if="providerList.length > 0">
        <div v-for="prov in providerList" :key="prov.providerId" class="provider-section">
          <div class="provider-header">
            <span class="provider-name">{{ prov.providerName }}</span>
            <t-tag theme="primary" variant="light" size="small">
              {{ formatNumber(prov.totalTokens) }} {{ t('dashboard.totalTokens') }}
            </t-tag>
            <t-tag variant="light" size="small">
              {{ prov.requests }} {{ t('dashboard.requests') }}
            </t-tag>
          </div>
          <t-table
            :data="modelList(prov)"
            :columns="modelColumns"
            size="small"
            row-key="model"
            :bordered="false"
            :hover="true"
          />
        </div>
      </div>
      <t-empty v-else :description="t('dashboard.noData')" />
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { GetUsageStats } from '../../../wailsjs/go/agent/App'

const { t } = useI18n()

const stats = ref<any>({
  totalPromptTokens: 0,
  totalCompletionTokens: 0,
  totalTokens: 0,
  totalRequests: 0,
  modelCount: 0,
  skillCount: 0,
  sessionCount: 0,
  byProvider: {},
})

const promptPercent = computed(() => {
  if (stats.value.totalTokens === 0) return 0
  return (stats.value.totalPromptTokens / stats.value.totalTokens) * 100
})

const completionPercent = computed(() => {
  if (stats.value.totalTokens === 0) return 0
  return (stats.value.totalCompletionTokens / stats.value.totalTokens) * 100
})

const providerList = computed(() => {
  const bp = stats.value.byProvider || {}
  return Object.values(bp) as any[]
})

const modelList = (prov: any) => {
  const bm = prov.byModel || {}
  return Object.values(bm) as any[]
}

const modelColumns = computed(() => [
  { colKey: 'model', title: t('dashboard.model'), width: 200 },
  { colKey: 'promptTokens', title: t('dashboard.promptTokens'), cell: (_: any, { row }: any) => formatNumber(row.promptTokens) },
  { colKey: 'completionTokens', title: t('dashboard.completionTokens'), cell: (_: any, { row }: any) => formatNumber(row.completionTokens) },
  { colKey: 'totalTokens', title: t('dashboard.totalTokens'), cell: (_: any, { row }: any) => formatNumber(row.totalTokens) },
  { colKey: 'requests', title: t('dashboard.requests') },
])

const formatNumber = (n: number) => {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return String(n)
}

const loadStats = async () => {
  try {
    stats.value = await GetUsageStats()
  } catch (e) {
    console.error('Failed to load stats:', e)
  }
}

onMounted(() => loadStats())
</script>

<style lang="less" scoped>
.usage-page {
  padding: 20px;
  height: 100%;
  overflow-y: auto;
  box-sizing: border-box;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  h3 { margin: 0; font-size: 18px; }
}

.stat-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 16px;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  border-radius: var(--td-radius-large);
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-border-level-1-color);
}

.stat-icon {
  width: 40px; height: 40px;
  border-radius: var(--td-radius-medium);
  display: flex; align-items: center; justify-content: center;
  flex-shrink: 0;
}

.stat-value {
  font-size: 22px; font-weight: 700;
  color: var(--td-text-color-primary);
}

.stat-label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 2px;
}

.section-card { margin-bottom: 16px; }

.token-overview { padding: 8px 0; }

.token-bar-wrapper { max-width: 600px; }

.token-bar {
  height: 20px;
  border-radius: 10px;
  background: var(--td-bg-color-page);
  display: flex;
  overflow: hidden;
  margin-bottom: 10px;
}

.token-bar-prompt {
  background: var(--td-brand-color);
  transition: width 0.5s ease;
  border-radius: 10px 0 0 10px;
}

.token-bar-completion {
  background: var(--td-success-color);
  transition: width 0.5s ease;
  border-radius: 0 10px 10px 0;
}

.token-legend {
  display: flex; gap: 20px; align-items: center; flex-wrap: wrap;
}

.legend-item {
  display: flex; align-items: center; gap: 6px;
  font-size: 13px; color: var(--td-text-color-secondary);
  &.total { font-weight: 600; color: var(--td-text-color-primary); }
}

.legend-dot {
  width: 10px; height: 10px; border-radius: 50%;
  &.prompt { background: var(--td-brand-color); }
  &.completion { background: var(--td-success-color); }
}

.provider-section {
  margin-bottom: 16px;
  &:last-child { margin-bottom: 0; }
}

.provider-header {
  display: flex; align-items: center; gap: 8px;
  margin-bottom: 8px;
}

.provider-name { font-weight: 600; font-size: 14px; }
</style>
