// --- Platform types ---

export interface User {
  githubId: number;
  login: string;
  name: string;
  avatarUrl: string;
}

export interface Connection {
  id: string;
  type: "azure-storage" | "github-repo";
  name: string;
  config: Record<string, string>;
  status: "verified" | "unverified";
  createdAt: string;
}

export interface CreateConnectionRequest {
  type: "azure-storage" | "github-repo";
  name: string;
  config: Record<string, string>;
}

export interface Repo {
  owner: string;
  repo: string;
  fullName: string;
}

export interface EvalSpec {
  path: string;
  name: string;
}

export interface TriggerRunConfig {
  owner: string;
  repo: string;
  evalPath: string;
  model: string;
  workers: number;
  parallel: boolean;
}

export interface RunQueueItem {
  id: string;
  status: string;
  evalPath: string;
  model: string;
  workers: number;
  createdAt: string;
}

// --- Existing types ---

export interface SummaryResponse {
  totalRuns: number;
  totalTasks: number;
  passRate: number;
  avgTokens: number;
  avgCost: number;
  avgDuration: number;
}

export interface RunSummary {
  id: string;
  spec: string;
  model: string;
  judgeModel?: string;
  outcome: string;
  passCount: number;
  taskCount: number;
  tokens: number;
  cost: number;
  duration: number;
  timestamp: string;
  weightedScore?: number;
}

export interface GraderResult {
  name: string;
  type: string;
  passed: boolean;
  score: number;
  weight?: number;
  message: string;
}

export interface TranscriptEvent {
  type: string;
  content?: string;
  message?: string;
  toolCallId?: string;
  toolName?: string;
  arguments?: unknown;
  toolResult?: unknown;
  success?: boolean;
}

export interface BootstrapCI {
  lower: number;
  upper: number;
  mean: number;
  confidenceLevel: number;
}

export interface SessionDigest {
  totalTurns: number;
  toolCallCount: number;
  tokensIn: number;
  tokensOut: number;
  tokensTotal: number;
  toolsUsed: string[];
  errors: string[];
}

export interface TaskResult {
  name: string;
  outcome: string;
  score: number;
  weightedScore?: number;
  duration: number;
  graderResults: GraderResult[];
  transcript?: TranscriptEvent[];
  sessionDigest?: SessionDigest;
  bootstrapCI?: BootstrapCI;
  isSignificant?: boolean;
}

export interface RunDetail extends RunSummary {
  tasks: TaskResult[];
}

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export function fetchSummary(): Promise<SummaryResponse> {
  return fetchJSON<SummaryResponse>("/api/summary");
}

export function fetchRuns(
  sort = "timestamp",
  order = "desc",
): Promise<RunSummary[]> {
  return fetchJSON<RunSummary[]>(
    `/api/runs?sort=${encodeURIComponent(sort)}&order=${encodeURIComponent(order)}`,
  );
}

export function fetchRunDetail(id: string): Promise<RunDetail> {
  return fetchJSON<RunDetail>(`/api/runs/${encodeURIComponent(id)}`);
}

// --- Platform API functions ---

export function fetchCurrentUser(): Promise<User> {
  return fetchJSON<User>("/api/auth/me");
}

export async function logout(): Promise<void> {
  const res = await fetch("/api/auth/logout", { method: "POST" });
  if (!res.ok) throw new Error(`Logout failed: ${res.status}`);
}

export function fetchConnections(): Promise<Connection[]> {
  return fetchJSON<Connection[]>("/api/connections");
}

export async function createConnection(
  data: CreateConnectionRequest,
): Promise<Connection> {
  const res = await fetch("/api/connections", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`Failed to create connection: ${res.status}`);
  return res.json() as Promise<Connection>;
}

export async function testConnection(
  id: string,
): Promise<{ ok: boolean; message: string }> {
  const res = await fetch("/api/connections/test", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id }),
  });
  if (!res.ok) throw new Error(`Connection test failed: ${res.status}`);
  return res.json() as Promise<{ ok: boolean; message: string }>;
}

export async function deleteConnection(id: string): Promise<void> {
  const res = await fetch(`/api/connections/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error(`Failed to delete connection: ${res.status}`);
}

export function fetchRepos(): Promise<Repo[]> {
  return fetchJSON<Repo[]>("/api/repos");
}

export function fetchRepoEvals(owner: string, repo: string): Promise<EvalSpec[]> {
  return fetchJSON<EvalSpec[]>(
    `/api/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/evals`,
  );
}

export async function triggerRun(
  config: TriggerRunConfig,
): Promise<{ runId: string }> {
  const res = await fetch("/api/runs/trigger", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
  if (!res.ok) throw new Error(`Failed to trigger run: ${res.status}`);
  return res.json() as Promise<{ runId: string }>;
}

export function fetchRunQueue(): Promise<RunQueueItem[]> {
  return fetchJSON<RunQueueItem[]>("/api/runs/queue");
}

export async function cancelRun(id: string): Promise<void> {
  const res = await fetch(`/api/runs/cancel/${encodeURIComponent(id)}`, {
    method: "POST",
  });
  if (!res.ok) throw new Error(`Failed to cancel run: ${res.status}`);
}
