// The decision logic, kept free of OpenClaw imports so it can be tested
// on its own. index.ts wires it to the before_tool_call hook.

export type GuardConfig = { url: string; token: string; failOpen?: boolean; timeoutMs?: number }

export type HookResult =
  | undefined
  | { block: true; blockReason: string }
  | { requireApproval: { title: string; description: string; severity: 'warning' | 'critical'; allowedDecisions: ('allow-once' | 'deny')[] } }

export async function decide(cfg: GuardConfig, toolName: string, params: Record<string, unknown>, session?: string): Promise<HookResult> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), cfg.timeoutMs ?? 8000)
  try {
    const res = await fetch(cfg.url.replace(/\/$/, '') + '/api/guard/check', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + cfg.token },
      body: JSON.stringify({ agent: 'openclaw', tool: toolName, params, session }),
      signal: controller.signal,
    })
    if (!res.ok) throw new Error('Zodim answered ' + res.status)
    const ans = (await res.json()) as { decision: string; reason?: string; capability: string }
    if (ans.decision === 'block') return { block: true, blockReason: 'Zodim bloqueou: ' + (ans.reason ?? ans.capability) }
    if (ans.decision === 'ask') {
      return {
        requireApproval: {
          title: `Zodim pede aprovação: ${toolName}`,
          description: ans.reason ?? `Regra para ${ans.capability}`,
          severity: ans.capability === 'guard.delete' || ans.capability === 'guard.exec' ? 'critical' : 'warning',
          allowedDecisions: ['allow-once', 'deny'],
        },
      }
    }
    return undefined
  } catch (err) {
    if (cfg.failOpen) return undefined
    return { block: true, blockReason: 'Zodim não respondeu, então não deixo passar: ' + (err as Error).message }
  } finally {
    clearTimeout(timer)
  }
}
