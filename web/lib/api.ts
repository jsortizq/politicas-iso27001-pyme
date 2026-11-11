/**
 * API client for the SGSI backend.
 *
 * In development, Next.js rewrites /api/* to the Go server.
 * In production, NEXT_PUBLIC_API_URL points to the Go backend.
 */

const API_BASE = process.env.NEXT_PUBLIC_API_URL || '';

interface RequestOptions {
  method?: string;
  body?: unknown;
  tenantId?: string;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, tenantId } = options;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (tenantId) {
    headers['X-Tenant-ID'] = tenantId;
  }

  const res = await fetch(`${API_BASE}/api${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: 'Unknown error' }));
    throw new ApiError(res.status, error.error || `HTTP ${res.status}`);
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

// ── SoA API ──

export interface SoAEntry {
  id: string;
  tenant_id: string;
  control_id: string;
  control_title?: string;
  applicable: boolean;
  justification: string;
  implementation_pct: number;
  implementation_evidence: string;
  effectiveness_pct: number;
  effectiveness_evidence: string;
  responsible_role: string;
  review_date?: string;
  created_at: string;
  updated_at: string;
}

export interface SoASummary {
  total_controls: number;
  applicable: number;
  not_applicable: number;
  implemented: number;
  partial: number;
  not_implemented: number;
  avg_implementation_pct: number;
  avg_effectiveness_pct: number;
  compliance_score_pct: number;
}

export interface SoAListResponse {
  data: SoAEntry[];
  count: number;
}

export const soaApi = {
  list: (tenantId: string) =>
    request<SoAListResponse>('/v1/soa', { tenantId }),

  get: (tenantId: string, id: string) =>
    request<SoAEntry>(`/v1/soa/${id}`, { tenantId }),

  create: (tenantId: string, entry: Partial<SoAEntry>) =>
    request<SoAEntry>('/v1/soa', { method: 'POST', body: entry, tenantId }),

  update: (tenantId: string, id: string, entry: Partial<SoAEntry>) =>
    request<SoAEntry>(`/v1/soa/${id}`, { method: 'PUT', body: entry, tenantId }),

  delete: (tenantId: string, id: string) =>
    request<void>(`/v1/soa/${id}`, { method: 'DELETE', tenantId }),

  summary: (tenantId: string) =>
    request<SoASummary>('/v1/soa/summary', { tenantId }),
};
