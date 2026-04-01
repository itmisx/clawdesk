<template>
  <div class="audit-page">
    <div class="page-header">
      <h3>{{ t('audit.title') }}</h3>
      <div class="header-actions">
        <t-select v-model="queryDays" size="small" style="width: 120px" @change="loadData">
          <t-option :value="7" label="最近 7 天" />
          <t-option :value="14" label="最近 14 天" />
          <t-option :value="30" label="最近 30 天" />
        </t-select>
        <t-input v-model="queryTool" size="small" :placeholder="t('audit.filterTool')" clearable style="width: 150px" @change="loadRecords" />
        <t-button variant="outline" size="small" @click="loadData">
          <template #icon><t-icon name="refresh" /></template>
        </t-button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="stat-cards">
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(0, 82, 217, 0.1); color: #0052d9">
          <t-icon name="call" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.totalCalls }}</div>
          <div class="stat-label">{{ t('audit.totalCalls') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(0, 168, 112, 0.1); color: #00a870">
          <t-icon name="check-circle" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.successCalls }}</div>
          <div class="stat-label">{{ t('audit.successCalls') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(227, 77, 89, 0.1); color: #e34d59">
          <t-icon name="close-circle" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.failedCalls }}</div>
          <div class="stat-label">{{ t('audit.failedCalls') }}</div>
        </div>
      </div>
    </div>

    <!-- 按工具 / 按助手 统计 -->
    <div class="stat-row" v-if="stats.totalCalls > 0">
      <t-card :title="t('audit.byTool')" class="stat-half">
        <div v-for="(count, tool) in stats.byTool" :key="tool" class="bar-item">
          <span class="bar-label">{{ tool }}</span>
          <div class="bar-track">
            <div class="bar-fill" :style="{ width: (count / stats.totalCalls * 100) + '%' }"></div>
          </div>
          <span class="bar-count">{{ count }}</span>
        </div>
      </t-card>
      <t-card :title="t('audit.byBot')" class="stat-half">
        <div v-for="(count, bot) in stats.byBot" :key="bot" class="bar-item">
          <span class="bar-label">{{ bot }}</span>
          <div class="bar-track">
            <div class="bar-fill success" :style="{ width: (count / stats.totalCalls * 100) + '%' }"></div>
          </div>
          <span class="bar-count">{{ count }}</span>
        </div>
        <t-empty v-if="Object.keys(stats.byBot || {}).length === 0" size="small" />
      </t-card>
    </div>

    <!-- 操作记录表格 -->
    <t-card :title="t('audit.recentRecords')" class="records-card">
      <t-table
        v-if="pageResult.records.length > 0"
        :data="pageResult.records"
        :columns="columns"
        row-key="timestamp"
        size="small"
        :hover="true"
        :stripe="true"
        :max-height="400"
      />
      <t-empty v-else :description="t('audit.noData')" />
      <div class="pagination" v-if="pageResult.total > pageResult.pageSize">
        <t-pagination
          v-model:current="currentPage"
          :total="pageResult.total"
          :pageSize="pageResult.pageSize"
          size="small"
          :showPageSize="false"
          @current-change="loadRecords"
        />
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { Tag } from 'tdesign-vue-next'
import { GetAuditRecords, GetAuditStats } from '../../../wailsjs/go/agent/App'

const { t } = useI18n()

const queryDays = ref(7)
const queryTool = ref('')
const currentPage = ref(1)
const pageResult = ref<any>({ records: [], total: 0, page: 1, pageSize: 20 })
const stats = ref<any>({ totalCalls: 0, successCalls: 0, failedCalls: 0, byTool: {}, byBot: {} })

const columns = computed(() => [
  {
    colKey: 'timestamp', title: t('audit.time'), width: 160,
    cell: (_: any, { row }: any) => new Date(row.timestamp).toLocaleString()
  },
  { colKey: 'botName', title: t('audit.bot'), width: 100, ellipsis: true },
  { colKey: 'skillName', title: t('audit.skill'), width: 130,
    cell: (_: any, { row }: any) => row.skillName || row.toolName },
  {
    colKey: 'args', title: t('audit.args'), width: 200, ellipsis: true,
    cell: (_: any, { row }: any) => {
      try { return Object.values(JSON.parse(row.args)).join(', ') }
      catch { return row.args }
    }
  },
  { colKey: 'result', title: t('audit.result'), ellipsis: true },
  {
    colKey: 'durationMs', title: t('audit.duration'), width: 80,
    cell: (_: any, { row }: any) => `${row.durationMs}${t('audit.ms')}`
  },
  {
    colKey: 'success', title: t('audit.status'), width: 80,
    cell: (_: any, { row }: any) => h(Tag, {
      theme: row.success ? 'success' : 'danger', variant: 'light', size: 'small',
    }, () => row.success ? t('audit.success') : t('audit.failed'))
  },
])

const loadRecords = async () => {
  try {
    pageResult.value = await GetAuditRecords({
      days: queryDays.value,
      toolName: queryTool.value,
      page: currentPage.value,
      pageSize: 20,
    } as any)
  } catch (e) { console.error(e) }
}

const loadData = async () => {
  currentPage.value = 1
  try {
    stats.value = await GetAuditStats(queryDays.value)
  } catch (e) { console.error(e) }
  await loadRecords()
}

onMounted(() => loadData())
</script>

<style lang="less" scoped>
.audit-page { padding: 20px; height: 100%; overflow-y: auto; box-sizing: border-box; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; h3 { margin: 0; font-size: 18px; } }
.header-actions { display: flex; align-items: center; gap: 8px; }

.stat-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-bottom: 16px; }
.stat-card {
  display: flex; align-items: center; gap: 12px; padding: 16px;
  border-radius: var(--td-radius-large); background: var(--td-bg-color-container); border: 1px solid var(--td-border-level-1-color);
}
.stat-icon { width: 40px; height: 40px; border-radius: var(--td-radius-medium); display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.stat-value { font-size: 22px; font-weight: 700; }
.stat-label { font-size: 12px; color: var(--td-text-color-secondary); margin-top: 2px; }

.stat-row { display: flex; gap: 12px; margin-bottom: 16px; }
.stat-half { flex: 1; }

.bar-item { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.bar-label { width: 120px; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex-shrink: 0; }
.bar-track { flex: 1; height: 8px; border-radius: 4px; background: var(--td-bg-color-page); overflow: hidden; }
.bar-fill { height: 100%; border-radius: 4px; background: var(--td-brand-color); transition: width 0.5s; &.success { background: var(--td-success-color); } }
.bar-count { font-size: 12px; color: var(--td-text-color-secondary); width: 36px; text-align: right; flex-shrink: 0; }

.records-card { margin-bottom: 16px; }
.pagination { display: flex; justify-content: flex-end; margin-top: 12px; }
</style>
