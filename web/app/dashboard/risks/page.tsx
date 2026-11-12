'use client';

import { useState } from 'react';
import { errorMessage, ratingFor, risksApi, type Risk, type RiskInput, type RiskRating } from '@/lib/api';
import { useSession } from '@/lib/session';
import { useResource } from '@/lib/useResource';
import { EmptyState, ErrorBanner, Field, PageHeader, Panel, Spinner } from '@/components/ui/feedback';

const ratingLabels: Record<RiskRating, string> = {
  low: 'Bajo',
  medium: 'Medio',
  high: 'Alto',
  critical: 'Crítico',
};

const ratingBadge: Record<RiskRating, string> = {
  low: 'badge-full',
  medium: 'badge-partial',
  high: 'badge-none',
  critical: 'badge-critical',
};

const ratingCell: Record<RiskRating, string> = {
  low: 'bg-green-500/25',
  medium: 'bg-amber-400/40',
  high: 'bg-orange-500/50',
  critical: 'bg-red-600/60',
};

const treatmentLabels: Record<Risk['treatment'], string> = {
  mitigate: 'Mitigar',
  accept: 'Aceptar',
  transfer: 'Transferir',
  avoid: 'Evitar',
};

const statusLabels: Record<Risk['status'], string> = {
  open: 'Abierto',
  in_treatment: 'En tratamiento',
  accepted: 'Aceptado',
  closed: 'Cerrado',
};

const scale = [1, 2, 3, 4, 5];
const likelihoodLabels = ['', 'Raro', 'Improbable', 'Posible', 'Probable', 'Casi seguro'];
const impactLabels = ['', 'Insignificante', 'Menor', 'Moderado', 'Mayor', 'Grave'];

function RatingBadge({ level }: { level: number }) {
  const rating = ratingFor(level);
  return (
    <span className={`badge ${ratingBadge[rating]}`}>
      {level} · {ratingLabels[rating]}
    </span>
  );
}

/** 5×5 heat map: likelihood on the vertical axis, impact on the horizontal. */
function RiskMatrix({ risks }: { risks: Risk[] }) {
  const count = (likelihood: number, impact: number) =>
    risks.filter((r) => r.likelihood === likelihood && r.impact === impact).length;

  return (
    <section className="rounded-lg border p-4 sm:p-6">
      <h2 className="font-semibold">Mapa de riesgo inherente</h2>
      <p className="mb-4 text-xs text-muted-foreground">
        Número de riesgos abiertos por probabilidad e impacto.
      </p>
      <div className="flex max-w-md items-stretch gap-2">
        <span className="rotate-180 self-center text-xs text-muted-foreground [writing-mode:vertical-rl]">
          Probabilidad
        </span>
        <div className="flex-1">
          <div className="grid grid-cols-[1.25rem_repeat(5,minmax(0,1fr))] gap-1">
            {[...scale].reverse().map((likelihood) => (
              <div key={likelihood} className="contents">
                <span className="self-center text-center text-xs text-muted-foreground">{likelihood}</span>
                {scale.map((impact) => {
                  const n = count(likelihood, impact);
                  const level = likelihood * impact;
                  return (
                    <div
                      key={impact}
                      title={`Probabilidad ${likelihood} × impacto ${impact} = ${level} (${ratingLabels[ratingFor(level)]})`}
                      className={`flex aspect-square items-center justify-center rounded text-sm font-semibold tabular-nums ${ratingCell[ratingFor(level)]}`}
                    >
                      {n > 0 ? n : ''}
                    </div>
                  );
                })}
              </div>
            ))}
            <span />
            {scale.map((impact) => (
              <span key={impact} className="text-center text-xs text-muted-foreground">
                {impact}
              </span>
            ))}
          </div>
          <p className="mt-1 text-center text-xs text-muted-foreground">Impacto</p>
        </div>
      </div>
    </section>
  );
}

type FormState = Omit<RiskInput, 'residual_likelihood' | 'residual_impact'> & {
  residual_likelihood: number | '';
  residual_impact: number | '';
};

const emptyRisk: FormState = {
  title: '',
  description: '',
  asset: '',
  threat: '',
  vulnerability: '',
  likelihood: 3,
  impact: 3,
  treatment: 'mitigate',
  treatment_plan: '',
  residual_likelihood: '',
  residual_impact: '',
  status: 'open',
  review_date: '',
};

function RiskForm({ risk, onDone }: { risk: Risk | null; onDone: () => void }) {
  const [form, setForm] = useState<FormState>(
    risk
      ? {
          ...risk,
          residual_likelihood: risk.residual_likelihood ?? '',
          residual_impact: risk.residual_impact ?? '',
          review_date: risk.review_date ?? '',
        }
      : emptyRisk,
  );
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((current) => ({ ...current, [key]: value }));

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await action();
      onDone();
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  };

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    if ((form.residual_likelihood === '') !== (form.residual_impact === '')) {
      setError('Indica la probabilidad y el impacto residuales, o deja ambos vacíos.');
      return;
    }
    const input: RiskInput = {
      ...form,
      residual_likelihood: form.residual_likelihood === '' ? undefined : form.residual_likelihood,
      residual_impact: form.residual_impact === '' ? undefined : form.residual_impact,
      review_date: form.review_date || undefined,
    };
    run(() => (risk ? risksApi.update(risk.id, input) : risksApi.create(input)));
  };

  const level = form.likelihood * form.impact;
  const residual =
    form.residual_likelihood !== '' && form.residual_impact !== ''
      ? form.residual_likelihood * form.residual_impact
      : null;

  const scaleSelect = (
    key: 'likelihood' | 'impact' | 'residual_likelihood' | 'residual_impact',
    labels: string[],
    optional: boolean,
  ) => (
    <select
      className="input"
      value={form[key]}
      onChange={(e) => set(key, e.target.value === '' ? '' : Number(e.target.value))}
    >
      {optional && <option value="">Sin evaluar</option>}
      {scale.map((n) => (
        <option key={n} value={n}>
          {n} · {labels[n]}
        </option>
      ))}
    </select>
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {error && <ErrorBanner message={error} />}
      <fieldset disabled={busy} className="space-y-5">
        <Field label="Riesgo">
          <input
            className="input"
            required
            maxLength={300}
            value={form.title}
            onChange={(e) => set('title', e.target.value)}
            placeholder="p. ej. Secuestro de datos por ransomware"
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Activo afectado">
            <input className="input" value={form.asset} onChange={(e) => set('asset', e.target.value)} />
          </Field>
          <Field label="Amenaza">
            <input className="input" value={form.threat} onChange={(e) => set('threat', e.target.value)} />
          </Field>
        </div>
        <Field label="Vulnerabilidad">
          <input className="input" value={form.vulnerability} onChange={(e) => set('vulnerability', e.target.value)} />
        </Field>

        <div className="space-y-3 rounded-md border p-4">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">Riesgo inherente</p>
            <RatingBadge level={level} />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Probabilidad">{scaleSelect('likelihood', likelihoodLabels, false)}</Field>
            <Field label="Impacto">{scaleSelect('impact', impactLabels, false)}</Field>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Tratamiento">
            <select className="input" value={form.treatment} onChange={(e) => set('treatment', e.target.value as Risk['treatment'])}>
              {Object.entries(treatmentLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Estado">
            <select className="input" value={form.status} onChange={(e) => set('status', e.target.value as Risk['status'])}>
              {Object.entries(statusLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <Field label="Plan de tratamiento">
          <textarea className="input" rows={3} value={form.treatment_plan} onChange={(e) => set('treatment_plan', e.target.value)} />
        </Field>

        <div className="space-y-3 rounded-md border p-4">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">Riesgo residual</p>
            {residual !== null && <RatingBadge level={residual} />}
          </div>
          <p className="text-xs text-muted-foreground">Lo que queda después de aplicar el tratamiento.</p>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Probabilidad">{scaleSelect('residual_likelihood', likelihoodLabels, true)}</Field>
            <Field label="Impacto">{scaleSelect('residual_impact', impactLabels, true)}</Field>
          </div>
        </div>

        <Field label="Próxima revisión">
          <input type="date" className="input" value={form.review_date} onChange={(e) => set('review_date', e.target.value)} />
        </Field>
      </fieldset>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {busy ? 'Guardando…' : risk ? 'Guardar cambios' : 'Registrar riesgo'}
        </button>
        {risk &&
          (confirmDelete ? (
            <span className="flex items-center gap-2 text-sm">
              ¿Eliminar definitivamente?
              <button type="button" className="btn btn-danger h-8 px-3" disabled={busy} onClick={() => run(() => risksApi.remove(risk.id))}>
                Sí, eliminar
              </button>
              <button type="button" className="btn btn-outline h-8 px-3" onClick={() => setConfirmDelete(false)}>
                No
              </button>
            </span>
          ) : (
            <button type="button" className="btn btn-danger" onClick={() => setConfirmDelete(true)}>
              Eliminar
            </button>
          ))}
      </div>
    </form>
  );
}

export default function RisksPage() {
  const { canWrite } = useSession();
  const { data, error, loading, reload } = useResource(risksApi.list);
  const [panel, setPanel] = useState<{ risk: Risk | null } | null>(null);

  if (loading) return <Spinner />;
  if (error) return <ErrorBanner message={error} onRetry={reload} />;

  const risks = data?.data ?? [];
  const open = risks.filter((r) => r.status !== 'closed');
  const addButton = canWrite ? (
    <button className="btn btn-primary" onClick={() => setPanel({ risk: null })}>
      Registrar riesgo
    </button>
  ) : undefined;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Registro de riesgos"
        subtitle="Evaluación y tratamiento de riesgos de seguridad de la información"
        action={risks.length > 0 ? addButton : undefined}
      />

      {risks.length === 0 ? (
        <EmptyState title="Aún no hay riesgos registrados" action={addButton}>
          Para cada activo importante pregúntate qué podría salir mal, qué tan probable es y cuánto
          daño causaría.
        </EmptyState>
      ) : (
        <>
          <RiskMatrix risks={open} />

          <div className="overflow-x-auto rounded-lg border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50 text-left">
                  <th className="px-4 py-3 font-medium">Riesgo</th>
                  <th className="px-4 py-3 font-medium">Inherente</th>
                  <th className="hidden px-4 py-3 font-medium sm:table-cell">Residual</th>
                  <th className="hidden px-4 py-3 font-medium md:table-cell">Tratamiento</th>
                  <th className="hidden px-4 py-3 font-medium md:table-cell">Estado</th>
                  {canWrite && (
                    <th className="px-4 py-3">
                      <span className="sr-only">Acciones</span>
                    </th>
                  )}
                </tr>
              </thead>
              <tbody>
                {risks.map((risk) => (
                  <tr key={risk.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3">
                      <span className="font-mono text-xs">{risk.code}</span>
                      <span className="block max-w-md font-medium">{risk.title}</span>
                      {risk.asset && <span className="block text-xs text-muted-foreground">{risk.asset}</span>}
                    </td>
                    <td className="px-4 py-3">
                      <RatingBadge level={risk.risk_level} />
                    </td>
                    <td className="hidden px-4 py-3 sm:table-cell">
                      {risk.residual_level != null ? (
                        <RatingBadge level={risk.residual_level} />
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </td>
                    <td className="hidden px-4 py-3 md:table-cell">{treatmentLabels[risk.treatment]}</td>
                    <td className="hidden px-4 py-3 md:table-cell">{statusLabels[risk.status]}</td>
                    {canWrite && (
                      <td className="px-4 py-3 text-right">
                        <button className="btn btn-outline h-8 px-3" onClick={() => setPanel({ risk })}>
                          Editar
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {panel && (
        <Panel title={panel.risk ? `Riesgo ${panel.risk.code}` : 'Nuevo riesgo'} onClose={() => setPanel(null)}>
          <RiskForm
            key={panel.risk?.id ?? 'new'}
            risk={panel.risk}
            onDone={() => {
              setPanel(null);
              reload();
            }}
          />
        </Panel>
      )}
    </div>
  );
}
