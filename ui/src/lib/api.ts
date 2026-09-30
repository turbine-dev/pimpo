export type RunOutcome = 'ok' | 'failed' | 'skipped'

export type Watch = { capability: string; args?: Record<string, unknown>; key: string; every?: string }

export type RoutineSummary = {
  watch?: Watch
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
  model?: string
  effort?: string
  thinks?: boolean
  webhook?: boolean
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
export type OpenApiOp = { id: string; method: string; path: string; summary: string; risk: CapRisk }
export type OpenApiPreview = { title: string; description: string; base: string; keys: { name: string; description: string }[]; operations: OpenApiOp[]; unsupported: { id: string; why: string }[] }
export type OpenApiSource = { url?: string; spec?: string; header?: string }
export type McpTool = { tool: string; capability: string; title?: string; description: string; risk: CapRisk; claimed?: CapRisk }

export type RecentRun = { id: number; routine: string; name: string; version: number; started_at: string; ended_at?: string; outcome: 'ok' | 'failed' | 'skipped' | 'running'; error?: string; cost_usd: number; calls: number }

export type Organized = { at?: string; checked: number; merged: { kept: string; dropped: string }[]; error?: string }

export type Chat = { id: string; title: string; assistant?: string; created_at: string; updated_at: string; turns: number }
export type ChatHit = { chat: string; title: string; turn: string; snippet: string; at: string }
export type ChatAction = { capability: string; text: string; risk: CapRisk; args: unknown }
export type Effort = 'low' | 'medium' | 'high' | 'max'
export const EFFORTS: Effort[] = ['low', 'medium', 'high', 'max']
export type Routed = { model: string; tier?: 'simple' | 'normal' | 'hard'; by: 'jev' | 'rules' | 'fixed' | 'default'; effort?: Effort; effort_by?: 'fixed' | 'auto' | 'default' }
export type ChatTurn = { model?: Routed; id: string; request: string; state: 'running' | 'ready' | 'compiling' | 'done' | 'failed' | 'discarded'; summary?: string; error?: string; cost_usd: number; routine?: string; created_at: string
  actions: ChatAction[]; done?: { state: 'running' | 'done' | 'failed'; at: string; results: { capability: string; ok: boolean; error?: string }[] } }

export type Assistant = { id: string; name: string; emoji: string; instructions: string; capabilities: string[] }
export type InstalledSkill = { id: string; name: string; description: string; source: string; capabilities: string[]; scripts: string[]; unsupported: string[]; installed: string }
export type SkillPreview = { token: string; exists: boolean; skill: { id: string; name: string; description: string; body: string; files: string[]; scripts: string[]; suggested: string[]; unsupported: string[]; secrets: string[] } }
export type CapabilitySpec = { name: string; risk: CapRisk; signature: string; returns: string }

export type SysComponent = { id: string; group: 'channel' | 'account' | 'service' | 'access' | 'backup' | 'brain'; name: string; state: 'ok' | 'off' | 'error' | 'waiting'; detail?: string }
export type SystemState = {
  process: { cpu_percent: number; heap_bytes: number; sys_bytes: number; goroutines: number; uptime_s: number }
  host: { cpus: number; load: [number, number, number]; mem_total: number; disk_free: number; disk_size: number }
  activity: { explorations: number; runs: number; approvals: number; runs_ok_today: number; runs_failed_today: number }
  components: SysComponent[]
  version: string
}

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
  manifest: { schedule: string; capabilities: string[]; judgments?: Record<string, string>; locale?: string; uses?: string[] }
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
  report: { verified: boolean; problems?: string[]; uses: string[]; sends: boolean; outside?: string[]; risk: 'read' | 'notify' | 'reversible' | 'irreversible' }
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

export type AppState = { budget: { spent: number; limit: number }; healthy: boolean; broken: number; awaiting: number; approvals?: number; telegram_paired: boolean; log_intact: boolean; claude: boolean; person?: string; role?: 'owner' | 'member' | 'guest'; name?: string; admin_account?: boolean }
export type Connection = { kind: 'telegram' | 'mail' | 'calendar' | 'whatsapp' | 'jev' | 'claude'; configured: boolean; detail?: string; paired?: boolean; pairing_code?: string; bot?: string; webhook?: string; verify_token?: string }
export type Settings = { labs_on?: string[]; suggest_off?: boolean; learn_off?: boolean; zone: string; locale: string; judge_backend: 'local' | 'jev' | 'llm'; ollama_model: string; local_judge_url: string; explore_model: string; compile_model: string; judge_model: string; gallery_url?: string; email_channel?: boolean; protection_network?: boolean; mute?: string[]; labs_off?: string[]; models?: ModelOption[]; ollama_url?: string; lmstudio_url?: string; custom_url?: string; fallbacks?: Partial<Record<Job, string[]>>; auto_off?: boolean; auto_light?: string; auto_strong?: string; efforts?: Partial<Record<Job, Effort>>; voice?: 'auto' | 'local' | 'system' | 'openai' | 'elevenlabs'; voice_model?: string; voice_name?: string; chat_voice?: string; chat_voice_model?: string; chat_voice_name?: string }
export type ModelOption = { id: string; price_in: number; price_out: number }
export type Job = 'explore' | 'compile' | 'judge'
export type Provider = { id: string; name: string; key_url?: string; needs_key: boolean; local?: boolean }
export type CatalogModel = { id: string; name: string; price_in: number; price_out: number; priced: boolean; context?: number; free?: boolean }
export type Found = { claude_code?: string; ollama: CatalogModel[]; ollama_url: string; ollama_up?: boolean; lmstudio: CatalogModel[]; lmstudio_url: string; lmstudio_up?: boolean; codex?: string; codex_login?: boolean; opencode?: string; qwen_code?: string; apps?: string[] }
export type Reminder = { id: string; at: string; text: string }
export type LocalItem = { id: string; kind: 'engine' | 'voice' | 'transcriber'; name: string; about?: string; languages?: string[]; size: number; installed: boolean; quality?: number }
export type LocalJob = { id: string; item: string; name: string; state: 'downloading' | 'verifying' | 'unpacking' | 'done' | 'failed' | 'cancelled'; done: number; total: number; detail?: string; error?: string; started: string }
export type LocalView = { engine?: LocalItem; voices: LocalItem[]; transcribers?: LocalItem[]; jobs: LocalJob[]; free: number; memory: number; suggestions: { model: string; about: string; size: number; min_ram: number }[]; ollama: { url: string; up?: boolean; models?: CatalogModel[] } }
export type QuickChoice = { kind: 'claude_code' | 'codex' | 'opencode' | 'ollama' | 'lmstudio' | 'provider'; provider?: string; key?: string; model?: string }
export type OpencodeModel = { id: string; provider: string; name: string; subscription: boolean }
export type ModelTest = { ok: boolean; text?: string; cost_usd?: number; ms?: number; error?: string; problem?: string }

export type Receipt = VEvent<ActionRecord> & { action: ActionRecord & { done?: string; approved?: string }; undoable: boolean; undo_until?: string; undone: boolean }
// A Need is one thing waiting for the person signed in, from /api/needs.
// The kinds the server may add later (credential requests, lessons) show
// with their title and an open link until the UI learns their buttons.
export type NeedKind = 'approval' | 'question' | 'failed_routine' | 'job_error' | 'job_planned' | 'exploration_ready' | 'suggestion' | 'system'
export type Need = { kind: NeedKind; id: string; title: string; detail?: string; created?: string; urgency: number; expires?: string; link?: string; actions: string[]; options?: string[]; proposal?: string; risk?: number }
export type Needs = { items: Need[]; counts: Partial<Record<NeedKind, number>>; total: number }
export type Approval = { id: string; action: { capability: string; scope?: string; args: unknown; risk: number; source: string }; text: string; reason: string; created: string }
export type Rule = { id: string; text: string; when: { capabilities?: string[]; min_risk?: string; source?: string; args_contain?: string[]; hosts?: string[]; people?: string[]; roles?: string[] }; then: 'allow' | 'reversible' | 'ask' | 'block'; off?: boolean }
export type CostView = { today: number; limit: number; month: number; projected_month: number; by_day: Record<string, number>; by_source: Record<string, number>; by_model?: Record<string, number>; by_job?: Record<string, number>; calls_by_model?: Record<string, number>; subscription?: { today: number; month: number; by_model: Record<string, number> } }

export type JobPart = { id: string; title: string; instructions: string; capabilities: string[]; state: 'waiting' | 'running' | 'done' | 'failed'; exploration?: string; summary?: string; error?: string; cost_usd: number; attempts: number }
export type LongJob = { id: string; request: string; state: 'planned' | 'running' | 'reporting' | 'done' | 'stopped' | 'failed'; budget_usd: number; spent_usd: number; parts: JobPart[]; report?: string; error?: string; created: string; updated: string }
export type WidgetKind = 'metric' | 'progress' | 'list' | 'status' | 'text' | 'table' | 'chart'
export type WidgetSnap = {
  kind: WidgetKind; title: string; subtitle?: string; value?: number; goal?: number; unit?: string; trend?: number
  status?: 'ok' | 'warn' | 'alert'; text?: string; link?: string
  items?: { title: string; detail?: string; badge?: string; value?: string; status?: string; link?: string }[]
  columns?: string[]; rows?: string[][]; chart?: 'line' | 'area' | 'bar' | 'donut'
  series?: { name?: string; points: { label?: string; y: number }[] }[]; meta?: Record<string, string>
}
export type WidgetView = { id: string; source: 'routine' | 'builtin' | 'status'; routine?: string; kind: WidgetKind; title: string; snapshot: WidgetSnap; history?: { t: string; v: number }[]; updated: string; stale?: boolean; shared?: boolean; mine: boolean }
export type LayoutItem = { id: string; x: number; y: number; w: number; h: number }
export type Dashboard = { id: string; name: string; emoji: string; position: number; layout: LayoutItem[]; shared: boolean; mine: boolean; updated: string }
export type PhoneShare = 'location' | 'camera' | 'shortcuts'
export type PhonePlace = { name: string; lat?: number; lon?: number; radius?: number }
export type PhoneState = { places: PhonePlace[]; shares: PhoneShare[]; device?: { id: string; name: string; shares: PhoneShare[]; has_key: boolean } }

export type Fact = { id: string; text: string; topic: string; source: string; trust: 'high' | 'low' | 'learned'; person?: string; created: string }
export type MemoryVersion = { hash: string; message: string; when: string }

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export type RepoChange = { id: string; name: string; new: boolean; added: string[]; removed: string[]; tests: number; problems: string[]; hash: string }
export type RepoView = { path: string; git: boolean; remote: boolean; head?: string; changes: RepoChange[]; broken: Record<string, string>; error?: string }

export type Finding = { id: string; group: string; name: string; state: 'ok' | 'warn' | 'fail'; detail?: string; fix?: string; link?: string }

export type MyDevice = { id: string; name: string; created: string; last_seen?: string; session?: boolean; pending?: boolean; current?: boolean }

export type Snapshot = { name: string; label: string; when: string; bytes: number }

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
  openapiPreview: (s: OpenApiSource) => request<OpenApiPreview>('POST', '/api/connectors/openapi/preview', s),
  openapiAdd: (s: OpenApiSource & { name: string; operations: Record<string, CapRisk>; keys: Record<string, string> }) => request<{ loaded: number }>('POST', '/api/connectors/openapi/add', s),
  mcpRemove: (name: string) => request<{ loaded: number }>('DELETE', `/api/connectors/${name}`),
  recentRuns: (outcome = '', before = 0) => request<RecentRun[]>('GET', `/api/runs?outcome=${outcome}&before=${before}&limit=50`),
  searchMemory: (q: string) => request<{ facts: (Fact & { by: 'words' | 'meaning'; score: number })[]; meaning: boolean }>('GET', `/api/memory/search?q=${encodeURIComponent(q)}`),
  organizeMemory: () => request<Organized>('POST', '/api/memory/organize'),
  memoryOrganized: () => request<Organized>('GET', '/api/memory/organized'),
  chats: () => request<Chat[]>('GET', '/api/chats'),
  searchChats: (q: string) => request<ChatHit[]>('GET', `/api/chats/search?q=${encodeURIComponent(q)}`),
  chat: (id: string) => request<{ chat: Chat; turns: ChatTurn[]; model: string; effort?: string }>('GET', `/api/chats/${id}`),
  newChat: (text: string, assistant = '', model = '', effort = '') => request<{ chat: string; turn: string }>('POST', '/api/chats', { text, assistant, model, effort }),
  sendChat: (id: string, text: string, model = '', effort = '') => request<{ chat: string; turn: string }>('POST', `/api/chats/${id}/messages`, { text, model, effort }),
  chatDo: (id: string, turn: string) => request<ChatTurn>('POST', `/api/chats/${id}/turns/${turn}/do`),
  deleteChat: (id: string) => request<{ deleted: string }>('DELETE', `/api/chats/${id}`),
  assistants: () => request<Assistant[]>('GET', '/api/assistants'),
  browserLogin: (url: string) => request<{ state: string }>('POST', '/api/browser/login', { url }),
  skills: () => request<InstalledSkill[]>('GET', '/api/skills'),
  jobs: () => request<LongJob[]>('GET', '/api/jobs'),
  job: (id: string) => request<LongJob>('GET', `/api/jobs/${id}`),
  createJob: (req: string, budget_usd: number) => request<LongJob>('POST', '/api/jobs', { request: req, budget_usd }),
  startJob: (id: string) => request<LongJob>('POST', `/api/jobs/${id}/start`),
  stopJob: (id: string) => request<LongJob>('POST', `/api/jobs/${id}/stop`),
  saveAccount: (name: string) => request<{ name: string }>('PUT', '/api/account', { name }),
  passkeys: () => request<{ id: string; name: string; address: string; created: string; last_used?: string }[]>('GET', '/api/passkeys'),
  deletePasskey: (id: string) => request<{ removed: string }>('DELETE', `/api/passkeys/${encodeURIComponent(id)}`),
  myDevices: () => request<MyDevice[]>('GET', '/api/me/devices'),
  signOutDevice: (id: string) => request<{ revoked: string }>('DELETE', `/api/me/devices/${encodeURIComponent(id)}`),
  dashboards: () => request<Dashboard[]>('GET', '/api/dashboards'),
  createDashboard: (name: string, emoji: string) => request<Dashboard>('POST', '/api/dashboards', { name, emoji }),
  saveDashboard: (id: string, d: Partial<Pick<Dashboard, 'name' | 'emoji' | 'position' | 'layout' | 'shared'>>) => request<Dashboard>('PUT', `/api/dashboards/${id}`, d),
  deleteDashboard: (id: string) => request<{ removed: string }>('DELETE', `/api/dashboards/${id}`),
  dashboardWidgets: (id: string) => request<Record<string, WidgetView | { id: string; hidden: true }>>('GET', `/api/dashboards/${id}/widgets`),
  widgets: () => request<WidgetView[]>('GET', '/api/widgets'),
  shareWidget: (id: string, shared: boolean) => request<WidgetView>('PUT', `/api/widgets/${encodeURIComponent(id)}`, { shared }),
  deleteWidget: (id: string) => request<{ removed: string }>('DELETE', `/api/widgets/${encodeURIComponent(id)}`),
  refreshWidget: (id: string, confirm = false) => request<{ run: unknown }>('POST', `/api/widgets/${encodeURIComponent(id)}/refresh`, { confirm }),
  phone: () => request<PhoneState>('GET', '/api/phone'),
  phoneShares: (shares: PhoneShare[]) => request<{ shares: PhoneShare[] }>('POST', '/api/phone/shares', { shares }),
  phoneKey: () => request<{ key: string }>('POST', '/api/phone/key'),
  addPlace: (p: PhonePlace) => request<PhonePlace[]>('POST', '/api/phone/places', p),
  deletePlace: (name: string) => request<PhonePlace[]>('DELETE', `/api/phone/places/${encodeURIComponent(name)}`),
  phoneLocation: (lat: number, lon: number, accuracy: number) => request<{ at: string[] }>('POST', '/api/phone/location', { lat, lon, accuracy }),
  phonePhoto: async (file: File) => {
    const form = new FormData()
    form.append('photo', file)
    const res = await fetch('/api/phone/photo', { method: 'POST', body: form, credentials: 'same-origin' })
    const body = await res.json().catch(() => ({}))
    if (!res.ok) throw new ApiError(res.status, body.error ?? res.statusText)
    return body as { id: string; text: string; note: string }
  },
  previewSkill: async (src: { file?: File; url?: string }) => {
    if (src.file) {
      const form = new FormData()
      form.append('file', src.file)
      const res = await fetch('/api/skills/preview', { method: 'POST', body: form, credentials: 'same-origin' })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(body.error ?? res.statusText)
      return body as SkillPreview
    }
    return request<SkillPreview>('POST', '/api/skills/preview', { url: src.url })
  },
  installSkill: (token: string, capabilities: string[]) => request<InstalledSkill>('POST', '/api/skills/install', { token, capabilities }),
  deleteSkill: (id: string) => request<{ state: string }>('DELETE', `/api/skills/${id}`),
  saveAssistant: (a: Assistant) => request<Assistant>('PUT', `/api/assistants/${a.id}`, a),
  deleteAssistant: (id: string) => request<{ deleted: string }>('DELETE', `/api/assistants/${id}`),
  capabilities: () => request<CapabilitySpec[]>('GET', '/api/capabilities'),
  models: () => request<{ keys: Record<string, boolean>; claude_code: boolean; providers: Provider[]; auto?: { light: string; strong: string; base: string; weigher: 'jev' | 'rules' } }>('GET', '/api/models'),
  detectModels: () => request<Found>('GET', '/api/models/detect'),
  voice: () => request<{ openai_key: boolean; elevenlabs_key: boolean; openai_voices: string[]; openai_prices: Record<string, number>; elevenlabs_voices?: { id: string; name: string }[]; elevenlabs_error?: string }>('GET', '/api/voice'),
  setElevenLabsKey: (key: string) => request<{ ok: boolean }>('PUT', '/api/voice/elevenlabs-key', { key }),
  local: () => request<LocalView>('GET', '/api/local'),
  installLocal: (id: string) => request<LocalJob>('POST', `/api/local/install/${id}`),
  removeLocal: (id: string) => request<{ ok: boolean }>('DELETE', `/api/local/${id}`),
  cancelDownload: (id: string) => request<{ ok: boolean }>('POST', `/api/local/jobs/${id}/cancel`),
  pullOllama: (model: string) => request<LocalJob>('POST', '/api/local/ollama/pull', { model }),
  removeOllama: (model: string) => request<{ ok: boolean }>('DELETE', `/api/local/ollama/${model}`),
  voiceSample: (language: string, forWhat: 'chat' | 'routines' = 'routines') => request<{ id: string; seconds: number; voice: string }>('POST', '/api/local/sample', { language, for: forWhat }),
  media: () => request<{ id: string; title: string; at: string }[]>('GET', '/api/media'),
  webhook: (id: string) => request<{ urls?: { local?: string; lan?: string; public?: string } }>('GET', `/api/routines/${id}/webhook`),
  setWebhook: (id: string, action: 'on' | 'off' | 'rotate') => request<{ urls?: { local?: string; lan?: string; public?: string } }>('POST', `/api/routines/${id}/webhook/${action}`),
  needs: () => request<Needs>('GET', '/api/needs'),
  questions: () => request<{ id: string; routine?: string; question: string; options: string[]; asked: string }[]>('GET', '/api/questions'),
  suggestions: () => request<{ id: string; title: string; why: string; request: string; made: string }[]>('GET', '/api/suggestions'),
  suggestion: (id: string, action: 'accept' | 'dismiss') => request<{ exploration?: string }>('POST', `/api/suggestions/${id}/${action}`),
  answerQuestion: (id: string, index: number) => request<{ text: string }>('POST', `/api/questions/${id}/answer`, { index }),
  spotify: () => request<{ connected: boolean; client_id: boolean; redirect: string }>('GET', '/api/spotify'),
  spotifyStart: (clientId: string) => request<{ url: string; redirect: string }>('POST', '/api/oauth/spotify/start', { client_id: clientId }),
  spotifyOff: () => request<{ ok: boolean }>('DELETE', '/api/spotify'),
  reminders: () => request<Reminder[]>('GET', '/api/reminders'),
  cancelReminder: (id: string) => request<{ ok: boolean }>('DELETE', `/api/reminders/${id}`),
  quickSetup: (c: QuickChoice) => request<{ explore: string; judge: string; text?: string; cost_usd: number; ms?: number }>('POST', '/api/setup/model', c),
  opencodeModels: () => request<OpencodeModel[]>('GET', '/api/models/opencode'),
  modelCatalog: (provider: string) => request<CatalogModel[]>('GET', `/api/models/catalog/${provider}`),
  tryModel: async (id: string, price_in: number, price_out: number): Promise<ModelTest> => {
    const res = await fetch('/api/models/test', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id, price_in, price_out }) })
    return res.json()
  },
  setModelKey: (provider: string, key: string) => request<{ set: boolean }>('PUT', `/api/models/keys/${provider}`, { key }),
  testModel: (id: string) => request<{ ok: boolean; text: string; cost_usd: number }>('POST', '/api/models/test', { id }),
  system: () => request<SystemState>('GET', '/api/system'),
  doctor: () => request<Finding[]>('POST', '/api/doctor'),
  repo: () => request<RepoView>('GET', '/api/repo'),
  setRepo: (path: string) => request<RepoView>('PUT', '/api/repo', { path }),
  repoExport: () => request<{ written: number; commit?: string; view: RepoView }>('POST', '/api/repo/export'),
  repoPull: () => request<RepoView>('POST', '/api/repo/pull'),
  repoPush: () => request<{ pushed: boolean; view: RepoView }>('POST', '/api/repo/push'),
  repoApply: (id: string) => request<{ applied: boolean; view: RepoView }>('POST', `/api/repo/apply/${id}`),
  snapshots: () => request<{ snapshots: Snapshot[]; staged?: string; available: boolean; version?: string }>('GET', '/api/snapshots'),
  createSnapshot: () => request<Snapshot>('POST', '/api/snapshots'),
  stageRestore: (name: string) => request<{ staged: string }>('POST', '/api/snapshots/restore', { name }),
  cancelRestore: () => request<{ staged: string }>('DELETE', '/api/snapshots/restore'),
  openLink: (url: string) => request<{ opened: boolean }>('POST', '/api/open', { url }),
  routines: () => request<RoutineSummary[]>('GET', '/api/routines'),
  routine: (id: string) => request<{ summary: RoutineSummary; routine: Routine; versions: Version[]; runs: Run[]; state: Record<string, unknown>; used_by: string[] }>('GET', `/api/routines/${id}`),
  routineWidget: (id: string, kind: string) => request<{ exploration: string }>('POST', `/api/routines/${id}/widget`, { kind }),
  routineAction: (id: string, action: 'run' | 'pause' | 'resume' | 'repair' | 'forget') => request<{ run?: Run; error?: string; exploration?: string }>('POST', `/api/routines/${id}/${action}`),
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
  addFact: (text: string, topic: string, shared?: boolean) => request<Fact>('POST', '/api/memory', { text, topic, shared: !!shared }),
  removeFact: (id: string) => request<void>('DELETE', `/api/memory/${id}`),
  confirmFact: (id: string) => request<void>('POST', `/api/memory/${id}/confirm`),
  restoreMemory: (hash: string) => request<void>('POST', `/api/memory-versions/${hash}/restore`),
  settings: () => request<Settings>('GET', '/api/settings'),
  saveSettings: (s: Settings) => request<Settings>('PUT', '/api/settings', s),
  setBudget: (daily_usd: number) => request<void>('PUT', '/api/budget', { daily_usd }),
  migratePreview: (from: MigrationSource, home: string) => request<MigrationPlan>('POST', '/api/migrate/preview', { from, home }),
  migrateApply: (from: MigrationSource, home: string, o: ImportOptions) => request<Imported>('POST', '/api/migrate/apply', { from, home, ...o }),
  exploreImported: (id: string) => request<{ id: string }>('POST', `/api/explorations/${id}/explore`),
  pairing: () => request<{ base: string; devices: { id: string; name: string; person: string; created: string; last_seen?: string; pending?: boolean }[] }>('GET', '/api/pairing'),
  setPairing: (base: string, device?: string, person?: string) => request<{ base: string; link?: string; id?: string; expires?: string }>('POST', '/api/pairing', { base, device, person: person || undefined }),
  remote: () => request<RemoteState>('GET', '/api/remote'),
  switchRemote: (kind: 'tailscale' | 'lan', on: boolean) => request<RemoteState>('POST', `/api/remote/${kind}/${on ? 'on' : 'off'}`),
  revokeDevice: (id: string) => request<void>('DELETE', `/api/devices/${id}`),
  people: () => request<Person[]>('GET', '/api/people'),
  addPerson: (name: string, role: Role, responsible: string) => request<Person>('POST', '/api/people', { name, role, responsible }),
  updatePerson: (id: string, role: Role, responsible: string) => request<Person>('PUT', `/api/people/${id}`, { role, responsible }),
  removePerson: (id: string) => request<void>('DELETE', `/api/people/${id}`),
  personConnection: (id: string, kind: 'mail' | 'calendar', body: Record<string, string>) => request<void>('PUT', `/api/people/${id}/connections/${kind}`, body),
  gallery: (fresh = false) => request<GalleryItem[]>('GET', `/api/gallery${fresh ? '?fresh=1' : ''}`),
  installFromGallery: (id: string, confirm = false) => request<RoutineSummary>('POST', `/api/gallery/${id}/install`, confirm ? { confirm } : undefined),
  publishRoutine: (id: string, author: string) => request<{ entry: unknown; author: { name: string; key: string } }>('POST', `/api/routines/${id}/publish`, { author }),
  catalog: () => request<{ connectors: CatalogKind[]; broken: string[] }>('GET', '/api/catalog'),
  setCatalog: (id: string, values: Record<string, string>) => request<void>('PUT', `/api/catalog/${id}`, values),
  removeCatalog: (id: string) => request<void>('DELETE', `/api/catalog/${id}`),
  checkCatalog: (id: string) => request<{ ok: boolean; detail?: string }>('POST', `/api/catalog/${id}/check`),
  protection: () => request<{ version: number; entries: number; updated: string; fetched?: string; enabled: boolean; blocked: number }>('GET', '/api/protection'),
  updateFromGallery: (id: string) => request<RoutineSummary>('POST', `/api/routines/${id}/update`),
  saveRoutineSettings: (id: string, schedule: string, params: Record<string, unknown>, watchEvery = '', model = '', effort = '') => request<RoutineSummary>('PUT', `/api/routines/${id}/settings`, { schedule, params, watch_every: watchEvery, model: model || 'auto', effort: effort || 'auto' }),
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
