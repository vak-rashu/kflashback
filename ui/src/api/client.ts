import type {
  ApiResponse,
  TrackedResource,
  ResourceRevision,
  StorageStats,
  KindStats,
  DiffResult,
  AISummary,
  AIDiffSummary,
  AnomalyReport,
  QueryResult,
} from '../types';

const API_BASE = '/api/v1';

async function fetchApi<T>(path: string): Promise<ApiResponse<T>> {
  const res = await fetch(`${API_BASE}${path}`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  return res.json();
}

export async function getStats(): Promise<StorageStats> {
  const res = await fetchApi<StorageStats>('/stats');
  return res.data;
}

export async function getKindStats(): Promise<KindStats[]> {
  const res = await fetchApi<KindStats[]>('/stats/kinds');
  return res.data;
}

export async function listResources(params: {
  kind?: string;
  namespace?: string;
  deleted?: boolean;
  limit?: number;
  offset?: number;
}): Promise<{ data: TrackedResource[]; total: number }> {
  const query = new URLSearchParams();
  if (params.kind) query.set('kind', params.kind);
  if (params.namespace) query.set('namespace', params.namespace);
  if (params.deleted !== undefined) query.set('deleted', String(params.deleted));
  if (params.limit) query.set('limit', String(params.limit));
  if (params.offset) query.set('offset', String(params.offset));

  const res = await fetchApi<TrackedResource[]>(`/resources?${query}`);
  return { data: res.data, total: res.meta?.total ?? 0 };
}

export async function getResource(uid: string): Promise<TrackedResource> {
  const res = await fetchApi<TrackedResource>(`/resources/${uid}`);
  return res.data;
}

export async function getHistory(
  uid: string,
  params?: { limit?: number; offset?: number; since?: string; until?: string; eventType?: string }
): Promise<{ data: ResourceRevision[]; total: number }> {
  const query = new URLSearchParams();
  if (params?.limit) query.set('limit', String(params.limit));
  if (params?.offset) query.set('offset', String(params.offset));
  if (params?.since) query.set('since', params.since);
  if (params?.until) query.set('until', params.until);
  if (params?.eventType) query.set('eventType', params.eventType);

  const res = await fetchApi<ResourceRevision[]>(`/resources/${uid}/history?${query}`);
  return { data: res.data, total: res.meta?.total ?? 0 };
}

export async function getRevision(uid: string, revision: number): Promise<ResourceRevision> {
  const res = await fetchApi<ResourceRevision>(`/resources/${uid}/revisions/${revision}`);
  return res.data;
}

export async function reconstructAtRevision(
  uid: string,
  revision: number
): Promise<Record<string, unknown>> {
  const res = await fetchApi<Record<string, unknown>>(
    `/resources/${uid}/reconstruct/${revision}`
  );
  return res.data;
}

export async function diffRevisions(
  uid: string,
  from: number,
  to: number
): Promise<DiffResult> {
  const res = await fetchApi<DiffResult>(
    `/resources/${uid}/diff?from=${from}&to=${to}`
  );
  return res.data;
}

// --- AI API ---

export async function getAISummary(uid: string, revision: number): Promise<AISummary> {
  const res = await fetchApi<AISummary>(`/ai/summarize/${uid}/revisions/${revision}`);
  return res.data;
}

export async function getAIDiffSummary(uid: string, from: number, to: number): Promise<AIDiffSummary> {
  const res = await fetchApi<AIDiffSummary>(`/ai/summarize/${uid}/diff?from=${from}&to=${to}`);
  return res.data;
}

export async function getAnomalies(hours?: number): Promise<AnomalyReport> {
  const query = hours ? `?hours=${hours}` : '';
  const res = await fetchApi<AnomalyReport>(`/ai/anomalies${query}`);
  return res.data;
}

export async function askAI(question: string): Promise<QueryResult> {
  const res = await fetch(`${API_BASE}/ai/query`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ question }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  const json = await res.json();
  return json.data;
}
