import { defineStore } from 'pinia'

export type ThemeMode = 'dark' | 'light'

const STORAGE_KEY = 'gowms_theme'

/** 主题切换：驱动 html.dark class，全站 CSS 变量自动切换 */
export const useThemeStore = defineStore('theme', {
  state: () => ({
    theme: (localStorage.getItem(STORAGE_KEY) as ThemeMode) || 'dark',
  }),
  actions: {
    /** 应用启动时调用，同步 class 状态 */
    init() {
      document.documentElement.classList.toggle('dark', this.theme === 'dark')
    },
    toggle() {
      this.theme = this.theme === 'dark' ? 'light' : 'dark'
      localStorage.setItem(STORAGE_KEY, this.theme)
      this.init()
    },
  },
})
