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
  type AuditFields,
  type AuditType,
  type Finding,
  type FindingContent,
  type FindingPlan,
  type FindingSeverity,
  type FindingStatus,
  type InternalAudit,
  type InternalAuditStatus,
} from '@/lib/api';
import { formatDay, formatInstant, todayLocal } from '@/lib/format';
import { useSession } from '@/lib/session';
import { useResource } from '@/lib/useResource';
import { ControlPicker } from '@/components/ControlPicker';
import { EmptyState, ErrorBanner, Field, NoteForm, PageHeader, Panel, Spinner } from '@/components/ui/feedback';

const typeLabels: Record<AuditType, string> = {
  internal: 'Interna',
  external: 'Externa',
  surveillance: 'De seguimiento',
};

const statusLabels: Record<InternalAuditStatus, string> = {
  planned: 'Planificada',
  in_progress: 'En curso',
  completed: 'Completada',
  cancelled: 'Cancelada',
};

const statusBadge: Record<InternalAuditStatus, string> = {
  planned: 'badge-na',
  in_progress: 'badge-partial',
  completed: 'badge-full',
  cancelled: 'badge-none',
};

const severityBadge: Record<FindingSeverity, string> = {
  major_nc: 'badge-critical',
  minor_nc: 'badge-none',
  observation: 'badge-partial',
  opportunity: 'badge-na',
};

const findingBadge: Record<FindingStatus, string> = {
  open: 'badge-none',
  in_progress: 'badge-partial',
  implemented: 'badge-partial',
  closed: 'badge-full',
};

/** A 409 means the audit or finding is no longer in the state the screen showed. */
function auditError(err: unknown): string {
  if (err instanceof ApiError && err.status === 409) {
    return 'La acción ya no aplica al estado actual. Revisa la auditoría e inténtalo de nuevo.';
  }
  if (err instanceof ApiError && err.status === 422 && err.message.includes('nonconformity')) {
    return 'Una no conformidad necesita su causa raíz y su acción correctiva antes de cerrarse.';
  }
  if (err instanceof ApiError && err.status === 422 && err.message.includes('Annex A catalog')) {
    return 'Ese control no existe en el Anexo A. Usa un identificador como A.5.1.';
  }
  if (err instanceof ApiError && err.status === 422 && err.message.includes('implemented')) {
    return 'Describe la acción realizada antes de marcarla como implementada.';
  }
  return errorMessage(err);
}

const emptyFields: AuditFields = {
  title: '',
  audit_type: 'internal',
  scope: '',
  criteria: 'ISO/IEC 27001:2022 y políticas internas del SGSI',
  lead_auditor: '',
  planned_date: '',
  controls: [],
};

function fieldsOf(a: InternalAudit): AuditFields {
  return {
    title: a.title,
    audit_type: a.audit_type,
    scope: a.scope,
    criteria: a.criteria,
    lead_auditor: a.lead_auditor,
    planned_date: a.planned_date ?? '',
    controls: a.controls,
  };
}

function AuditFieldsForm({
  fields,
  onChange,
  disabled,
}: {
  fields: AuditFields;
  onChange: (fields: AuditFields) => void;
  disabled: boolean;
}) {
  const set = <K extends keyof AuditFields>(key: K, value: AuditFields[K]) => onChange({ ...fields, [key]: value });
  return (
    <>
      <Field label="Título">
        <input className="input" required maxLength={300} disabled={disabled} value={fields.title} onChange={(e) => set('title', e.target.value)} placeholder="p. ej. Auditoría interna 2026" />
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Tipo">
          <select className="input" disabled={disabled} value={fields.audit_type} onChange={(e) => set('audit_type', e.target.value as AuditType)}>
            {Object.entries(typeLabels).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Fecha prevista">
          <input type="date" className="input" disabled={disabled} value={fields.planned_date} onChange={(e) => set('planned_date', e.target.value)} />
        </Field>
      </div>
      <Field label="Auditor líder" hint="Quien audita no debería auditar su propio trabajo.">
        <input className="input" maxLength={200} disabled={disabled} value={fields.lead_auditor} onChange={(e) => set('lead_auditor', e.target.value)} />
      </Field>
      <Field label="Alcance" hint="Qué procesos, áreas o sedes se revisan.">
        <textarea className="input" rows={2} maxLength={4000} disabled={disabled} value={fields.scope} onChange={(e) => set('scope', e.target.value)} />
      </Field>
      <Field label="Criterios" hint="Contra qué se compara: la norma, políticas, requisitos legales.">
        <textarea className="input" rows={2} maxLength={4000} disabled={disabled} value={fields.criteria} onChange={(e) => set('criteria', e.target.value)} />
      </Field>
      <div className="space-y-1.5">
        <span className="text-sm font-medium">Controles a auditar</span>
        <ControlPicker value={fields.controls} onChange={(controls) => set('controls', controls)} disabled={disabled} />
      </div>
    </>
  );
}

const emptyContent: FindingContent = { severity: 'minor_nc', control_id: '', clause: '', description: '', evidence: '' };

function FindingContentForm({
  initial,
  submitLabel,
  busy,
  onSubmit,
  onCancel,
}: {
  initial: FindingContent;
  submitLabel: string;
  busy: boolean;
  onSubmit: (content: FindingContent) => void;
  onCancel?: () => void;
}) {
  const [content, setContent] = useState(initial);
  const [controlIds, setControlIds] = useState<string[]>([]);
  useEffect(() => {
    let cancelled = false;
    loadCatalog()
      .then((controls) => !cancelled && setControlIds(controls.map((c) => c.id)))
      .catch(() => undefined); // suggestions are optional; the server validates
    return () => {
      cancelled = true;
    };
  }, []);
  const set = <K extends keyof FindingContent>(key: K, value: FindingContent[K]) =>
    setContent((current) => ({ ...current, [key]: value }));

  return (
    <form
      className="space-y-4"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(content);
      }}
    >
      <Field label="Tipo de hallazgo">
        <select className="input" disabled={busy} value={content.severity} onChange={(e) => set('severity', e.target.value as FindingSeverity)}>
          {Object.entries(severityLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Control del Anexo A" hint="Opcional. Ej.: A.5.1">
          <input className="input" list="annex-a-controls" disabled={busy} value={content.control_id} onChange={(e) => set('control_id', e.target.value.trim())} placeholder="A.5.1" />
          <datalist id="annex-a-controls">
            {controlIds.map((id) => (
              <option key={id} value={id} />
            ))}
          </datalist>
        </Field>
        <Field label="Cláusula" hint="Opcional. Ej.: 7.5.3">
          <input className="input" maxLength={100} disabled={busy} value={content.clause} onChange={(e) => set('clause', e.target.value)} />
        </Field>
      </div>
      <Field label="Qué se encontró">
        <textarea className="input" rows={3} required maxLength={4000} disabled={busy} value={content.description} onChange={(e) => set('description', e.target.value)} />
      </Field>
      <Field label="Evidencia objetiva" hint="Qué se vio, dónde y cuándo.">
        <textarea className="input" rows={2} maxLength={4000} disabled={busy} value={content.evidence} onChange={(e) => set('evidence', e.target.value)} />
      </Field>
      <div className="flex gap-2">
        <button type="submit" className="btn btn-primary h-9 px-3" disabled={busy || !content.description.trim()}>
          {submitLabel}
        </button>
        {onCancel && (
          <button type="button" className="btn btn-outline h-9 px-3" onClick={onCancel}>
            Volver
          </button>
        )}
      </div>
    </form>
  );
}

function FindingCard({
  audit,
  finding,
  busy,
  run,
}: {
  audit: InternalAudit;
  finding: Finding;
  busy: boolean;
  run: (action: () => Promise<InternalAudit>) => Promise<boolean>;
}) {
  const { canWrite, canManageMembers } = useSession();
  const [mode, setMode] = useState<'view' | 'edit' | 'remove' | 'close' | 'reopen'>('view');
  const [plan, setPlan] = useState<FindingPlan>({
    root_cause: finding.root_cause,
    corrective_action: finding.corrective_action,
    responsible: finding.responsible,
    due_date: finding.due_date ?? '',
    status: finding.status === 'closed' ? 'implemented' : finding.status,
  });

  const auditing = audit.status === 'in_progress';
  const closed = finding.status === 'closed';
  const followUp = audit.status !== 'cancelled' && !closed;
  const setP = <K extends keyof FindingPlan>(key: K, value: FindingPlan[K]) =>
    setPlan((current) => ({ ...current, [key]: value }));
  const then = (ok: boolean) => ok && setMode('view');

  const reference = [finding.control_id, finding.clause && `Cláusula ${finding.clause}`].filter(Boolean).join(' · ');

  return (
    <li className="space-y-4 rounded-md border p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-sm font-semibold">H-{finding.number}</span>
        <span className={`badge ${severityBadge[finding.severity]}`}>{severityLabels[finding.severity]}</span>
        <span className={`badge ${findingBadge[finding.status]}`}>{findingStatusLabels[finding.status]}</span>
        {finding.overdue && <span className="badge badge-critical">Vencido</span>}
      </div>

      {mode === 'edit' ? (
        <FindingContentForm
          initial={{
            severity: finding.severity,
            control_id: finding.control_id ?? '',
            clause: finding.clause,
            description: finding.description,
            evidence: finding.evidence,
          }}
          submitLabel="Guardar hallazgo"
          busy={busy}
          onSubmit={(content) => run(() => auditsApi.updateFinding(audit.id, finding.id, content)).then(then)}
          onCancel={() => setMode('view')}
        />
      ) : (
        <div className="space-y-1 text-sm">
          {reference && <p className="text-xs text-muted-foreground">{reference}</p>}
          <p className="whitespace-pre-wrap">{finding.description}</p>
          {finding.evidence && (
            <p className="whitespace-pre-wrap text-muted-foreground">Evidencia: {finding.evidence}</p>
          )}
          {auditing && canWrite && mode === 'view' && (
            <div className="flex gap-2 pt-1">
              <button className="btn btn-outline h-8 px-3" onClick={() => setMode('edit')}>
                Editar
              </button>
              <button className="btn btn-outline h-8 px-3" onClick={() => setMode('remove')}>
                Quitar
              </button>
            </div>
          )}
          {mode === 'remove' && (
            <div className="flex flex-wrap items-center gap-2 pt-1">
              ¿Quitar este hallazgo?
              <button className="btn btn-danger h-8 px-3" disabled={busy} onClick={() => run(() => auditsApi.removeFinding(audit.id, finding.id))}>
                Sí, quitar
              </button>
              <button className="btn btn-outline h-8 px-3" onClick={() => setMode('view')}>
                No
              </button>
            </div>
          )}
        </div>
      )}

      <div className="space-y-3 border-t pt-4">
        <h4 className="text-sm font-semibold">Plan de acción</h4>
        {followUp && canWrite ? (
          <form
            className="space-y-3"
            onSubmit={(event) => {
              event.preventDefault();
              run(() => auditsApi.planFinding(audit.id, finding.id, plan));
            }}
          >
            <Field label="Causa raíz">
              <textarea className="input" rows={2} maxLength={4000} disabled={busy} value={plan.root_cause} onChange={(e) => setP('root_cause', e.target.value)} />
            </Field>
            <Field label="Acción correctiva">
              <textarea className="input" rows={2} maxLength={4000} disabled={busy} value={plan.corrective_action} onChange={(e) => setP('corrective_action', e.target.value)} />
            </Field>
            <div className="grid gap-3 sm:grid-cols-3">
              <Field label="Responsable">
                <input className="input" maxLength={200} disabled={busy} value={plan.responsible} onChange={(e) => setP('responsible', e.target.value)} />
              </Field>
              <Field label="Fecha límite">
                <input type="date" className="input" disabled={busy} value={plan.due_date} onChange={(e) => setP('due_date', e.target.value)} />
              </Field>
              <Field label="Avance">
                <select className="input" disabled={busy} value={plan.status} onChange={(e) => setP('status', e.target.value as FindingPlan['status'])}>
                  <option value="open">Abierto</option>
                  <option value="in_progress">En curso</option>
                  <option value="implemented">Implementado</option>
                </select>
              </Field>
            </div>
            <button type="submit" className="btn btn-outline h-8 px-3" disabled={busy}>
              Guardar plan
            </button>
          </form>
        ) : (
          <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[8rem_1fr]">
            <dt className="text-muted-foreground">Causa raíz</dt>
            <dd className="whitespace-pre-wrap">{finding.root_cause || '—'}</dd>
            <dt className="text-muted-foreground">Acción</dt>
            <dd className="whitespace-pre-wrap">{finding.corrective_action || '—'}</dd>
            <dt className="text-muted-foreground">Responsable</dt>
            <dd>{finding.responsible || '—'}</dd>
            <dt className="text-muted-foreground">Fecha límite</dt>
            <dd>{formatDay(finding.due_date)}</dd>
          </dl>
        )}

        {finding.reopen_note && !closed && (
          <p className="text-sm text-muted-foreground">Reabierto: {finding.reopen_note}</p>
        )}
        {closed && (
          <p className="text-sm">
            <span className="text-muted-foreground">
              Cerrado por {finding.closed_by} el {formatInstant(finding.closed_at)}.
            </span>{' '}
            Verificación: {finding.verification}
          </p>
        )}

        {canManageMembers && audit.status !== 'cancelled' && mode === 'view' && (
          <button className="btn btn-outline h-8 px-3" onClick={() => setMode(closed ? 'reopen' : 'close')}>
            {closed ? 'Reabrir hallazgo' : 'Cerrar hallazgo'}
          </button>
        )}
        {mode === 'close' && (
          <NoteForm
            label="Cómo se verificó el resultado"
            hint="Cerrar significa que alguien comprobó que la acción funcionó."
            confirmLabel="Cerrar hallazgo"
            busy={busy}
            onConfirm={(note) => run(() => auditsApi.closeFinding(audit.id, finding.id, note)).then(then)}
            onCancel={() => setMode('view')}
          />
        )}
        {mode === 'reopen' && (
          <NoteForm
            label="Motivo de la reapertura"
            confirmLabel="Reabrir hallazgo"
            danger
            busy={busy}
            onConfirm={(note) => run(() => auditsApi.reopenFinding(audit.id, finding.id, note)).then(then)}
            onCancel={() => setMode('view')}
          />
        )}
      </div>
    </li>
  );
}

function AuditPanel({
  initial,
  onChanged,
  onDeleted,
}: {
  initial: InternalAudit;
  onChanged: () => void;
  onDeleted: () => void;
}) {
  const { canWrite, canManageMembers } = useSession();
  const [audit, setAudit] = useState(initial);
  const [fields, setFields] = useState<AuditFields>(fieldsOf(initial));
  const [mode, setMode] = useState<'view' | 'complete' | 'cancel' | 'delete' | 'finding'>('view');
  const [summary, setSummary] = useState('');
  const [completedDate, setCompletedDate] = useState(todayLocal());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const editable = canWrite && (audit.status === 'planned' || audit.status === 'in_progress');

  // The list may be minutes old and a colleague may have acted since:
  // open the panel on the current state, not the listed one.
  useEffect(() => {
    let cancelled = false;
    auditsApi
      .get(initial.id)
      .then((fresh) => {
        if (cancelled) return;
        setAudit(fresh);
        setFields(fieldsOf(fresh));
      })
      .catch(() => undefined); // the listed copy stays; actions report real errors
    return () => {
      cancelled = true;
    };
  }, [initial.id]);

  /** Runs an action that returns the updated audit; reports whether it worked. */
  const run = async (action: () => Promise<InternalAudit>): Promise<boolean> => {
    setBusy(true);
    setError(null);
    try {
      const updated = await action();
      setAudit(updated);
      setFields(fieldsOf(updated));
      onChanged();
      return true;
    } catch (err) {
      setError(auditError(err));
      return false;
    } finally {
      setBusy(false);
    }
  };
  const then = (ok: boolean) => ok && setMode('view');

  const remove = async () => {
    setBusy(true);
    setError(null);
    try {
      await auditsApi.remove(audit.id);
      onDeleted();
    } catch (err) {
      setError(auditError(err));
      setBusy(false);
    }
  };

  return (
    <div className="space-y-8">
      {error && <ErrorBanner message={error} />}

      <section className="space-y-3">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className={`badge ${statusBadge[audit.status]}`}>{statusLabels[audit.status]}</span>
          {audit.status === 'completed' && (
            <span className="text-muted-foreground">Completada el {formatDay(audit.completed_date)}</span>
          )}
          {audit.findings.length > 0 && (
            <span className="text-muted-foreground">
              {audit.findings.length} hallazgos · {audit.open_findings} sin cerrar
            </span>
          )}
        </div>

        {audit.status === 'completed' && (
          <div className="space-y-2 rounded-md border p-4 text-sm">
            <p className="font-semibold">Conclusiones</p>
            <p className="whitespace-pre-wrap">{audit.summary}</p>
          </div>
        )}
        {audit.status === 'cancelled' && (
          <p className="rounded-md border p-4 text-sm">Cancelada: {audit.cancel_reason}</p>
        )}

        {mode === 'view' && (
          <div className="flex flex-wrap gap-2">
            {audit.status === 'planned' && canWrite && (
              <button className="btn btn-primary" disabled={busy} onClick={() => run(() => auditsApi.start(audit.id))}>
                Iniciar auditoría
              </button>
            )}
            {audit.status === 'in_progress' && canWrite && (
              <button className="btn btn-primary" onClick={() => setMode('complete')}>
                Completar auditoría
              </button>
            )}
            {audit.status !== 'planned' && audit.status !== 'cancelled' && (
              <Link className="btn btn-outline" href={`/dashboard/audits/report?id=${audit.id}`}>
                Ver informe
              </Link>
            )}
            {canManageMembers && (audit.status === 'planned' || audit.status === 'in_progress') && (
              <button className="btn btn-danger" onClick={() => setMode('cancel')}>
                Cancelar auditoría
              </button>
            )}
            {canManageMembers && audit.status === 'planned' && (
              <button className="btn btn-danger" onClick={() => setMode('delete')}>
                Eliminar
              </button>
            )}
          </div>
        )}

        {mode === 'complete' && (
          <form
            className="space-y-3 rounded-md border p-4"
            onSubmit={(event) => {
              event.preventDefault();
              run(() => auditsApi.complete(audit.id, summary, completedDate)).then(then);
            }}
          >
            <p className="text-sm text-muted-foreground">
              Al completar, los hallazgos quedan fijos: no se podrán añadir, cambiar ni quitar. El
              seguimiento de sus acciones continúa.
            </p>
            <Field label="Conclusiones de la auditoría">
              <textarea className="input" rows={4} required maxLength={8000} autoFocus value={summary} onChange={(e) => setSummary(e.target.value)} />
            </Field>
            <Field label="Fecha de cierre">
              <input type="date" className="input" required max={todayLocal()} value={completedDate} onChange={(e) => setCompletedDate(e.target.value)} />
            </Field>
            <div className="flex gap-2">
              <button type="submit" className="btn btn-primary h-9 px-3" disabled={busy || !summary.trim()}>
                Completar auditoría
              </button>
              <button type="button" className="btn btn-outline h-9 px-3" onClick={() => setMode('view')}>
                Volver
              </button>
            </div>
          </form>
        )}
        {mode === 'cancel' && (
          <NoteForm
            label="Motivo de la cancelación"
            hint="La auditoría queda registrada como cancelada y ya no se podrá modificar."
            confirmLabel="Cancelar auditoría"
            danger
            busy={busy}
            onConfirm={(reason) => run(() => auditsApi.cancel(audit.id, reason)).then(then)}
            onCancel={() => setMode('view')}
          />
        )}
        {mode === 'delete' && (
          <div className="space-y-2 text-sm">
            <p>Se eliminará la auditoría planificada. No se puede deshacer.</p>
            <div className="flex gap-2">
              <button className="btn btn-danger h-8 px-3" disabled={busy} onClick={remove}>
                Sí, eliminar
              </button>
              <button className="btn btn-outline h-8 px-3" onClick={() => setMode('view')}>
                Volver
              </button>
            </div>
          </div>
        )}
      </section>

      <form
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault();
          run(() => auditsApi.update(audit.id, fields));
        }}
      >
        <AuditFieldsForm fields={fields} onChange={setFields} disabled={!editable || busy} />
        {editable && (
          <button type="submit" className="btn btn-outline" disabled={busy}>
            Guardar datos
          </button>
        )}
      </form>

      <section className="space-y-3">
        <h3 className="font-semibold">Hallazgos</h3>
        {audit.findings.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {audit.status === 'planned'
              ? 'Los hallazgos se registran cuando la auditoría está en curso.'
              : 'No se registraron hallazgos.'}
          </p>
        )}
        <ol className="space-y-3">
          {audit.findings.map((f) => (
            <FindingCard key={`${f.id}:${f.status}:${f.updated_at}`} audit={audit} finding={f} busy={busy} run={run} />
          ))}
        </ol>

        {audit.status === 'in_progress' && canWrite && mode !== 'finding' && (
          <button className="btn btn-outline" onClick={() => setMode('finding')}>
            Registrar hallazgo
          </button>
        )}
        {mode === 'finding' && (
          <div className="rounded-md border p-4">
            <h4 className="mb-3 text-sm font-semibold">Nuevo hallazgo</h4>
            <FindingContentForm
              initial={emptyContent}
              submitLabel="Registrar hallazgo"
              busy={busy}
              onSubmit={(content) => run(() => auditsApi.addFinding(audit.id, content)).then(then)}
              onCancel={() => setMode('view')}
            />
          </div>
        )}
      </section>
    </div>
  );
}

function NewAuditForm({ onDone }: { onDone: () => void }) {
  const [fields, setFields] = useState<AuditFields>(emptyFields);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await auditsApi.create(fields);
      onDone();
    } catch (err) {
      setError(auditError(err));
      setBusy(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {error && <ErrorBanner message={error} />}
      <AuditFieldsForm fields={fields} onChange={setFields} disabled={busy} />
      <button type="submit" className="btn btn-primary" disabled={busy}>
        {busy ? 'Guardando…' : 'Programar auditoría'}
      </button>
    </form>
  );
}

export default function AuditsPage() {
  const { canWrite } = useSession();
  const { data, error, loading, reload } = useResource(auditsApi.list);
  const [creating, setCreating] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);

  if (loading) return <Spinner />;
  if (error) return <ErrorBanner message={error} onRetry={reload} />;

  const audits = data?.data ?? [];
  const opened = audits.find((a) => a.id === openId) ?? null;
  const addButton = canWrite ? (
    <button className="btn btn-primary" onClick={() => setCreating(true)}>
      Programar auditoría
    </button>
  ) : undefined;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Auditorías"
        subtitle="Auditorías internas del SGSI y seguimiento de sus hallazgos hasta el cierre"
        action={audits.length > 0 ? addButton : undefined}
      />

      {audits.length === 0 ? (
        <EmptyState title="Aún no hay auditorías" action={addButton}>
          La norma pide auditar el SGSI a intervalos planificados. Programa la primera, registra lo
          que encuentres y sigue cada hallazgo hasta comprobar que se corrigió.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left">
                <th className="px-4 py-3 font-medium">Auditoría</th>
                <th className="px-4 py-3 font-medium">Estado</th>
                <th className="hidden px-4 py-3 font-medium sm:table-cell">Fecha</th>
                <th className="hidden px-4 py-3 font-medium md:table-cell">Hallazgos</th>
                <th className="px-4 py-3">
                  <span className="sr-only">Acciones</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {audits.map((a) => (
                <tr key={a.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3">
                    <span className="block max-w-md font-medium">{a.title}</span>
                    <span className="text-xs text-muted-foreground">
                      {typeLabels[a.audit_type]}
                      {a.lead_auditor && ` · ${a.lead_auditor}`}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`badge ${statusBadge[a.status]}`}>{statusLabels[a.status]}</span>
                    {a.overdue_findings > 0 && (
                      <span className="badge badge-critical ml-1 md:hidden">{a.overdue_findings} vencidos</span>
                    )}
                  </td>
                  <td className="hidden px-4 py-3 sm:table-cell">
                    {formatDay(a.status === 'completed' ? a.completed_date : a.planned_date)}
                  </td>
                  <td className="hidden px-4 py-3 md:table-cell">
                    {a.findings.length === 0 ? (
                      <span className="text-muted-foreground">—</span>
                    ) : (
                      <>
                        {a.findings.length} · {a.open_findings} sin cerrar
                        {a.overdue_findings > 0 && (
                          <span className="badge badge-critical ml-2">{a.overdue_findings} vencidos</span>
                        )}
                      </>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button className="btn btn-outline h-8 px-3" onClick={() => setOpenId(a.id)}>
                      Abrir
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {creating && (
        <Panel title="Programar auditoría" onClose={() => setCreating(false)}>
          <NewAuditForm
            onDone={() => {
              setCreating(false);
              reload();
            }}
          />
        </Panel>
      )}
      {opened && (
        <Panel title={opened.title} onClose={() => setOpenId(null)}>
          <AuditPanel
            key={opened.id}
            initial={opened}
            onChanged={reload}
            onDeleted={() => {
              setOpenId(null);
              reload();
            }}
          />
        </Panel>
      )}
    </div>
  );
}
