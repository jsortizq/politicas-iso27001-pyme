'use client';

import { useMemo, useState } from 'react';
import { errorMessage, soaApi, type SoAEntry } from '@/lib/api';
import { useSession } from '@/lib/session';
import { useResource } from '@/lib/useResource';
import {
  EmptyState,
  ErrorBanner,
  Field,
  PageHeader,
  Panel,
  ProgressBar,
  Spinner,
} from '@/components/ui/feedback';

const categories: Record<string, string> = {
  'A.5': 'Organizacionales',
  'A.6': 'Personas',
  'A.7': 'Físicos',
  'A.8': 'Tecnológicos',
};

const categoryOf = (controlId: string) => controlId.split('.').slice(0, 2).join('.');

function StatusBadge({ entry }: { entry: SoAEntry }) {
  if (!entry.applicable) return <span className="badge badge-na">No aplica</span>;
  if (entry.implementation_pct >= 100) return <span className="badge badge-full">Implementado</span>;
  if (entry.implementation_pct > 0) return <span className="badge badge-partial">Parcial</span>;
  return <span className="badge badge-none">Sin iniciar</span>;
}

function EditForm({
  entry,
  readOnly,
  onSaved,
}: {
  entry: SoAEntry;
  readOnly: boolean;
  onSaved: () => void;
}) {
  const [form, setForm] = useState({
    ...entry,
    review_date: entry.review_date ? entry.review_date.slice(0, 10) : '',
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) =>
    setForm((current) => ({ ...current, [key]: value }));

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!form.applicable && !form.justification.trim()) {
      setError('Indica por qué el control no aplica: un auditor pedirá esta justificación.');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await soaApi.update(entry.id, {
        ...form,
        // Percentages are meaningless for an excluded control.
        implementation_pct: form.applicable ? form.implementation_pct : 0,
        effectiveness_pct: form.applicable ? form.effectiveness_pct : 0,
        review_date: form.review_date ? `${form.review_date}T00:00:00Z` : undefined,
      });
      onSaved();
    } catch (err) {
      setError(errorMessage(err));
      setSaving(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <p className="text-sm text-muted-foreground">{entry.control_title}</p>
      {error && <ErrorBanner message={error} />}

      <fieldset disabled={readOnly || saving} className="space-y-5">
        <label className="flex items-center gap-3 text-sm font-medium">
          <input
            type="checkbox"
            className="h-4 w-4"
            checked={form.applicable}
            onChange={(e) => set('applicable', e.target.checked)}
          />
          Este control aplica a la organización
        </label>

        <Field
          label={form.applicable ? 'Justificación de inclusión' : 'Justificación de exclusión'}
          hint={form.applicable ? undefined : 'Obligatoria cuando el control no aplica.'}
        >
          <textarea
            className="input"
            rows={3}
            value={form.justification}
            onChange={(e) => set('justification', e.target.value)}
          />
        </Field>

        {form.applicable && (
          <>
            <Field label={`Implementación: ${form.implementation_pct}%`}>
              <input
                type="range"
                min={0}
                max={100}
                step={5}
                className="w-full"
                value={form.implementation_pct}
                onChange={(e) => set('implementation_pct', Number(e.target.value))}
              />
            </Field>
            <Field label="Evidencia de implementación">
              <textarea
                className="input"
                rows={2}
                value={form.implementation_evidence}
                onChange={(e) => set('implementation_evidence', e.target.value)}
              />
            </Field>
            <Field
              label={`Efectividad: ${form.effectiveness_pct}%`}
              hint="Un control implementado no es automáticamente efectivo: evalúalo por separado."
            >
              <input
                type="range"
                min={0}
                max={100}
                step={5}
                className="w-full"
                value={form.effectiveness_pct}
                onChange={(e) => set('effectiveness_pct', Number(e.target.value))}
              />
            </Field>
            <Field label="Evidencia de efectividad">
              <textarea
                className="input"
                rows={2}
                value={form.effectiveness_evidence}
                onChange={(e) => set('effectiveness_evidence', e.target.value)}
              />
            </Field>
          </>
        )}

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Responsable">
            <input
              className="input"
              value={form.responsible_role}
              onChange={(e) => set('responsible_role', e.target.value)}
              placeholder="p. ej. CISO"
            />
          </Field>
          <Field label="Próxima revisión">
            <input
              type="date"
              className="input"
              value={form.review_date}
              onChange={(e) => set('review_date', e.target.value)}
            />
          </Field>
        </div>
      </fieldset>

      {readOnly ? (
        <p className="text-sm text-muted-foreground">Tu rol es de solo lectura.</p>
      ) : (
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? 'Guardando…' : 'Guardar evaluación'}
        </button>
      )}
    </form>
  );
}

export default function SoAPage() {
  const { canWrite } = useSession();
  const { data, error, loading, reload } = useResource(soaApi.list);
  const [filter, setFilter] = useState<'all' | 'applicable' | 'not_applicable'>('all');
  const [category, setCategory] = useState('all');
  const [query, setQuery] = useState('');
  const [editing, setEditing] = useState<SoAEntry | null>(null);
  const [initializing, setInitializing] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const entries = useMemo(() => data?.data ?? [], [data]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return entries.filter((e) => {
      if (filter === 'applicable' && !e.applicable) return false;
      if (filter === 'not_applicable' && e.applicable) return false;
      if (category !== 'all' && categoryOf(e.control_id) !== category) return false;
      if (q && !`${e.control_id} ${e.control_title ?? ''}`.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [entries, filter, category, query]);

  const initialize = async () => {
    setInitializing(true);
    setActionError(null);
    try {
      await soaApi.initialize();
      await reload();
    } catch (err) {
      setActionError(errorMessage(err));
    } finally {
      setInitializing(false);
    }
  };

  if (loading) return <Spinner />;
  if (error) return <ErrorBanner message={error} onRetry={reload} />;

  const missing = 93 - entries.length;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Declaración de Aplicabilidad"
        subtitle="Evaluación de los controles del Anexo A de ISO/IEC 27001:2022"
        action={
          canWrite && entries.length > 0 && missing > 0 ? (
            <button className="btn btn-outline" onClick={initialize} disabled={initializing}>
              {initializing ? 'Cargando…' : `Añadir ${missing} controles faltantes`}
            </button>
          ) : undefined
        }
      />

      {actionError && <ErrorBanner message={actionError} />}

      {entries.length === 0 ? (
        <EmptyState
          title="Tu Declaración de Aplicabilidad está vacía"
          action={
            canWrite ? (
              <button className="btn btn-primary" onClick={initialize} disabled={initializing}>
                {initializing ? 'Cargando…' : 'Cargar los 93 controles'}
              </button>
            ) : undefined
          }
        >
          {canWrite
            ? 'Carga el catálogo del Anexo A. Todos los controles empiezan como aplicables y sin implementar; después evalúa cada uno.'
            : 'Un administrador debe cargar el catálogo de controles.'}
        </EmptyState>
      ) : (
        <>
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
            <input
              type="search"
              className="input lg:max-w-xs"
              placeholder="Buscar control…"
              aria-label="Buscar control"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <select
              className="input lg:max-w-[13rem]"
              aria-label="Categoría"
              value={category}
              onChange={(e) => setCategory(e.target.value)}
            >
              <option value="all">Todas las categorías</option>
              {Object.entries(categories).map(([id, name]) => (
                <option key={id} value={id}>
                  {id} · {name}
                </option>
              ))}
            </select>
            <div className="flex gap-2">
              {(['all', 'applicable', 'not_applicable'] as const).map((f) => (
                <button
                  key={f}
                  onClick={() => setFilter(f)}
                  aria-pressed={filter === f}
                  className={`btn h-9 px-3 ${filter === f ? 'btn-primary' : 'btn-outline'}`}
                >
                  {f === 'all' ? 'Todos' : f === 'applicable' ? 'Aplicables' : 'No aplicables'}
                </button>
              ))}
            </div>
          </div>

          <div className="overflow-x-auto rounded-lg border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50 text-left">
                  <th className="px-4 py-3 font-medium">Control</th>
                  <th className="hidden px-4 py-3 font-medium sm:table-cell">Estado</th>
                  <th className="hidden px-4 py-3 font-medium sm:table-cell">Implementación</th>
                  <th className="hidden px-4 py-3 font-medium md:table-cell">Efectividad</th>
                  <th className="hidden px-4 py-3 font-medium lg:table-cell">Responsable</th>
                  <th className="px-4 py-3">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((entry) => (
                  <tr key={entry.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3">
                      <span className="font-mono text-xs">{entry.control_id}</span>
                      <span className="block max-w-md">{entry.control_title}</span>
                      <span className="mt-1.5 block sm:hidden">
                        <StatusBadge entry={entry} />
                      </span>
                    </td>
                    <td className="hidden px-4 py-3 sm:table-cell">
                      <StatusBadge entry={entry} />
                    </td>
                    {(['implementation_pct', 'effectiveness_pct'] as const).map((key, i) => (
                      <td
                        key={key}
                        className={`hidden w-40 px-4 py-3 ${i === 0 ? 'sm:table-cell' : 'md:table-cell'}`}
                      >
                        {entry.applicable ? (
                          <div className="flex items-center gap-2">
                            <ProgressBar value={entry[key]} />
                            <span className="w-9 text-right text-xs tabular-nums text-muted-foreground">
                              {entry[key]}%
                            </span>
                          </div>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                    ))}
                    <td className="hidden px-4 py-3 text-xs lg:table-cell">
                      {entry.responsible_role || <span className="text-muted-foreground">—</span>}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button className="btn btn-outline h-8 px-3" onClick={() => setEditing(entry)}>
                        {canWrite ? 'Evaluar' : 'Ver'}
                      </button>
                    </td>
                  </tr>
                ))}
                {filtered.length === 0 && (
                  <tr>
                    <td colSpan={6} className="px-4 py-10 text-center text-muted-foreground">
                      Ningún control coincide con los filtros.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <p className="text-xs text-muted-foreground">
            Mostrando {filtered.length} de {entries.length} controles
          </p>
        </>
      )}

      {editing && (
        <Panel title={`Control ${editing.control_id}`} onClose={() => setEditing(null)}>
          <EditForm
            key={editing.id}
            entry={editing}
            readOnly={!canWrite}
            onSaved={() => {
              setEditing(null);
              reload();
            }}
          />
        </Panel>
      )}
    </div>
  );
}
