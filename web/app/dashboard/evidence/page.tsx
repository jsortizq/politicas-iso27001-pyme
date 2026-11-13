'use client';

import { useState } from 'react';
import { errorMessage, evidenceApi, type Evidence, type EvidenceMeta } from '@/lib/api';
import { formatBytes, formatDay, formatInstant, todayLocal } from '@/lib/format';
import { useSession } from '@/lib/session';
import { useResource } from '@/lib/useResource';
import { ControlPicker } from '@/components/ControlPicker';
import { IntegrityResult, type IntegrityState } from '@/components/IntegrityResult';
import { EmptyState, ErrorBanner, Field, PageHeader, Panel, Spinner } from '@/components/ui/feedback';

function MetaFields({
  meta,
  onChange,
  disabled,
  titleRequired,
}: {
  meta: EvidenceMeta;
  onChange: (meta: EvidenceMeta) => void;
  disabled: boolean;
  titleRequired: boolean;
}) {
  const set = <K extends keyof EvidenceMeta>(key: K, value: EvidenceMeta[K]) => onChange({ ...meta, [key]: value });
  return (
    <>
      <Field label="Título" hint={titleRequired ? undefined : 'Si lo dejas vacío se usa el nombre del archivo.'}>
        <input className="input" required={titleRequired} maxLength={300} disabled={disabled} value={meta.title} onChange={(e) => set('title', e.target.value)} />
      </Field>
      <Field label="Qué demuestra">
        <textarea className="input" rows={2} maxLength={4000} disabled={disabled} value={meta.description} onChange={(e) => set('description', e.target.value)} />
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Fecha de obtención">
          <input type="date" className="input" required max={todayLocal()} disabled={disabled} value={meta.collected_on} onChange={(e) => set('collected_on', e.target.value)} />
        </Field>
        <Field label="Vigente hasta" hint="Opcional. Pasada esta fecha habrá que renovarla.">
          <input type="date" className="input" min={meta.collected_on} disabled={disabled} value={meta.valid_until} onChange={(e) => set('valid_until', e.target.value)} />
        </Field>
      </div>
      <div className="space-y-1.5">
        <span className="text-sm font-medium">Controles que respalda</span>
        <ControlPicker value={meta.controls} onChange={(controls) => set('controls', controls)} disabled={disabled} />
      </div>
    </>
  );
}

function NewEvidenceForm({ onDone }: { onDone: () => void }) {
  const [meta, setMeta] = useState<EvidenceMeta>({
    title: '',
    description: '',
    collected_on: todayLocal(),
    valid_until: '',
    controls: [],
  });
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!file) return;
    setBusy(true);
    setError(null);
    const form = new FormData();
    form.set('title', meta.title);
    form.set('description', meta.description);
    form.set('collected_on', meta.collected_on);
    form.set('valid_until', meta.valid_until);
    form.set('controls', meta.controls.join(','));
    form.set('file', file);
    try {
      await evidenceApi.create(form);
      onDone();
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {error && <ErrorBanner message={error} />}
      <fieldset disabled={busy} className="space-y-5">
        <Field label="Archivo" hint="Capturas, informes, exportaciones, actas firmadas… El archivo no se podrá cambiar después.">
          <input type="file" required className="input h-auto py-2" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        </Field>
        <MetaFields meta={meta} onChange={setMeta} disabled={busy} titleRequired={false} />
      </fieldset>
      <button type="submit" className="btn btn-primary" disabled={busy || !file}>
        {busy ? 'Subiendo…' : 'Subir evidencia'}
      </button>
    </form>
  );
}

function EvidencePanel({ evidence, onDone }: { evidence: Evidence; onDone: () => void }) {
  const { canWrite, canManageMembers } = useSession();
  const [meta, setMeta] = useState<EvidenceMeta>({
    title: evidence.title,
    description: evidence.description,
    collected_on: evidence.collected_on,
    valid_until: evidence.valid_until ?? '',
    controls: evidence.controls,
  });
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [integrity, setIntegrity] = useState<IntegrityState>(null);
  const [error, setError] = useState<string | null>(null);

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

  const checkIntegrity = async () => {
    setIntegrity('checking');
    try {
      setIntegrity(await evidenceApi.integrity(evidence.id));
    } catch {
      setIntegrity('error');
    }
  };

  return (
    <div className="space-y-8">
      {error && <ErrorBanner message={error} />}

      <section className="space-y-3 rounded-md border p-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="break-all font-medium">{evidence.file_name}</p>
          <a className="btn btn-outline h-8 px-3" href={evidenceApi.downloadUrl(evidence.id)}>
            Descargar
          </a>
        </div>
        <div className="space-y-1 text-sm text-muted-foreground">
          <p>
            {formatBytes(evidence.file_size)} · subida por {evidence.uploaded_by} el {formatInstant(evidence.created_at)}
          </p>
          <p className="break-all font-mono text-xs" title="Huella SHA-256 del archivo">
            SHA-256 {evidence.sha256}
          </p>
        </div>
        <button className="btn btn-outline h-8 px-3" disabled={integrity === 'checking'} onClick={checkIntegrity}>
          {integrity === 'checking' ? 'Comprobando…' : 'Comprobar integridad'}
        </button>
        <IntegrityResult result={integrity} />
      </section>

      <form
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault();
          run(() => evidenceApi.update(evidence.id, meta));
        }}
      >
        <MetaFields meta={meta} onChange={setMeta} disabled={!canWrite || busy} titleRequired />
        {canWrite && (
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {busy ? 'Guardando…' : 'Guardar cambios'}
          </button>
        )}
      </form>

      {canManageMembers && (
        <section className="border-t pt-6">
          {confirmDelete ? (
            <div className="space-y-2 text-sm">
              <p>
                Se eliminará la evidencia y su archivo. En el registro de actividad quedará su huella
                SHA-256. No se puede deshacer.
              </p>
              <div className="flex gap-2">
                <button className="btn btn-danger h-8 px-3" disabled={busy} onClick={() => run(() => evidenceApi.remove(evidence.id))}>
                  Sí, eliminar
                </button>
                <button className="btn btn-outline h-8 px-3" onClick={() => setConfirmDelete(false)}>
                  Cancelar
                </button>
              </div>
            </div>
          ) : (
            <button className="btn btn-danger" onClick={() => setConfirmDelete(true)}>
              Eliminar
            </button>
          )}
        </section>
      )}
    </div>
  );
}

export default function EvidencePage() {
  const { canWrite } = useSession();
  const { data, error, loading, reload } = useResource(evidenceApi.list);
  const [creating, setCreating] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);

  if (loading) return <Spinner />;
  if (error) return <ErrorBanner message={error} onRetry={reload} />;

  const items = data?.data ?? [];
  const opened = items.find((e) => e.id === openId) ?? null;
  const close = () => {
    setCreating(false);
    setOpenId(null);
    reload();
  };
  const addButton = canWrite ? (
    <button className="btn btn-primary" onClick={() => setCreating(true)}>
      Subir evidencia
    </button>
  ) : undefined;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Evidencias"
        subtitle="Archivos que demuestran que los controles funcionan, con su huella de integridad"
        action={items.length > 0 ? addButton : undefined}
      />

      {items.length === 0 ? (
        <EmptyState title="Aún no hay evidencias" action={addButton}>
          Un auditor no se queda con «sí, hacemos respaldos»: pide verlo. Sube capturas, informes o
          actas y vincúlalas a los controles que respaldan.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left">
                <th className="px-4 py-3 font-medium">Evidencia</th>
                <th className="hidden px-4 py-3 font-medium sm:table-cell">Controles</th>
                <th className="hidden px-4 py-3 font-medium md:table-cell">Obtenida</th>
                <th className="px-4 py-3 font-medium">Vigencia</th>
                <th className="px-4 py-3">
                  <span className="sr-only">Acciones</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((e) => (
                <tr key={e.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3">
                    <span className="block max-w-md font-medium">{e.title}</span>
                    <a className="break-all text-xs text-muted-foreground underline underline-offset-4" href={evidenceApi.downloadUrl(e.id)}>
                      {e.file_name}
                    </a>
                  </td>
                  <td className="hidden px-4 py-3 font-mono text-xs sm:table-cell">
                    {e.controls.length === 0 ? (
                      <span className="font-sans text-muted-foreground">Sin vincular</span>
                    ) : (
                      <>
                        {e.controls.slice(0, 3).join(', ')}
                        {e.controls.length > 3 && ` +${e.controls.length - 3}`}
                      </>
                    )}
                  </td>
                  <td className="hidden px-4 py-3 md:table-cell">{formatDay(e.collected_on)}</td>
                  <td className="px-4 py-3">
                    {e.expired ? (
                      <span className="badge badge-none">Vencida</span>
                    ) : e.valid_until ? (
                      <span className="whitespace-nowrap">Hasta {formatDay(e.valid_until)}</span>
                    ) : (
                      <span className="text-muted-foreground">Sin vencimiento</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button className="btn btn-outline h-8 px-3" onClick={() => setOpenId(e.id)}>
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
        <Panel title="Nueva evidencia" onClose={() => setCreating(false)}>
          <NewEvidenceForm onDone={close} />
        </Panel>
      )}
      {opened && (
        <Panel title={opened.title} onClose={() => setOpenId(null)}>
          <EvidencePanel key={opened.id} evidence={opened} onDone={close} />
        </Panel>
      )}
    </div>
  );
}
