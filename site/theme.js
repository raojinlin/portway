// Apply before the stylesheet paints; the preference is independent of the desktop app.
(() => {
  const key = 'portway-site-theme'
  const valid = (value) => ['system', 'light', 'dark'].includes(value)
  let preference = 'system'
  let media
  try { media = window.matchMedia('(prefers-color-scheme: dark)') } catch { /* Older or restricted browser. */ }
  try {
    const stored = window.localStorage.getItem(key)
    if (valid(stored)) preference = stored
  } catch { /* Local files and private browsing may deny storage. */ }

  function apply() {
    const dark = preference === 'dark' || (preference === 'system' && media?.matches)
    document.documentElement.dataset.theme = dark ? 'dark' : 'light'
    const meta = document.querySelector('meta[name="theme-color"]')
    if (meta) meta.content = dark ? '#181a1d' : '#f3f5f7'
    const select = document.getElementById('theme-select')
    if (select) select.value = preference
  }

  apply()
  if (media?.addEventListener) media.addEventListener('change', apply)
  else media?.addListener?.(apply)
  window.addEventListener('storage', (event) => {
    if (event.key !== key && event.key !== null) return
    preference = valid(event.newValue) ? event.newValue : 'system'
    apply()
  })
  document.addEventListener('DOMContentLoaded', () => {
    const select = document.getElementById('theme-select')
    if (!select) return
    select.value = preference
    select.addEventListener('change', () => {
      if (!valid(select.value)) return
      preference = select.value
      try { window.localStorage.setItem(key, preference) } catch { /* Keep the in-memory choice. */ }
      apply()
    })
  })
})()
