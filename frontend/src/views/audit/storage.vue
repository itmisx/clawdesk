<template>
  <div class="audit-page">
    <div class="page-header">
      <h3>{{ t('storageAudit.title') }}</h3>
      <div class="header-actions">
        <t-select v-model="queryDays" size="small" style="width: 120px" @change="loadData">
          <t-option :value="7" label="最近 7 天" />
          <t-option :value="14" label="最近 14 天" />
          <t-option :value="30" label="最近 30 天" />
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
          <t-icon name="data-base" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.totalOps }}</div>
          <div class="stat-label">{{ t('storageAudit.totalOps') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(0, 168, 112, 0.1); color: #00a870">
          <t-icon name="check-circle" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.successOps }}</div>
          <div class="stat-label">{{ t('storageAudit.successOps') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(227, 77, 89, 0.1); color: #e34d59">
          <t-icon name="close-circle" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.failedOps }}</div>
          <div class="stat-label">{{ t('storageAudit.failedOps') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: rgba(237, 123, 47, 0.1); color: #ed7b2f">
          <t-icon name="file" size="20px" />
        </div>
        <div class="stat-info">
          <div class="stat-value">{{ formatBytes(stats.totalBytes) }}</div>
          <div class="stat-label">{{ t('storageAudit.totalBytes') }}</div>
        </div>
      </div>
    </div>

    <!-- 按类型统计 -->
    <t-card :title="t('storageAudit.byType')" class="section-card" v-if="stats.totalOps > 0">
      <div v-for="(count, type_) in stats.byType" :key="type_" class="bar-item">
        <span class="bar-label">{{ t('storageAudit.' + type_) || type_ }}</span>
        <div class="bar-track">
          <div class="bar-fill" :style="{ width: (count / stats.totalOps * 100) + '%' }"></div>
        </div>
        <span class="bar-count">{{ count }}</span>
      </div>
    </t-card>

    <!-- 记录表格 -->
    <t-card :title="t('storageAudit.recentRecords')" class="section-card">
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
      <t-empty v-else :description="t('storageAudit.noData')" />
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
import { GetStorageAuditRecords, GetStorageAuditStats } from '../../../wailsjs/go/agent/App'

const { t } = useI18n()

const queryDays = ref(7)
const currentPage = ref(1)
const pageResult = ref<any>({ records: [], total: 0, page: 1, pageSize: 20 })
const stats = ref<any>({ totalOps: 0, successOps: 0, failedOps: 0, totalBytes: 0, byType: {} })

const columns = computed(() => [
  { colKey: 'timestamp', title: t('storageAudit.time'), width: 160,
    cell: (_: any, { row }: any) => new Date(row.timestamp).toLocaleString() },
  { colKey: 'type', title: t('storageAudit.type'), width: 120,
    cell: (_: any, { row }: any) => t('storageAudit.' + row.type) || row.type },
  { colKey: 'sessionId', title: t('storageAudit.session'), width: 100, ellipsis: true,
    cell: (_: any, { row }: any) => row.sessionId ? row.sessionId.slice(-8) : '-' },
  { colKey: 'fileName', title: t('storageAudit.file'), width: 120, ellipsis: true },
  { colKey: 'detail', title: t('storageAudit.detail'), ellipsis: true },
  { colKey: 'size', title: t('storageAudit.size'), width: 80,
    cell: (_: any, { row }: any) => row.size ? formatBytes(row.size) : '-' },
  { colKey: 'durationMs', title: t('storageAudit.duration'), width: 70,
    cell: (_: any, { row }: any) => row.durationMs ? `${row.durationMs}${t('storageAudit.ms')}` : '-' },
  { colKey: 'success', title: t('storageAudit.status'), width: 70,
    cell: (_: any, { row }: any) => h(Tag, {
      theme: row.success ? 'success' : 'danger', variant: 'light', size: 'small',
    }, () => row.success ? t('storageAudit.success') : t('storageAudit.failed'))
  },
])

const formatBytes = (bytes: number): string => {
  if (!bytes) return '0'
  if (bytes < 1024) return bytes + 'B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + 'KB'
  return (bytes / 1024 / 1024).toFixed(1) + 'MB'
}

const loadRecords = async () => {
  try {
    pageResult.value = await GetStorageAuditRecords({
      days: queryDays.value,
      page: currentPage.value,
      pageSize: 20,
    } as any)
  } catch (e) { console.error(e) }
}

const loadData = async () => {
  currentPage.value = 1
  try {
    stats.value = await GetStorageAuditStats(queryDays.value)
  } catch (e) { console.error(e) }
  await loadRecords()
}

onMounted(() => loadData())
</script>

<style lang="less" scoped>
.audit-page { padding: 20px; height: 100%; overflow-y: auto; box-sizing: border-box; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; h3 { margin: 0; font-size: 18px; } }
.header-actions { display: flex; align-items: center; gap: 8px; }
.stat-cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 16px; }
.stat-card { display: flex; align-items: center; gap: 12px; padding: 16px; border-radius: var(--td-radius-large); background: var(--td-bg-color-container); border: 1px solid var(--td-border-level-1-color); }
.stat-icon { width: 40px; height: 40px; border-radius: var(--td-radius-medium); display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.stat-value { font-size: 22px; font-weight: 700; }
.stat-label { font-size: 12px; color: var(--td-text-color-secondary); margin-top: 2px; }
.section-card { margin-bottom: 16px; }
.bar-item { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.bar-label { width: 100px; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex-shrink: 0; }
.bar-track { flex: 1; height: 8px; border-radius: 4px; background: var(--td-bg-color-page); overflow: hidden; }
.bar-fill { height: 100%; border-radius: 4px; background: var(--td-brand-color); transition: width 0.5s; }
.bar-count { font-size: 12px; color: var(--td-text-color-secondary); width: 36px; text-align: right; flex-shrink: 0; }
.pagination { display: flex; justify-content: flex-end; margin-top: 12px; }
</style>
