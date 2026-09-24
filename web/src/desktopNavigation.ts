interface DesktopNavigationRuntime {
  EventsOn: (name: string, callback: (page: unknown) => void) => () => void
  EventsEmit: (name: string, ...data: unknown[]) => void
}

export function listenForDesktopNavigation(onLogs: () => void): () => void {
  const runtime = (window as Window & { runtime?: DesktopNavigationRuntime }).runtime
  if (!runtime?.EventsOn || !runtime?.EventsEmit) return () => {}
  const unsubscribe = runtime.EventsOn('desktop:navigate', page => {
    if (page !== 'activity') return
    onLogs()
    runtime.EventsEmit('desktop:navigation-applied', page)
  })
  // Register before announcing readiness so clicks made during startup are replayed.
  runtime.EventsEmit('desktop:navigation-ready')
  return unsubscribe
}
