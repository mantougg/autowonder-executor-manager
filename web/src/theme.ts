// theme.ts：主题机制（ui-guidelines §7.1）——默认跟随系统，设置页可
// 手动覆盖，选择存 localStorage；切换即时生效（改 <html data-theme>，
// 不刷新页面）。
export type ThemeChoice = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'awem-theme'

export function getStoredTheme(): ThemeChoice {
  const v = localStorage.getItem(STORAGE_KEY)
  if (v === 'light' || v === 'dark' || v === 'system') return v
  return 'system'
}

export function storeTheme(choice: ThemeChoice) {
  localStorage.setItem(STORAGE_KEY, choice)
}

function effective(choice: ThemeChoice): 'light' | 'dark' {
  if (choice === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return choice
}

export function applyTheme(choice: ThemeChoice) {
  document.documentElement.dataset.theme = effective(choice)
}

// 启动初始化 + 监听系统主题变化（仅"跟随系统"时重算）。
export function initTheme(onChange?: (choice: ThemeChoice) => void) {
  const choice = getStoredTheme()
  applyTheme(choice)
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  mq.addEventListener('change', () => {
    if (getStoredTheme() === 'system') {
      applyTheme('system')
      onChange?.('system')
    }
  })
}
