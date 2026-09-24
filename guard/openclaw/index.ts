import { definePluginEntry } from 'openclaw/plugin-sdk/plugin-entry'
import { decide, type GuardConfig } from './guard.ts'

export default definePluginEntry({
  id: 'vigia-guard',
  name: 'Vigia Guard',
  description: "Every tool call passes Vigia's rules and the shared protection list first.",
  register(api) {
    const cfg = api.pluginConfig as GuardConfig
    api.on(
      'before_tool_call',
      (event, ctx) => decide(cfg, event.toolName, event.params, ctx.sessionId ?? ctx.runId),
      { priority: 1000, timeoutMs: 10_000 },
    )
  },
})
