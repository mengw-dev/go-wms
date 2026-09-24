export const DATA_CHANGED_EVENT = 'wms:data-changed'

export function emitDataChanged(): void {
  window.dispatchEvent(new Event(DATA_CHANGED_EVENT))
}

export function onDataChanged(handler: () => void): () => void {
  window.addEventListener(DATA_CHANGED_EVENT, handler)
  return () => window.removeEventListener(DATA_CHANGED_EVENT, handler)
}

export const OPEN_DEMO_CONSOLE_EVENT = 'wms:open-demo-console'

export function openDemoConsole(): void {
  window.dispatchEvent(new Event(OPEN_DEMO_CONSOLE_EVENT))
}

export type DemoConsoleScenario = 'inbound' | 'outbound' | 'stocktake' | 'full'

export const RUN_DEMO_SCENARIO_EVENT = 'wms:run-demo-scenario'

export function runDemoScenarioInConsole(scenario: DemoConsoleScenario): void {
  window.dispatchEvent(new CustomEvent<{ scenario: DemoConsoleScenario }>(RUN_DEMO_SCENARIO_EVENT, { detail: { scenario } }))
}
