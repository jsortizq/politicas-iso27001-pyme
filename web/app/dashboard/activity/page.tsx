'use client';

import { useCallback, useEffect, useState } from 'react';
import { ApiError, auditApi, errorMessage, roleLabels, type AuditEvent, type Role } from '@/lib/api';
import { useSession } from '@/lib/session';
import { EmptyState, ErrorBanner, PageHeader, Spinner } from '@/components/ui/feedback';

const entityLabels: Record<string, string> = {
  soa: 'control',
  asset: 'activo',
  risk: 'riesgo',
};

const verbLabels: Record<string, string> = {
  create: 'Creó',
  update: 'Modificó',
  delete: 'Eliminó',
};

const text = (value: unknown) => (typeof value === 'string' || typeof value === 'number' ? String(value) : '');
const roleName = (value: unknown) => roleLabels[text(value) as Role] ?? text(value);

/** One readable sentence per event, from the action and its detail. */
function describe(event: AuditEvent): string {
  const d = event.detail ?? {};
  const before = (d.before ?? {}) as Record<string, unknown>;
  const after = (d.after ?? {}) as Record<string, unknown>;

  switch (event.action) {
    case 'organization.create':
      return `Creó la organización «${text(d.name)}»`;
    case 'soa.initialize':
      return `Cargó ${text(d.controls)} controles del Anexo A en la Declaración de Aplicabilidad`;
    case 'document.create':
      return `Creó el documento «${text(d.title)}» (${text(d.file_name)})`;
    case 'document.update':
      return `Modificó los datos del documento «${text(d.title)}»`;
    case 'document.version_add':
      return `Subió la versión ${text(d.version)} de «${text(d.title)}»`;
    case 'document.submit':
      return `Envió a revisión la versión ${text(d.version)} de «${text(d.title)}»`;
    case 'document.approve':
      return `Aprobó la versión ${text(d.version)} de «${text(d.title)}»`;
    case 'document.reject':
      return `Rechazó la versión ${text(d.version)} de «${text(d.title)}»${d.note ? `: ${text(d.note)}` : ''}`;
    case 'document.obsolete':
      return `Marcó como obsoleto el documento «${text(d.title)}»`;
    case 'document.delete':
      return `Eliminó el documento «${text(d.title)}»`;
    case 'evidence.create':
      return `Subió la evidencia «${text(d.title)}» (${text(d.file_name)})`;
    case 'evidence.update':
      return `Modificó la evidencia «${text(d.title)}»`;
    case 'evidence.delete':
      return `Eliminó la evidencia «${text(d.title)}» (SHA-256 ${text(d.sha256).slice(0, 12)}…)`;
    case 'audit.create':
      return `Programó la auditoría «${text(d.title)}»`;
    case 'audit.update':
      return `Modificó los datos de la auditoría «${text(d.title)}»`;
    case 'audit.start':
      return `Inició la auditoría «${text(d.title)}»`;
    case 'audit.complete':
      return `Completó la auditoría «${text(d.title)}» con ${text(d.findings)} hallazgos`;
    case 'audit.cancel':
      return `Canceló la auditoría «${text(d.title)}»: ${text(d.reason)}`;
    case 'audit.delete':
      return `Eliminó la auditoría planificada «${text(d.title)}»`;
    case 'finding.create':
      return `Registró el hallazgo H-${text(d.finding)} en «${text(d.audit)}»`;
    case 'finding.update':
      return `Modificó el hallazgo H-${text(d.finding)} de «${text(d.audit)}»`;
    case 'finding.delete':
      return `Quitó el hallazgo H-${text(d.finding)} de «${text(d.audit)}»`;
    case 'finding.plan':
      return `Actualizó el plan de acción del hallazgo H-${text(d.finding)} de «${text(d.audit)}»`;
    case 'finding.close':
      return `Cerró el hallazgo H-${text(d.finding)} de «${text(d.audit)}»: ${text(d.verification)}`;
    case 'finding.reopen':
      return `Reabrió el hallazgo H-${text(d.finding)} de «${text(d.audit)}»: ${text(d.note)}`;
    case 'member.invite':
      return `Invitó a ${text(d.email)} como ${roleName(d.member_role)}`;
    case 'member.join':
      return 'Aceptó la invitación y entró en la organización';
    case 'member.role_change':
      return `Cambió el rol de ${text(d.email)}: ${roleName(d.before)} → ${roleName(d.after)}`;
    case 'member.remove':
      return `Quitó a ${text(d.email)} de la organización`;
    case 'soa.update': {
      const changes: string[] = [];
      if (before.applicable !== after.applicable) {
        changes.push(after.applicable ? 'ahora aplica' : 'ya no aplica');
      }
      if (before.implementation_pct !== after.implementation_pct) {
        changes.push(`implementación ${text(before.implementation_pct)}% → ${text(after.implementation_pct)}%`);
      }
      if (before.effectiveness_pct !== after.effectiveness_pct) {
        changes.push(`efectividad ${text(before.effectiveness_pct)}% → ${text(after.effectiveness_pct)}%`);
      }
      return `Evaluó el control ${text(d.control_id)}${changes.length ? `: ${changes.join(', ')}` : ''}`;
    }
    case 'risk.update': {
      const changes: string[] = [];
      if (before.risk_level !== after.risk_level) {
        changes.push(`nivel ${text(before.risk_level)} → ${text(after.risk_level)}`);
      }
      if (before.status !== after.status) changes.push(`estado ${text(before.status)} → ${text(after.status)}`);
      if (before.treatment !== after.treatment) {
        changes.push(`tratamiento ${text(before.treatment)} → ${text(after.treatment)}`);
      }
      return `Modificó el riesgo ${text(d.code)}${changes.length ? `: ${changes.join(', ')}` : ''}`;
    }
  }

  const [entity, verb] = event.action.split('.');
  const label = text(d.control_id) || text(d.code) || text(d.name);
  if (entityLabels[entity] && verbLabels[verb]) {
    return `${verbLabels[verb]} el ${entityLabels[entity]}${label ? ` ${label}` : ''}`;
  }
  return event.action;
}

const dateFormat = new Intl.DateTimeFormat('es', { dateStyle: 'medium', timeStyle: 'short' });

export default function ActivityPage() {
  const { canViewAudit } = useSession();
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [next, setNext] = useState<number | undefined>();
  const [recorded, setRecorded] = useState(true);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (before?: number) => {
    setLoading(true);
    try {
      const page = await auditApi.list(before);
      setEvents((current) => (before ? [...current, ...page.data] : page.data));
      setNext(page.next_before);
      setRecorded(page.recorded);
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) window.location.assign('/login');
      else setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (canViewAudit) load();
  }, [canViewAudit, load]);

  if (!canViewAudit) {
    return <ErrorBanner message="Tu rol no permite ver el registro de actividad." />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Registro de actividad"
        subtitle="Quién cambió qué y cuándo. Los registros no se pueden editar ni borrar."
      />

      {error && <ErrorBanner message={error} onRetry={() => load()} />}

      {!recorded ? (
        <EmptyState title="El registro de actividad no está activo">
          El servidor está funcionando sin base de datos, así que los cambios no se registran.
          Configura PostgreSQL (DATABASE_URL) para conservar la trazabilidad.
        </EmptyState>
      ) : events.length === 0 && !loading && !error ? (
        <EmptyState title="Aún no hay actividad">
          Aquí aparecerán los cambios en controles, activos, riesgos y miembros.
        </EmptyState>
      ) : (
        events.length > 0 && (
          <ol className="divide-y rounded-lg border">
            {events.map((event) => (
              <li key={event.id} className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-baseline sm:gap-4">
                <time
                  dateTime={event.created_at}
                  className="shrink-0 text-xs tabular-nums text-muted-foreground sm:w-40"
                >
                  {dateFormat.format(new Date(event.created_at))}
                </time>
                <div className="min-w-0 text-sm">
                  <p className="break-words">{describe(event)}</p>
                  <p className="truncate text-xs text-muted-foreground">{event.actor || 'sistema'}</p>
                </div>
              </li>
            ))}
          </ol>
        )
      )}

      {loading && <Spinner />}
      {next && !loading && (
        <button className="btn btn-outline" onClick={() => load(next)}>
          Cargar más antiguos
        </button>
      )}
    </div>
  );
}
