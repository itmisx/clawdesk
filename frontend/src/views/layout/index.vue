<template>
  <t-layout class="main">
    <!-- 标题栏区域（与系统红绿灯同行） -->
    <div class="titlebar" style="--wails-draggable: drag">
      <div class="titlebar-spacer"></div>
      <div class="titlebar-actions" style="--wails-draggable: no-drag">
        <t-dropdown :options="langOptions" @click="handleLangChange" trigger="click">
          <t-button variant="text" shape="square" size="small">
            <template #icon><t-icon name="translate" size="16px" /></template>
          </t-button>
        </t-dropdown>
        <t-button variant="text" shape="square" size="small" @click="toggleTheme">
          <template #icon>
            <t-icon :name="isDark ? 'sunny' : 'moon'" size="16px" />
          </template>
        </t-button>
      </div>
    </div>
    <t-layout>
      <t-aside class="sidebar" :class="{ collapsed }">
        <div class="sidebar-logo" :class="{ collapsed }">
          <img src="@/assets/logo.png" class="app-icon" alt="ClawDesk" />
          <span v-if="!collapsed" class="app-name">ClawDesk</span>
        </div>
        <t-menu
          :value="activeMenu"
          :collapsed="collapsed"
          @change="changeHandler"
        >
          <t-menu-item value="chat">
            <template #icon><t-icon name="chat-message" /></template>
            {{ t('nav.chat') }}
          </t-menu-item>
          <t-menu-group :title="collapsed ? '' : t('nav.configGroup')">
            <t-menu-item value="model-configure">
              <template #icon><t-icon name="cpu" /></template>
              {{ t('nav.model') }}
            </t-menu-item>
            <t-menu-item value="skill-configure">
              <template #icon><t-icon name="extension" /></template>
              {{ t('nav.skill') }}
            </t-menu-item>
            <t-menu-item value="channel-configure">
              <template #icon><t-icon name="link" /></template>
              {{ t('nav.channel') }}
            </t-menu-item>
          </t-menu-group>
          <t-menu-group :title="collapsed ? '' : t('nav.statsGroup')">
            <t-menu-item value="usage-dashboard">
              <template #icon><t-icon name="chart-bar" /></template>
              {{ t('nav.dashboard') }}
            </t-menu-item>
          </t-menu-group>
          <t-menu-group :title="collapsed ? '' : t('nav.auditGroup')">
            <t-menu-item value="skill-audit">
              <template #icon><t-icon name="file-safety" /></template>
              {{ t('nav.skillAudit') }}
            </t-menu-item>
            <t-menu-item value="storage-audit">
              <template #icon><t-icon name="data-base" /></template>
              {{ t('nav.storageAudit') }}
            </t-menu-item>
          </t-menu-group>
          <t-menu-group :title="collapsed ? '' : t('nav.debugGroup')">
            <t-menu-item value="request-log">
              <template #icon><t-icon name="file-paste" /></template>
              {{ t('nav.requestLog') }}
            </t-menu-item>
          </t-menu-group>
        </t-menu>
        <div class="sidebar-footer">
          <div class="collapse-btn" @click="collapsed = !collapsed">
            <t-icon
              :name="collapsed ? 'chevron-right-double' : 'chevron-left-double'"
              size="16px"
            />
          </div>
        </div>
      </t-aside>
      <t-content class="content-container">
        <router-view />
      </t-content>
    </t-layout>
    <div class="status-bar">
      <div class="status-item">
        <t-icon name="cpu" size="13px" />
        <span>CPU {{ sysInfo.cpuPercent.toFixed(1) }}%</span>
      </div>
      <div class="status-item">
        <t-icon name="memory" size="13px" />
        <span>MEM {{ sysInfo.memUsedMB }}MB</span>
        <div class="status-bar-mini">
          <div class="status-bar-fill" :style="{ width: sysInfo.memPercent + '%' }"></div>
        </div>
      </div>
      <div class="status-item">
        <t-icon name="hard-drive" size="13px" />
        <span>{{ formatStorage(sysInfo.storageUsedKB) }}</span>
      </div>
      <div class="status-item" :class="{ 'status-ready': sysInfo.embeddingReady, 'status-pending': !sysInfo.embeddingReady }">
        <t-icon :name="sysInfo.embeddingReady ? 'check-circle' : 'time'" size="13px" />
        <span>{{ sysInfo.embeddingReady ? t('status.embeddingReady') : t('status.embeddingLoading') }}</span>
      </div>
    </div>
  </t-layout>
</template>

<script lang="ts" setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { setLocale, getLocale } from '@/i18n'
import * as Runtime from '../../../wailsjs/runtime/runtime'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const collapsed = ref(false)
const isDark = ref(false)

const sysInfo = reactive({
  cpuPercent: 0,
  memUsedMB: 0,
  memTotalMB: 0,
  memPercent: 0,
  storageUsedKB: 0,
  goRoutines: 0,
  embeddingReady: false,
})

const formatStorage = (kb: number): string => {
  if (kb < 1024) return kb + 'KB'
  return (kb / 1024).toFixed(1) + 'MB'
}

const activeMenu = computed(() => route.name as string)

const langOptions = computed(() => [
  { content: t('lang.zh'), value: 'zh' },
  { content: t('lang.en'), value: 'en' },
  { content: t('lang.ja'), value: 'ja' },
  { content: t('lang.ko'), value: 'ko' },
  { content: t('lang.fr'), value: 'fr' },
  { content: t('lang.de'), value: 'de' },
  { content: t('lang.es'), value: 'es' },
  { content: t('lang.ru'), value: 'ru' },
  { content: t('lang.pt'), value: 'pt' },
  { content: t('lang.ar'), value: 'ar' },
])

const changeHandler = (menuName: string) => {
  router.push({ name: menuName })
}

const toggleTheme = () => {
  isDark.value = !isDark.value
  if (isDark.value) {
    document.documentElement.setAttribute('theme-mode', 'dark')
    localStorage.setItem('theme', 'dark')
  } else {
    document.documentElement.removeAttribute('theme-mode')
    localStorage.setItem('theme', 'light')
  }
}

const handleLangChange = (data: { value: string }) => {
  setLocale(data.value)
}

onMounted(() => {
  const savedTheme = localStorage.getItem('theme') || 'light'
  isDark.value = savedTheme === 'dark'

  Runtime.EventsOn('system:info', (info: any) => {
    for (const key in info) {
      if ((sysInfo as any)[key] !== info[key]) {
        (sysInfo as any)[key] = info[key]
      }
    }
  })
})

onUnmounted(() => {
  Runtime.EventsOff('system:info')
})
</script>

<style scoped lang="less">
.main {
  height: 100%;
  width: 100%;
}

.titlebar {
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: 0 8px;
  background: var(--td-bg-color-container);
  border-bottom: 1px solid var(--td-border-level-1-color);
  flex-shrink: 0;
}

.titlebar-spacer {
  flex: 1;
}

.titlebar-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.sidebar {
  width: 180px;
  transition: width 0.25s ease;
  border-right: 1px solid var(--td-border-level-1-color);
  background: linear-gradient(180deg, #f0f5ff 0%, #e8f0fe 100%);
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  flex-shrink: 0;
  &.collapsed { width: 64px; }
}

.sidebar-footer {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  padding: 4px 8px 8px;
}

.collapse-btn {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  cursor: pointer;
  color: var(--td-text-color-placeholder);
  transition: all 0.2s;
  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-secondary);
  }
}

.sidebar :deep(.t-default-menu) {
  background: transparent;
  flex: 1;
  overflow-y: auto;
  width: 100% !important;
}

[theme-mode='dark'] .sidebar {
  background: var(--td-bg-color-container);
}

.sidebar-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 16px 8px;
  &.collapsed {
    justify-content: center;
    padding: 16px 0 8px;
  }
}

.app-icon {
  // width: 28px;
  height: 48px;
  border-radius: 6px;
  margin-left: -10px;
}

.app-name {
  font-weight: 700;
  font-size: 15px;
  color: var(--td-text-color-primary);
  white-space: nowrap;
}

.content-container {
  height: calc(100vh - 38px - 28px);
  overflow: hidden;
  background: var(--td-bg-color-page);
}

.status-bar {
  height: 28px;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 12px;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-border-level-1-color);
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 4px;
}

.status-bar-mini {
  width: 40px;
  height: 4px;
  border-radius: 2px;
  background: var(--td-bg-color-page);
  overflow: hidden;
}

.status-bar-fill {
  height: 100%;
  border-radius: 2px;
  background: var(--td-brand-color);
  transition: width 0.5s ease;
}

.status-ready {
  color: var(--td-success-color);
}

.status-pending {
  color: var(--td-warning-color);
}
</style>
