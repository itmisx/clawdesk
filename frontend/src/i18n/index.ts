import { createI18n } from 'vue-i18n'
import zh from './zh'
import en from './en'
import ja from './ja'
import ko from './ko'
import fr from './fr'
import de from './de'
import es from './es'
import ru from './ru'
import pt from './pt'
import ar from './ar'

const savedLocale = localStorage.getItem('locale') || 'zh'

const i18n = createI18n({
  legacy: false,
  locale: savedLocale,
  fallbackLocale: 'en',
  messages: { zh, en, ja, ko, fr, de, es, ru, pt, ar },
})

export default i18n

export function setLocale(locale: string) {
  ;(i18n.global.locale as any).value = locale
  localStorage.setItem('locale', locale)
}

export function getLocale(): string {
  return (i18n.global.locale as any).value
}
