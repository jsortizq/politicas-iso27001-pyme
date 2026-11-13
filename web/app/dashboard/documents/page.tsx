'use client';

import { useState } from 'react';
import {
  ApiError,
  documentsApi,
  errorMessage,
  type DocumentMeta,
  type DocumentStatus,
  type DocumentType,
  type DocumentVersion,
  type SgsiDocument,
  type VersionStatus,
} from '@/lib/api';
import { formatBytes, formatDay, formatInstant } from '@/lib/format';
import { useSession } from '@/lib/session';
import { useResource } from '@/lib/useResource';
import { ControlPicker } from '@/components/ControlPicker';
import { IntegrityResult, type IntegrityState } from '@/components/IntegrityResult';
import { EmptyState, ErrorBanner, Field, PageHeader, Panel, Spinner } from '@/components/ui/feedback';

const typeLabels: Record<DocumentType, string> = {
  policy: 'Política',
  procedure: 'Procedimiento',
  record: 'Registro',
  template: 'Plantilla',
  report: 'Informe',
  other: 'Otro',
};

const statusLabels: Record<DocumentStatus, string> = {
  draft: 'Borrador',
  review: 'En revisión',
  approved: 'Aprobado',
  obsolete: 'Obsoleto',
};

const statusBadge: Record<DocumentStatus, string> = {
  draft: 'badge-na',
  review: 'badge-partial',
  approved: 'badge-full',
  obsolete: 'badge-none',
};

const versionLabels: Record<VersionStatus, string> = {
  draft: 'Borrador',
  in_review: 'En revisión',
  approved: 'Vigente',
  rejected: 'Rechazada',
  superseded: 'Sustituida',
};

const versionBadge: Record<VersionStatus, string> = {
  draft: 'badge-na',
  in_review: 'badge-partial',
  approved: 'badge-full',
  rejected: 'badge-none',
  superseded: 'badge-na',
};

/** A 409 here means the document changed under us (someone else acted first). */
function documentError(err: unknown): string {
  if (err instanceof ApiError && err.status === 409) {
    return 'La acción ya no aplica al estado actual del documento. Revisa su estado e inténtalo de nuevo.';
  }
  return errorMessage(err);
}

function metaOf(doc: SgsiDocument): DocumentMeta {
  return {
    title: doc.title,
    doc_type: doc.doc_type,
    owner: doc.owner,
    next_review: doc.next_review ?? '',
    controls: doc.controls,
  };
}

const emptyMeta: DocumentMeta = { title: '', doc_type: 'policy', owner: '', next_review: '', controls: [] };

function MetaFields({
  meta,
  onChange,
  disabled,
}: {
  meta: DocumentMeta;
  onChange: (meta: DocumentMeta) => void;
  disabled: boolean;
}) {
  const set = <K extends keyof DocumentMeta>(key: K, value: DocumentMeta[K]) => onChange({ ...meta, [key]: value });
  return (
    <>
      <Field label="Título">
        <input className="input" required maxLength={300} disabled={disabled} value={meta.title} onChange={(e) => set('title', e.target.value)} />
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Tipo">
          <select className="input" disabled={disabled} value={meta.doc_type} onChange={(e) => set('doc_type', e.target.value as DocumentType)}>
            {Object.entries(typeLabels).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Responsable">
          <input className="input" maxLength={200} disabled={disabled} value={meta.owner} onChange={(e) => set('owner', e.target.value)} placeholder="p. ej. CISO" />
        </Field>
      </div>
      <Field label="Próxima revisión" hint="La norma pide revisar los documentos a intervalos planificados.">
        <input type="date" className="input" disabled={disabled} value={meta.next_review} onChange={(e) => set('next_review', e.target.value)} />
      </Field>
      <div className="space-y-1.5">
        <span className="text-sm font-medium">Controles que respalda</span>
        <ControlPicker value={meta.controls} onChange={(controls) => set('controls', controls)} disabled={disabled} />
      </div>
    </>
  );
}

function NewDocumentForm({ onDone }: { onDone: () => void }) {
  const [meta, setMeta] = useState<DocumentMeta>(emptyMeta);
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
    form.set('doc_type', meta.doc_type);
    form.set('owner', meta.owner);
    form.set('next_review', meta.next_review);
    form.set('controls', meta.controls.join(','));
    form.set('file', file);
    try {
      await documentsApi.create(form);
      onDone();
    } catch (err) {
      setError(documentError(err));
      setBusy(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {error && <ErrorBanner message={error} />}
      <fieldset disabled={busy} className="space-y-5">
        <Field label="Archivo" hint="Será la versión 1, en borrador hasta que se apruebe.">
          <input type="file" required className="input h-auto py-2" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        </Field>
        <MetaFields meta={meta} onChange={setMeta} disabled={busy} />
      </fieldset>
      <button type="submit" className="btn btn-primary" disabled={busy || !file}>
        {busy ? 'Subiendo…' : 'Crear documento'}
      </button>
    </form>
  );
}

function VersionCard({
  doc,
  version,
  onAction,
  busy,
}: {
  doc: SgsiDocument;
  version: DocumentVersion;
  onAction: (action: () => Promise<SgsiDocument>) => void;
  busy: boolean;
}) {
  const { canWrite, canManageMembers } = useSession();
  const [rejecting, setRejecting] = useState(false);
  const [note, setNote] = useState('');
  const [integrity, setIntegrity] = useState<IntegrityState>(null);

  const isLatest = version.number === doc.versions[0].number;
  const open = doc.status !== 'obsolete' && isLatest;

  const checkIntegrity = async () => {
    setIntegrity('checking');
    try {
      setIntegrity(await documentsApi.integrity(doc.id, version.number));
    } catch {
      setIntegrity('error');
    }
  };

  return (
    <li className="space-y-3 rounded-md border p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="font-medium">
          Versión {version.number}{' '}
          <span className={`badge ml-1 ${versionBadge[version.status]}`}>
            {version.status === 'approved' && doc.status === 'obsolete' ? 'Aprobada (retirada)' : versionLabels[version.status]}
          </span>
        </p>
        <a className="btn btn-outline h-8 px-3" href={documentsApi.downloadUrl(doc.id, version.number)}>
          Descargar
        </a>
      </div>

      <div className="space-y-1 text-sm">
        <div className="break-all">
          {version.file_name} · {formatBytes(version.file_size)}
        </div>
        <div className="text-muted-foreground">
          Subida por {version.uploaded_by} el {formatInstant(version.uploaded_at)}
        </div>
        {version.change_summary && <div>Cambios: {version.change_summary}</div>}
        {version.reviewed_by && (
          <div className="text-muted-foreground">
            {version.status === 'rejected' ? 'Rechazada' : 'Aprobada'} por {version.reviewed_by} el{' '}
            {formatInstant(version.reviewed_at)}
            {version.review_note && <span className="text-foreground"> — «{version.review_note}»</span>}
          </div>
        )}
        <div className="break-all font-mono text-xs text-muted-foreground" title="Huella SHA-256 del archivo">
          SHA-256 {version.sha256}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {open && canWrite && version.status === 'draft' && (
          <button className="btn btn-primary h-8 px-3" disabled={busy} onClick={() => onAction(() => documentsApi.submit(doc.id, version.number))}>
            Enviar a revisión
          </button>
        )}
        {open && canManageMembers && version.status === 'in_review' && !rejecting && (
          <>
            <button className="btn btn-primary h-8 px-3" disabled={busy} onClick={() => onAction(() => documentsApi.approve(doc.id, version.number, ''))}>
              Aprobar
            </button>
            <button className="btn btn-danger h-8 px-3" disabled={busy} onClick={() => setRejecting(true)}>
              Rechazar
            </button>
          </>
        )}
        {open && version.status === 'in_review' && !canManageMembers && (
          <span className="text-xs text-muted-foreground">Pendiente de aprobación por un propietario o administrador.</span>
        )}
        <button className="btn btn-outline h-8 px-3" disabled={integrity === 'checking'} onClick={checkIntegrity}>
          {integrity === 'checking' ? 'Comprobando…' : 'Comprobar integridad'}
        </button>
      </div>

      {rejecting && open && version.status === 'in_review' && (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            onAction(() => documentsApi.reject(doc.id, version.number, note));
          }}
        >
          <Field label="Motivo del rechazo">
            <textarea className="input" rows={2} required value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
          <div className="flex gap-2">
            <button type="submit" className="btn btn-danger h-8 px-3" disabled={busy || !note.trim()}>
              Confirmar rechazo
            </button>
            <button type="button" className="btn btn-outline h-8 px-3" onClick={() => setRejecting(false)}>
              Cancelar
            </button>
          </div>
        </form>
      )}

      <IntegrityResult result={integrity} />
    </li>
  );
}

function DocumentPanel({
  initial,
  onChanged,
  onDeleted,
}: {
  initial: SgsiDocument;
  onChanged: () => void;
  onDeleted: () => void;
}) {
  const { canWrite, canManageMembers } = useSession();
  const [doc, setDoc] = useState(initial);
  const [meta, setMeta] = useState<DocumentMeta>(metaOf(initial));
  const [file, setFile] = useState<File | null>(null);
  const [summary, setSummary] = useState('');
  const [fileKey, setFileKey] = useState(0); // remounts the file input to clear it
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<'obsolete' | 'delete' | null>(null);
  const [error, setError] = useState<string | null>(null);

  const obsolete = doc.status === 'obsolete';
  const editable = canWrite && !obsolete;
  // A document that was ever in force is history and can only be withdrawn.
  const everApproved = doc.versions.some((v) => v.reviewed_at && (v.status === 'approved' || v.status === 'superseded'));

  const run = async (action: () => Promise<SgsiDocument>) => {
    setBusy(true);
    setError(null);
    try {
      const updated = await action();
      setDoc(updated);
      setMeta(metaOf(updated));
      setConfirm(null);
      onChanged();
    } catch (err) {
      setError(documentError(err));
    } finally {
      setBusy(false);
    }
  };

  const uploadVersion = (event: React.FormEvent) => {
    event.preventDefault();
    if (!file) return;
    const form = new FormData();
    form.set('file', file);
    form.set('change_summary', summary);
    run(async () => {
      const updated = await documentsApi.addVersion(doc.id, form);
      setFile(null);
      setSummary('');
      setFileKey((k) => k + 1);
      return updated;
    });
  };

  const remove = async () => {
    setBusy(true);
    setError(null);
    try {
      await documentsApi.remove(doc.id);
      onDeleted();
    } catch (err) {
      setError(documentError(err));
      setBusy(false);
    }
  };

  return (
    <div className="space-y-8">
      {error && <ErrorBanner message={error} />}

      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className={`badge ${statusBadge[doc.status]}`}>{statusLabels[doc.status]}</span>
        {doc.review_overdue && <span className="badge badge-none">Revisión vencida</span>}
        <span className="text-muted-foreground">
          {obsolete
            ? doc.approved_version
              ? `Retirado. Última versión aprobada: ${doc.approved_version}`
              : 'Retirado'
            : doc.approved_version
              ? `Versión vigente: ${doc.approved_version}`
              : 'Sin versión vigente'}
        </span>
      </div>

      <form
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault();
          run(() => documentsApi.update(doc.id, meta));
        }}
      >
        <MetaFields meta={meta} onChange={setMeta} disabled={!editable || busy} />
        {editable && (
          <button type="submit" className="btn btn-outline" disabled={busy}>
            Guardar datos
          </button>
        )}
      </form>

      <section className="space-y-3">
        <h3 className="font-semibold">Versiones</h3>
        <ol className="space-y-3">
          {doc.versions.map((v) => (
            <VersionCard key={v.number} doc={doc} version={v} onAction={run} busy={busy} />
          ))}
        </ol>
      </section>

      {editable && (
        <form onSubmit={uploadVersion} className="space-y-4 rounded-md border p-4">
          <h3 className="font-semibold">Subir una versión nueva</h3>
          <p className="text-xs text-muted-foreground">
            Las versiones anteriores se conservan. La vigente sigue en vigor hasta que se apruebe la nueva.
          </p>
          <Field label="Archivo">
            <input key={fileKey} type="file" required className="input h-auto py-2" disabled={busy} onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
          </Field>
          <Field label="Resumen de cambios">
            <textarea className="input" rows={2} maxLength={2000} disabled={busy} value={summary} onChange={(e) => setSummary(e.target.value)} />
          </Field>
          <button type="submit" className="btn btn-primary" disabled={busy || !file}>
            {busy ? 'Subiendo…' : `Subir versión ${doc.versions[0].number + 1}`}
          </button>
        </form>
      )}

      {canManageMembers && !obsolete && (
        <section className="space-y-3 border-t pt-6">
          {confirm === null && (
            <div className="flex flex-wrap gap-2">
              <button className="btn btn-danger" onClick={() => setConfirm('obsolete')}>
                Marcar como obsoleto
              </button>
              {!everApproved && (
                <button className="btn btn-danger" onClick={() => setConfirm('delete')}>
                  Eliminar
                </button>
              )}
            </div>
          )}
          {confirm === 'obsolete' && (
            <div className="space-y-2 text-sm">
              <p>El documento dejará de poder modificarse. Sus versiones seguirán disponibles para consulta. No se puede deshacer.</p>
              <div className="flex gap-2">
                <button className="btn btn-danger h-8 px-3" disabled={busy} onClick={() => run(() => documentsApi.obsolete(doc.id))}>
                  Sí, marcar obsoleto
                </button>
                <button className="btn btn-outline h-8 px-3" onClick={() => setConfirm(null)}>
                  Cancelar
                </button>
              </div>
            </div>
          )}
          {confirm === 'delete' && (
            <div className="space-y-2 text-sm">
              <p>Se eliminarán el documento y todos sus archivos. No se puede deshacer.</p>
              <div className="flex gap-2">
                <button className="btn btn-danger h-8 px-3" disabled={busy} onClick={remove}>
                  Sí, eliminar
                </button>
                <button className="btn btn-outline h-8 px-3" onClick={() => setConfirm(null)}>
                  Cancelar
                </button>
              </div>
            </div>
          )}
        </section>
      )}
    </div>
  );
}

export default function DocumentsPage() {
  const { canWrite } = useSession();
  const { data, error, loading, reload } = useResource(documentsApi.list);
  const [creating, setCreating] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);

  if (loading) return <Spinner />;
  if (error) return <ErrorBanner message={error} onRetry={reload} />;

  const documents = data?.data ?? [];
  const opened = documents.find((d) => d.id === openId) ?? null;
  const addButton = canWrite ? (
    <button className="btn btn-primary" onClick={() => setCreating(true)}>
      Nuevo documento
    </button>
  ) : undefined;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Documentos"
        subtitle="Políticas, procedimientos y registros del SGSI, con versiones y aprobación"
        action={documents.length > 0 ? addButton : undefined}
      />

      {documents.length === 0 ? (
        <EmptyState title="Aún no hay documentos" action={addButton}>
          Sube tu política de seguridad de la información o cualquier procedimiento. Cada cambio será una
          versión nueva y quedará constancia de quién la aprobó.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left">
                <th className="px-4 py-3 font-medium">Documento</th>
                <th className="px-4 py-3 font-medium">Estado</th>
                <th className="hidden px-4 py-3 font-medium sm:table-cell">Versión vigente</th>
                <th className="hidden px-4 py-3 font-medium md:table-cell">Próxima revisión</th>
                <th className="hidden px-4 py-3 font-medium lg:table-cell">Controles</th>
                <th className="px-4 py-3">
                  <span className="sr-only">Acciones</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {documents.map((d) => {
                const latest = d.versions[0];
                const pending = d.approved_version && latest.number > d.approved_version && (latest.status === 'draft' || latest.status === 'in_review');
                return (
                  <tr key={d.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3">
                      <span className="block max-w-md font-medium">{d.title}</span>
                      <span className="text-xs text-muted-foreground">
                        {typeLabels[d.doc_type]}
                        {d.owner && ` · ${d.owner}`}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`badge ${statusBadge[d.status]}`}>{statusLabels[d.status]}</span>
                      {d.review_overdue && <span className="badge badge-none ml-1">Revisión vencida</span>}
                      {pending && (
                        <span className="mt-1 block text-xs text-muted-foreground">
                          v{latest.number} {latest.status === 'in_review' ? 'en revisión' : 'en borrador'}
                        </span>
                      )}
                    </td>
                    <td className="hidden px-4 py-3 sm:table-cell">
                      {d.approved_version && d.status !== 'obsolete' ? (
                        <a className="underline underline-offset-4" href={documentsApi.downloadUrl(d.id, d.approved_version)}>
                          v{d.approved_version}
                        </a>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </td>
                    <td className="hidden px-4 py-3 md:table-cell">{formatDay(d.next_review)}</td>
                    <td className="hidden px-4 py-3 font-mono text-xs lg:table-cell">
                      {d.controls.length === 0 ? (
                        <span className="font-sans text-muted-foreground">—</span>
                      ) : (
                        <>
                          {d.controls.slice(0, 3).join(', ')}
                          {d.controls.length > 3 && ` +${d.controls.length - 3}`}
                        </>
                      )}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button className="btn btn-outline h-8 px-3" onClick={() => setOpenId(d.id)}>
                        Abrir
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {creating && (
        <Panel title="Nuevo documento" onClose={() => setCreating(false)}>
          <NewDocumentForm
            onDone={() => {
              setCreating(false);
              reload();
            }}
          />
        </Panel>
      )}

      {opened && (
        <Panel title={opened.title} onClose={() => setOpenId(null)}>
          <DocumentPanel
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
