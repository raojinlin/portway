interface DesktopNavigationRuntime {
  EventsOn: (name: string, callback: (page: unknown) => void) => () => void
  EventsEmit: (name: string, ...data: unknown[]) => void
}

export interface ConnectionNavigation { name: string; showHistory: boolean }

export function listenForDesktopNavigation(onLogs: () => void, onConnections?: (target: ConnectionNavigation) => void): () => void {
  const runtime = (window as Window & { runtime?: DesktopNavigationRuntime }).runtime
  if (!runtime?.EventsOn || !runtime?.EventsEmit) return () => {}
  const unsubscribe = runtime.EventsOn('desktop:navigate', page => {
    if (page === 'activity') onLogs()
    else if (page && typeof page === 'object' && 'page' in page && page.page === 'connections' &&
      'name' in page && typeof page.name === 'string' && page.name.length > 0 &&
      'showHistory' in page && typeof page.showHistory === 'boolean' && onConnections) {
      onConnections({ name: page.name, showHistory: page.showHistory })
    } else return
    runtime.EventsEmit('desktop:navigation-applied', page)
  })
  // Register before announcing readiness so clicks made during startup are replayed.
  runtime.EventsEmit('desktop:navigation-ready')
  return unsubscribe
}
