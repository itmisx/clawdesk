import { createApp } from 'vue'
import TDesign from 'tdesign-vue-next'
import { createPinia } from 'pinia'
import TDesignChat from '@tdesign-vue-next/chat'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import 'tdesign-vue-next/es/style/index.css'

// 恢复保存的主题
const savedTheme = localStorage.getItem('theme') || 'light'
if (savedTheme === 'dark') {
  document.documentElement.setAttribute('theme-mode', 'dark')
}

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(TDesign)
app.use(TDesignChat)
app.use(i18n)
app.mount('#app')
