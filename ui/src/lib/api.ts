export type RunOutcome = 'ok' | 'failed' | 'skipped'

export type RoutineSummary = {
  id: string
  name: string
  description: string
  state: 'active' | 'paused' | 'broken'
  version: number
  next_run?: string
  runs: RunOutcome[]
  cost_month_usd: number
  capabilities: string[]
  schedule: string
  default_schedule?: string
  params?: RoutineParam[]
  values?: Record<string, unknown>
  gallery_update?: { name: string; description: string; settings: string[] }
}

export type ParamType = 'text' | 'number' | 'boolean' | 'date' | 'time' | 'location' | 'select' | 'multiselect' | 'email' | 'destinations'
export type RoutineParam = { name: string; label: string; type: ParamType; default?: unknown; options?: string[]; help?: string }
export type Place = { name: string; latitude: number; longitude: number; timezone?: string; country?: string }
export type CloudConfig = { kind: '' | 's3' | 'drive'; every: 'daily' | 'weekly'; keep: number; endpoint?: string; region?: string; bucket?: string; prefix?: string }
export type CloudRun = { at: string; ok: boolean; name?: string; size?: number; error?: string }
export type CloudState = { config: CloudConfig; has_keys: boolean; has_passphrase: boolean; google: { connected: boolean; drive: boolean }; last?: CloudRun; next?: string }
export type CloudFile = { name: string; size: number; modified: string }

export type CapRisk = 'read' | 'notify' | 'reversible' | 'irreversible'
export type McpInput = { kind: 'env' | 'header' | 'arg'; name: string; description?: string; secret?: boolean; required?: boolean; default?: string }
export type McpListing = { id: string; name: string; title: string; description: string; version: string; repository?: string; kind: 'npm' | 'pypi' | 'remote' | 'unsupported'; command?: string; args?: string[]; url?: string; inputs: McpInput[] }
export type McpSource = { name: string; command?: string; args?: string[]; url?: string; env?: Record<string, string>; headers?: Record<string, string>; arg_values?: Record<string, string> }
export type McpTool = { tool: string; capability: string; title?: string; description: string; risk: CapRisk }

export type RecentRun = { id: number; routine: string; name: string; version: number; started_at: string; ended_at?: string; outcome: 'ok' | 'failed' | 'skipped' | 'running'; error?: string; cost_usd: number; calls: number }

export type Organized = { at?: string; checked: number; merged: { kept: string; dropped: string }[]; error?: string }

export type Chat = { id: string; title: string; assistant?: string; created_at: string; updated_at: string; turns: number }
export type ChatAction = { capability: string; text: string; risk: CapRisk; args: unknown }
export type ChatTurn = { id: string; request: string; state: 'running' | 'ready' | 'compiling' | 'done' | 'failed' | 'discarded'; summary?: string; error?: string; cost_usd: number; routine?: string; created_at: string
  actions: ChatAction[]; done?: { state: 'running' | 'done' | 'failed'; at: string; results: { capability: string; ok: boolean; error?: string }[] } }

export type Assistant = { id: string; name: string; emoji: string; instructions: string; capabilities: string[] }
export type CapabilitySpec = { name: string; risk: CapRisk; signature: string; returns: string }

export type RemoteState = {
  tailscale: { state: 'off' | 'starting' | 'needs_login' | 'needs_funnel' | 'running' | 'error'; auth_url?: string; url?: string; error?: string }
  lan: { on: boolean; url?: string; error?: string }
}
export type Destination = { id: string; label: string; kind: string; ready: boolean }
export type TelegramBot = { id: string; name: string; username: string; chat?: number; chat_name?: string }

export type Scenario = {
  now: string
  responses: { capability: string; result: unknown }[]
  judgments?: Record<string, Record<string, number>>
  expect: { capability: string; count?: number; contains?: string[]; not_contains?: string[] }[]
}

export type Routine = {
  name: string
  description: string
  manifest: { schedule: string; capabilities: string[]; judgments?: Record<string, string>; locale?: string }
  code: string
  tests: ({ name: string } & Scenario)[]
}

export type Run = { id: number; version: number; started_at: string; ended_at?: string; outcome: string; error?: string; cost_usd: number; calls: number }
export type Version = { version: number; routine: Routine; reason: string; approved_by: string; created_at: string }

export type Role = 'owner' | 'member' | 'guest'
export type Person = { id: string; name: string; role: Role; chat?: number; responsible?: string; invite?: string; created: string; mail: boolean; calendar: boolean }

export type GalleryItem = {
  id: string
  author: string
  author_name: string
  routine: Routine
  hash: string
  published: string
  installed: boolean
  report: { verified: boolean; problems?: string[]; uses: string[]; sends: boolean; risk: 'read' | 'notify' | 'reversible' | 'irreversible' }
}

export type CatalogKind = {
  id: string
  title: string
  description: string
  help: string
  fields: { name: string; label: string; placeholder?: string; secret?: boolean; optional?: boolean }[]
  capabilities: { name: string; risk: 'read' | 'notify' | 'reversible' | 'irreversible'; signature: string; returns: string }[]
  configured: boolean
  source?: string
  values: Record<string, string>
  external?: boolean
}

export type MigrationSource = 'openclaw' | 'hermes'
export type MigrationPlan = {
  from: MigrationSource
  home: string
  timezone?: string
  memories: { text: string; topic: string }[]
  tasks: { name: string; prompt: string; schedule: string; timezone?: string; deliver?: string; enabled: boolean }[]
  rules: { file: string; text: string }[]
  skills: { name: string; description: string; capabilities: string[]; missing: string[]; secrets?: string[]; verdict: 'works' | 'partial' | 'no' }[]
  telegram: { has_bot: boolean; allowed?: string[] }
  mail: { address?: string; imap?: string; smtp?: string }
  warnings: string[]
}
export type ImportOptions = { memories: boolean; rules: boolean; tasks: boolean; secrets: boolean; trust: boolean }
export type Imported = { memories: number; rules: number; tasks: number; telegram: boolean; mail: boolean }

export type Exploration = {
  id: string
  request: string
  state: 'running' | 'ready' | 'compiling' | 'done' | 'failed' | 'discarded' | 'imported'
  summary: string
  routine?: string
  cost_usd: number
  error?: string
  created_at: string
  updated_at: string
  trace?: { judgments?: Record<string, Record<string, number>>; questions?: Record<string, string> }
  candidate?: Routine
}

export type VEvent<T = Record<string, unknown>> = { id: number; ts: string; type: string; actor: string; data: T; hash: string }

export type ActionRecord = {
  source: string
  capability: string
  scope?: string
  risk: 'read' | 'notify' | 'reversible' | 'irreversible'
  args: unknown
  result?: unknown
  error?: string
  verdict: string
  reason?: string
  rule?: string
  dry_run?: boolean
  ms: number
}

export type AppState = { budget: { spent: number; limit: number }; healthy: boolean; broken: number; awaiting: number; telegram_paired: boolean; log_intact: boolean; claude: boolean }
export type Connection = { kind: 'telegram' | 'mail' | 'calendar' | 'whatsapp' | 'jev' | 'claude'; configured: boolean; detail?: string; paired?: boolean; pairing_code?: string; bot?: string; webhook?: string; verify_token?: string }
export type Settings = { zone: string; locale: string; judge_backend: 'local' | 'jev' | 'llm'; ollama_model: string; local_judge_url: string; explore_model: string; compile_model: string; judge_model: string; gallery_url?: string; email_channel?: boolean; protection_network?: boolean }

export type Receipt = VEvent<ActionRecord> & { action: ActionRecord & { done?: string; approved?: string }; undoable: boolean; undo_until?: string; undone: boolean }
export type Approval = { id: string; action: { capability: string; scope?: string; args: unknown; risk: number; source: string }; text: string; reason: string; created: string }
export type Rule = { id: string; text: string; when: { capabilities?: string[]; min_risk?: string; source?: string; args_contain?: string[]; hosts?: string[]; people?: string[]; roles?: string[] }; then: 'allow' | 'reversible' | 'ask' | 'block'; off?: boolean }
export type CostView = { today: number; limit: number; month: number; projected_month: number; by_day: Record<string, number>; by_source: Record<string, number> }

export type Fact = { id: string; text: string; topic: string; source: string; trust: 'high' | 'low'; person?: string; created: string }
export type MemoryVersion = { hash: string; message: string; when: string }

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, { method, headers: body === undefined ? {} : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin' })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) throw new ApiError(res.status, data?.error ?? res.statusText)
  return data as T
}

export const api = {
  state: () => request<AppState>('GET', '/api/state'),
  cloud: () => request<CloudState>('GET', '/api/backup/cloud'),
  saveCloud: (c: CloudConfig & { access_key?: string; secret_key?: string; passphrase?: string }) => request<CloudState>('PUT', '/api/backup/cloud', c),
  cloudOff: () => request<CloudState>('DELETE', '/api/backup/cloud'),
  cloudRun: () => request<CloudRun>('POST', '/api/backup/cloud/run'),
  cloudFiles: () => request<CloudFile[]>('GET', '/api/backup/cloud/files'),
  cloudRestore: (name: string, passphrase?: string) => request<{ secrets: number }>('POST', '/api/backup/cloud/restore', { name, passphrase }),
  mcpSearch: (q: string, cursor = '') => request<{ servers: McpListing[]; next: string }>('GET', `/api/connectors/registry?q=${encodeURIComponent(q)}&cursor=${encodeURIComponent(cursor)}`),
  mcpProbe: (s: McpSource) => request<{ tools: McpTool[] }>('POST', '/api/connectors/probe', s),
  mcpAdd: (s: McpSource & { description?: string; source?: string; tools: Record<string, CapRisk> }) => request<{ loaded: number }>('POST', '/api/connectors/add', s),
  mcpRemove: (name: string) => request<{ loaded: number }>('DELETE', `/api/connectors/${name}`),
  recentRuns: (outcome = '', before = 0) => request<RecentRun[]>('GET', `/api/runs?outcome=${outcome}&before=${before}&limit=50`),
  searchMemory: (q: string) => request<{ facts: (Fact & { by: 'words' | 'meaning'; score: number })[]; meaning: boolean }>('GET', `/api/memory/search?q=${encodeURIComponent(q)}`),
  organizeMemory: () => request<Organized>('POST', '/api/memory/organize'),
  memoryOrganized: () => request<Organized>('GET', '/api/memory/organized'),
  chats: () => request<Chat[]>('GET', '/api/chats'),
  chat: (id: string) => request<{ chat: Chat; turns: ChatTurn[] }>('GET', `/api/chats/${id}`),
  newChat: (text: string, assistant = '') => request<{ chat: string; turn: string }>('POST', '/api/chats', { text, assistant }),
  sendChat: (id: string, text: string) => request<{ chat: string; turn: string }>('POST', `/api/chats/${id}/messages`, { text }),
  chatDo: (id: string, turn: string) => request<ChatTurn>('POST', `/api/chats/${id}/turns/${turn}/do`),
  deleteChat: (id: string) => request<{ deleted: string }>('DELETE', `/api/chats/${id}`),
  assistants: () => request<Assistant[]>('GET', '/api/assistants'),
  saveAssistant: (a: Assistant) => request<Assistant>('PUT', `/api/assistants/${a.id}`, a),
  deleteAssistant: (id: string) => request<{ deleted: string }>('DELETE', `/api/assistants/${id}`),
  capabilities: () => request<CapabilitySpec[]>('GET', '/api/capabilities'),
  openLink: (url: string) => request<{ opened: boolean }>('POST', '/api/open', { url }),
  routines: () => request<RoutineSummary[]>('GET', '/api/routines'),
  routine: (id: string) => request<{ summary: RoutineSummary; routine: Routine; versions: Version[]; runs: Run[] }>('GET', `/api/routines/${id}`),
  routineAction: (id: string, action: 'run' | 'pause' | 'resume' | 'repair') => request<{ run?: Run; error?: string; exploration?: string }>('POST', `/api/routines/${id}/${action}`),
  explorations: (state?: string) => request<Exploration[]>('GET', `/api/explorations${state ? `?state=${state}` : ''}`),
  exploration: (id: string) => request<{ exploration: Exploration; actions: VEvent<ActionRecord>[] }>('GET', `/api/explorations/${id}`),
  explore: (text: string) => request<{ id: string }>('POST', '/api/explorations', { request: text }),
  compile: (id: string) => request<RoutineSummary>('POST', `/api/explorations/${id}/compile`),
  discard: (id: string) => request<void>('POST', `/api/explorations/${id}/discard`),
  events: (q: { types?: string; q?: string; limit?: number }) => {
    const p = new URLSearchParams()
    if (q.types) p.set('types', q.types)
    if (q.q) p.set('q', q.q)
    if (q.limit) p.set('limit', String(q.limit))
    return request<VEvent[]>('GET', `/api/events?${p}`)
  },
  receipts: (q?: string) => request<Receipt[]>('GET', `/api/receipts${q ? `?q=${encodeURIComponent(q)}` : ''}`),
  undo: (id: number) => request<void>('POST', `/api/actions/${id}/undo`),
  approvals: () => request<Approval[]>('GET', '/api/approvals'),
  answer: (id: string, answer: 'once' | 'run' | 'always' | 'deny') => request<void>('POST', `/api/approvals/${id}/${answer}`),
  rules: () => request<Rule[]>('GET', '/api/rules'),
  saveRules: (rules: Rule[]) => request<Rule[]>('PUT', '/api/rules', rules),
  compileRule: (text: string) => request<{ rule: Rule; summary: string }>('POST', '/api/rules/compile', { text }),
  testRule: (rule: Rule) => request<{ matches: { event: number; ts: string; source: string; capability: string; was: string; would_be: string }[] }>('POST', '/api/rules/test', rule),
  cost: () => request<CostView>('GET', '/api/cost'),
  setup: () => request<{ done: boolean; demo: boolean; telegram: boolean; mail: boolean; calendar: boolean; preset: string; claude: boolean }>('GET', '/api/setup'),
  setupDone: () => request<void>('POST', '/api/setup/done'),
  preset: (preset: 'conservative' | 'balanced' | 'liberal') => request<Rule[]>('PUT', '/api/rules/preset', { preset }),
  memory: () => request<{ facts: Fact[]; history: MemoryVersion[] }>('GET', '/api/memory'),
  addFact: (text: string, topic: string, person?: string) => request<Fact>('POST', '/api/memory', { text, topic, person: person || undefined }),
  removeFact: (id: string) => request<void>('DELETE', `/api/memory/${id}`),
  confirmFact: (id: string) => request<void>('POST', `/api/memory/${id}/confirm`),
  restoreMemory: (hash: string) => request<void>('POST', `/api/memory-versions/${hash}/restore`),
  settings: () => request<Settings>('GET', '/api/settings'),
  saveSettings: (s: Settings) => request<Settings>('PUT', '/api/settings', s),
  setBudget: (daily_usd: number) => request<void>('PUT', '/api/budget', { daily_usd }),
  migratePreview: (from: MigrationSource, home: string) => request<MigrationPlan>('POST', '/api/migrate/preview', { from, home }),
  migrateApply: (from: MigrationSource, home: string, o: ImportOptions) => request<Imported>('POST', '/api/migrate/apply', { from, home, ...o }),
  exploreImported: (id: string) => request<{ id: string }>('POST', `/api/explorations/${id}/explore`),
  pairing: () => request<{ base: string; devices: { id: string; name: string; created: string; last_seen?: string }[] }>('GET', '/api/pairing'),
  setPairing: (base: string, device?: string) => request<{ base: string; link?: string; id?: string }>('POST', '/api/pairing', { base, device }),
  remote: () => request<RemoteState>('GET', '/api/remote'),
  switchRemote: (kind: 'tailscale' | 'lan', on: boolean) => request<RemoteState>('POST', `/api/remote/${kind}/${on ? 'on' : 'off'}`),
  revokeDevice: (id: string) => request<void>('DELETE', `/api/devices/${id}`),
  people: () => request<Person[]>('GET', '/api/people'),
  addPerson: (name: string, role: Role, responsible: string) => request<Person>('POST', '/api/people', { name, role, responsible }),
  updatePerson: (id: string, role: Role, responsible: string) => request<Person>('PUT', `/api/people/${id}`, { role, responsible }),
  removePerson: (id: string) => request<void>('DELETE', `/api/people/${id}`),
  personConnection: (id: string, kind: 'mail' | 'calendar', body: Record<string, string>) => request<void>('PUT', `/api/people/${id}/connections/${kind}`, body),
  gallery: (fresh = false) => request<GalleryItem[]>('GET', `/api/gallery${fresh ? '?fresh=1' : ''}`),
  installFromGallery: (id: string) => request<RoutineSummary>('POST', `/api/gallery/${id}/install`),
  publishRoutine: (id: string, author: string) => request<{ entry: unknown; author: { name: string; key: string } }>('POST', `/api/routines/${id}/publish`, { author }),
  catalog: () => request<{ connectors: CatalogKind[]; broken: string[] }>('GET', '/api/catalog'),
  setCatalog: (id: string, values: Record<string, string>) => request<void>('PUT', `/api/catalog/${id}`, values),
  removeCatalog: (id: string) => request<void>('DELETE', `/api/catalog/${id}`),
  checkCatalog: (id: string) => request<{ ok: boolean; detail?: string }>('POST', `/api/catalog/${id}/check`),
  protection: () => request<{ version: number; entries: number; updated: string; fetched?: string; enabled: boolean; blocked: number }>('GET', '/api/protection'),
  updateFromGallery: (id: string) => request<RoutineSummary>('POST', `/api/routines/${id}/update`),
  saveRoutineSettings: (id: string, schedule: string, params: Record<string, unknown>) => request<RoutineSummary>('PUT', `/api/routines/${id}/settings`, { schedule, params }),
  destinations: () => request<Destination[]>('GET', '/api/destinations'),
  geocode: (q: string) => request<Place[]>('GET', `/api/geocode?q=${encodeURIComponent(q)}`),
  bots: () => request<TelegramBot[]>('GET', '/api/telegram/bots'),
  addBot: (name: string, token: string) => request<TelegramBot>('POST', '/api/telegram/bots', { name, token }),
  detectBot: (id: string) => request<TelegramBot>('POST', `/api/telegram/bots/${id}/detect`),
  removeBot: (id: string) => request<void>('DELETE', `/api/telegram/bots/${id}`),
  connections: () => request<Connection[]>('GET', '/api/connections'),
  googleStart: (client_id: string, client_secret: string) => request<{ url: string; redirect: string }>('POST', '/api/oauth/google/start', { client_id, client_secret }),
  connect: (kind: string, body: Record<string, string>) => request<void>('PUT', `/api/connections/${kind}`, body),
  disconnect: (kind: string) => request<void>('DELETE', `/api/connections/${kind}`),
}
