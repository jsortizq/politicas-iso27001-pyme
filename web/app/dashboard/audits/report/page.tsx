'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  ApiError,
  auditsApi,
  errorMessage,
  findingStatusLabels,
  loadCatalog,
  severityLabels,
  type FindingSeverity,
  type InternalAudit,
} from '@/lib/api';
import { formatDay, formatInstant } from '@/lib/format';
import { useSession } from '@/lib/session';
import { ErrorBanner, Spinner } from '@/components/ui/feedback';

const typeLabels = { internal: 'Interna', external: 'Externa', surveillance: 'De seguimiento' } as const;
const severities: FindingSeverity[] = ['major_nc', 'minor_nc', 'observation', 'opportunity'];

/**
 * Audit report laid out as a document: readable on screen and clean on
 * paper (the navigation is hidden when printing).
 */
export default function AuditReportPage() {
  const { organizationName } = useSession();
  const [audit, setAudit] = useState<InternalAudit | null>(null);
  const [titles, setTitles] = useState<Map<string, string>>(new Map());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get('id');
    if (!id) {
      setError('Falta el identificador de la auditoría.');
      return;
    }
    auditsApi
      .get(id)
      .then(setAudit)
      .catch((err) => {
        if (err instanceof ApiError && err.status === 401) window.location.assign('/login');
        else if (err instanceof ApiError && err.status === 404) setError('La auditoría no existe.');
        else setError(errorMessage(err));
      });
    loadCatalog()
      .then((controls) => setTitles(new Map(controls.map((c) => [c.id, c.clause_es]))))
      .catch(() => undefined); // titles are a nicety; ids are still shown
  }, []);

  if (error) return <ErrorBanner message={error} />;
  if (!audit) return <Spinner />;

  const count = (severity: FindingSeverity) => audit.findings.filter((f) => f.severity === severity).length;

  return (
    <article className="mx-auto max-w-3xl space-y-8 print:max-w-none">
      <div className="flex flex-wrap items-center justify-between gap-3 print:hidden">
        <Link href="/dashboard/audits" className="text-sm underline underline-offset-4">
          ← Volver a auditorías
        </Link>
        <button className="btn btn-primary" onClick={() => window.print()}>
          Imprimir o guardar como PDF
        </button>
      </div>

      {audit.status !== 'completed' && (
        <p className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
          Borrador: la auditoría aún no está completada, así que los hallazgos pueden cambiar.
        </p>
      )}

      <header className="space-y-1 border-b pb-6">
        <p className="text-sm text-muted-foreground">{organizationName} · Informe de auditoría</p>
        <h1 className="text-2xl font-bold tracking-tight">{audit.title}</h1>
      </header>

      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[10rem_1fr]">
        <dt className="text-muted-foreground">Tipo</dt>
        <dd>{typeLabels[audit.audit_type]}</dd>
        <dt className="text-muted-foreground">Auditor líder</dt>
        <dd>{audit.lead_auditor || '—'}</dd>
        <dt className="text-muted-foreground">Fecha prevista</dt>
        <dd>{formatDay(audit.planned_date)}</dd>
        <dt className="text-muted-foreground">Fecha de cierre</dt>
        <dd>{formatDay(audit.completed_date)}</dd>
        <dt className="text-muted-foreground">Alcance</dt>
        <dd className="whitespace-pre-wrap">{audit.scope || '—'}</dd>
        <dt className="text-muted-foreground">Criterios</dt>
        <dd className="whitespace-pre-wrap">{audit.criteria || '—'}</dd>
      </dl>

      {audit.controls.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-lg font-semibold">Controles auditados</h2>
          <ul className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-2">
            {audit.controls.map((id) => (
              <li key={id}>
                <span className="font-mono text-xs">{id}</span> {titles.get(id)}
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="space-y-2">
        <h2 className="text-lg font-semibold">Conclusiones</h2>
        <p className="whitespace-pre-wrap text-sm">{audit.summary || 'Pendientes: se redactan al completar la auditoría.'}</p>
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Resumen de hallazgos</h2>
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {severities.map((severity) => (
            <div key={severity} className="rounded-md border p-3">
              <dd className="text-2xl font-bold tabular-nums">{count(severity)}</dd>
              <dt className="text-xs text-muted-foreground">{severityLabels[severity]}</dt>
            </div>
          ))}
        </dl>
        <p className="text-sm text-muted-foreground">
          {audit.findings.length === 0
            ? 'No se registraron hallazgos.'
            : `${audit.open_findings} de ${audit.findings.length} sin cerrar` +
              (audit.overdue_findings > 0 ? `, ${audit.overdue_findings} con fecha límite vencida.` : '.')}
        </p>
      </section>

      {audit.findings.length > 0 && (
        <section className="space-y-4">
          <h2 className="text-lg font-semibold">Detalle de hallazgos</h2>
          {audit.findings.map((f) => (
            <section key={f.id} className="break-inside-avoid space-y-2 rounded-md border p-4 text-sm">
              <h3 className="font-semibold">
                H-{f.number} · {severityLabels[f.severity]}
              </h3>
              <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-[9rem_1fr]">
                {(f.control_id || f.clause) && (
                  <>
                    <dt className="text-muted-foreground">Requisito</dt>
                    <dd>
                      {f.control_id && (
                        <>
                          {f.control_id} {titles.get(f.control_id)}
                        </>
                      )}
                      {f.control_id && f.clause && ' · '}
                      {f.clause && `Cláusula ${f.clause}`}
                    </dd>
                  </>
                )}
                <dt className="text-muted-foreground">Hallazgo</dt>
                <dd className="whitespace-pre-wrap">{f.description}</dd>
                <dt className="text-muted-foreground">Evidencia</dt>
                <dd className="whitespace-pre-wrap">{f.evidence || '—'}</dd>
                <dt className="text-muted-foreground">Causa raíz</dt>
                <dd className="whitespace-pre-wrap">{f.root_cause || '—'}</dd>
                <dt className="text-muted-foreground">Acción correctiva</dt>
                <dd className="whitespace-pre-wrap">{f.corrective_action || '—'}</dd>
                <dt className="text-muted-foreground">Responsable</dt>
                <dd>{f.responsible || '—'}</dd>
                <dt className="text-muted-foreground">Fecha límite</dt>
                <dd>
                  {formatDay(f.due_date)}
                  {f.overdue && ' (vencida)'}
                </dd>
                <dt className="text-muted-foreground">Estado</dt>
                <dd>
                  {findingStatusLabels[f.status]}
                  {f.status === 'closed' && ` por ${f.closed_by} el ${formatInstant(f.closed_at)}`}
                </dd>
                {f.status === 'closed' && (
                  <>
                    <dt className="text-muted-foreground">Verificación</dt>
                    <dd className="whitespace-pre-wrap">{f.verification}</dd>
                  </>
                )}
              </dl>
            </section>
          ))}
        </section>
      )}

      <footer className="border-t pt-4 text-xs text-muted-foreground">
        Generado el {formatInstant(new Date().toISOString())}. El estado de los hallazgos corresponde a ese momento.
      </footer>
    </article>
  );
}
