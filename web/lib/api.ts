/**
 * API client for the SGSI backend.
 *
 * Requests go to this same origin; Next.js proxies /api and /auth to the Go
 * server (see next.config.js). The session lives in an HttpOnly cookie set
 * by the backend, so there is no token handling here and the organization
 * is always the one bound to the session.
 */

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    /** Machine-readable reason sent by the API, e.g. "no_organization". */
    public code?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, 'No se pudo conectar con el servidor.');
  }

  if (!res.ok) {
    const payload = await res.json().catch(() => null);
    throw new ApiError(res.status, payload?.error || `Error HTTP ${res.status}`, payload?.code);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

/** Human-readable message for any thrown value. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 403) return 'Tu rol no permite realizar cambios.';
    if (err.status === 409) return 'Ya existe un registro con ese identificador.';
    if (err.status >= 500) return 'El servidor tuvo un problema. Intenta de nuevo.';
    return err.message;
  }
  return 'Ocurrió un error inesperado.';
}

export interface ListResponse<T> {
  data: T[];
  count: number;
}

// ── Auth ──

export type Role = 'owner' | 'admin' | 'editor' | 'auditor' | 'viewer';

export const roleLabels: Record<Role, string> = {
  owner: 'Propietario',
  admin: 'Administrador',
  editor: 'Editor',
  auditor: 'Auditor',
  viewer: 'Lector',
};

export interface User {
  id: string;
  email: string;
  name: string;
  tenant_id: string;
  role: Role;
}

export interface Organization {
  user_id: string;
  tenant_id: string;
  tenant_name: string;
  role: Role;
}

export interface SessionInfo {
  identity: { email: string; name: string };
  /** Member of the active organization; null until one is selected. */
  user: User | null;
  organizations: Organization[];
  needs_organization: boolean;
  can_create_organization: boolean;
}

export interface AuthConfig {
  dev_login: boolean;
  sso: boolean;
  signup: boolean;
}

export const authApi = {
  config: () => request<AuthConfig>('/auth/config'),
  devLogin: (email: string, password: string, tenantId: string) =>
    request<SessionInfo>('/auth/login', 'POST', { email, password, tenant_id: tenantId }),
  logout: () => request<void>('/auth/logout', 'POST'),
  session: () => request<SessionInfo>('/auth/session'),
  createOrganization: (name: string) =>
    request<SessionInfo>('/auth/organizations', 'POST', { name }),
  switchOrganization: (tenantId: string) =>
    request<SessionInfo>('/auth/switch', 'POST', { tenant_id: tenantId }),
};

/** Browser navigation target that starts single sign-on. */
export const SSO_LOGIN_URL = '/auth/oidc/login';

// ── Members ──

export interface Member {
  id: string;
  email: string;
  name: string;
  role: Role;
  status: 'invited' | 'active';
  last_login?: string;
  created_at: string;
}

export const membersApi = {
  list: () => request<ListResponse<Member>>('/api/v1/users'),
  invite: (email: string, name: string, role: Role) =>
    request<Member>('/api/v1/users', 'POST', { email, name, role }),
  setRole: (id: string, role: Role) => request<Member>(`/api/v1/users/${id}`, 'PUT', { role }),
  remove: (id: string) => request<void>(`/api/v1/users/${id}`, 'DELETE'),
};

// ── Audit trail ──

export interface AuditEvent {
  id: number;
  action: string;
  entity_type: string;
  entity_id?: string;
  actor: string;
  detail: Record<string, unknown>;
  created_at: string;
}

export interface AuditPage {
  data: AuditEvent[];
  /** Pass as `before` to load older events; absent on the last page. */
  next_before?: number;
  /** False when the server runs without a database and records nothing. */
  recorded: boolean;
}

export const auditApi = {
  list: (before?: number) =>
    request<AuditPage>(`/api/v1/audit-log?limit=50${before ? `&before=${before}` : ''}`),
};

// ── SoA ──

export interface SoAEntry {
  id: string;
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

export const soaApi = {
  list: () => request<ListResponse<SoAEntry>>('/api/v1/soa'),
  summary: () => request<SoASummary>('/api/v1/soa/summary'),
  initialize: () => request<{ created: number; total: number }>('/api/v1/soa/initialize', 'POST'),
  update: (id: string, entry: Partial<SoAEntry>) =>
    request<SoAEntry>(`/api/v1/soa/${id}`, 'PUT', entry),
};

// ── Assets ──

export interface Asset {
  id: string;
  name: string;
  asset_type: 'information' | 'software' | 'hardware' | 'service' | 'people' | 'facility';
  description: string;
  owner: string;
  location: string;
  classification: 'public' | 'internal' | 'confidential' | 'restricted';
  confidentiality: number;
  integrity: number;
  availability: number;
  criticality: number;
  status: 'active' | 'retired';
}

export type AssetInput = Omit<Asset, 'id' | 'criticality'>;

export const assetsApi = {
  list: () => request<ListResponse<Asset>>('/api/v1/assets'),
  create: (asset: AssetInput) => request<Asset>('/api/v1/assets', 'POST', asset),
  update: (id: string, asset: AssetInput) => request<Asset>(`/api/v1/assets/${id}`, 'PUT', asset),
  remove: (id: string) => request<void>(`/api/v1/assets/${id}`, 'DELETE'),
};

// ── Risks ──

export type RiskRating = 'low' | 'medium' | 'high' | 'critical';

export interface Risk {
  id: string;
  code: string;
  title: string;
  description: string;
  asset: string;
  threat: string;
  vulnerability: string;
  likelihood: number;
  impact: number;
  risk_level: number;
  risk_rating: RiskRating;
  treatment: 'accept' | 'mitigate' | 'transfer' | 'avoid';
  treatment_plan: string;
  residual_likelihood?: number;
  residual_impact?: number;
  residual_level?: number;
  residual_rating?: RiskRating;
  status: 'open' | 'in_treatment' | 'accepted' | 'closed';
  review_date?: string;
}

export type RiskInput = Omit<
  Risk,
  'id' | 'code' | 'risk_level' | 'risk_rating' | 'residual_level' | 'residual_rating'
> & { code?: string };

export const risksApi = {
  list: () => request<ListResponse<Risk>>('/api/v1/risks'),
  create: (risk: RiskInput) => request<Risk>('/api/v1/risks', 'POST', risk),
  update: (id: string, risk: RiskInput) => request<Risk>(`/api/v1/risks/${id}`, 'PUT', risk),
  remove: (id: string) => request<void>(`/api/v1/risks/${id}`, 'DELETE'),
};

/** Same bands as the backend's RiskRating (5×5 matrix). */
export function ratingFor(level: number): RiskRating {
  if (level >= 16) return 'critical';
  if (level >= 10) return 'high';
  if (level >= 5) return 'medium';
  return 'low';
}
