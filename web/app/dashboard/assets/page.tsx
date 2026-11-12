'use client';

import { useState } from 'react';
import { assetsApi, errorMessage, type Asset, type AssetInput } from '@/lib/api';
import { useSession } from '@/lib/session';
import { useResource } from '@/lib/useResource';
import { EmptyState, ErrorBanner, Field, PageHeader, Panel, Spinner } from '@/components/ui/feedback';

const typeLabels: Record<Asset['asset_type'], string> = {
  information: 'Información',
  software: 'Software',
  hardware: 'Hardware',
  service: 'Servicio',
  people: 'Personas',
  facility: 'Instalación',
};

const classificationLabels: Record<Asset['classification'], string> = {
  public: 'Pública',
  internal: 'Interna',
  confidential: 'Confidencial',
  restricted: 'Restringida',
};

const levelLabels = ['', 'Muy baja', 'Baja', 'Media', 'Alta', 'Muy alta'];

const emptyAsset: AssetInput = {
  name: '',
  asset_type: 'information',
  description: '',
  owner: '',
  location: '',
  classification: 'internal',
  confidentiality: 3,
  integrity: 3,
  availability: 3,
  status: 'active',
};

function CriticalityBadge({ level }: { level: number }) {
  const style = level >= 5 ? 'badge-critical' : level === 4 ? 'badge-none' : level === 3 ? 'badge-partial' : 'badge-full';
  return (
    <span className={`badge ${style}`}>
      {level} · {levelLabels[level]}
    </span>
  );
}

function AssetForm({
  asset,
  onSaved,
  onDeleted,
}: {
  asset: Asset | null;
  onSaved: () => void;
  onDeleted: () => void;
}) {
  const [form, setForm] = useState<AssetInput>(asset ?? emptyAsset);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const set = <K extends keyof AssetInput>(key: K, value: AssetInput[K]) =>
    setForm((current) => ({ ...current, [key]: value }));

  const run = async (action: () => Promise<unknown>, done: () => void) => {
    setBusy(true);
    setError(null);
    try {
      await action();
      done();
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  };

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    run(() => (asset ? assetsApi.update(asset.id, form) : assetsApi.create(form)), onSaved);
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {error && <ErrorBanner message={error} />}
      <fieldset disabled={busy} className="space-y-5">
        <Field label="Nombre">
          <input className="input" required maxLength={200} value={form.name} onChange={(e) => set('name', e.target.value)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Tipo">
            <select className="input" value={form.asset_type} onChange={(e) => set('asset_type', e.target.value as Asset['asset_type'])}>
              {Object.entries(typeLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Clasificación">
            <select className="input" value={form.classification} onChange={(e) => set('classification', e.target.value as Asset['classification'])}>
              {Object.entries(classificationLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Responsable">
            <input className="input" value={form.owner} onChange={(e) => set('owner', e.target.value)} />
          </Field>
          <Field label="Ubicación">
            <input className="input" value={form.location} onChange={(e) => set('location', e.target.value)} />
          </Field>
        </div>
        <Field label="Descripción">
          <textarea className="input" rows={2} value={form.description} onChange={(e) => set('description', e.target.value)} />
        </Field>

        <div className="space-y-3 rounded-md border p-4">
          <p className="text-sm font-medium">Valoración</p>
          <p className="text-xs text-muted-foreground">
            ¿Cuánto afectaría a la organización perder cada propiedad? La criticidad del activo es el
            valor más alto de las tres.
          </p>
          <div className="grid gap-4 sm:grid-cols-3">
            {(
              [
                ['confidentiality', 'Confidencialidad'],
                ['integrity', 'Integridad'],
                ['availability', 'Disponibilidad'],
              ] as const
            ).map(([key, label]) => (
              <Field key={key} label={label}>
                <select className="input" value={form[key]} onChange={(e) => set(key, Number(e.target.value))}>
                  {[1, 2, 3, 4, 5].map((n) => (
                    <option key={n} value={n}>
                      {n} · {levelLabels[n]}
                    </option>
                  ))}
                </select>
              </Field>
            ))}
          </div>
        </div>

        <Field label="Estado">
          <select className="input" value={form.status} onChange={(e) => set('status', e.target.value as Asset['status'])}>
            <option value="active">Activo</option>
            <option value="retired">Retirado</option>
          </select>
        </Field>
      </fieldset>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {busy ? 'Guardando…' : asset ? 'Guardar cambios' : 'Añadir activo'}
        </button>
        {asset &&
          (confirmDelete ? (
            <span className="flex items-center gap-2 text-sm">
              ¿Eliminar definitivamente?
              <button type="button" className="btn btn-danger h-8 px-3" disabled={busy} onClick={() => run(() => assetsApi.remove(asset.id), onDeleted)}>
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

export default function AssetsPage() {
  const { canWrite } = useSession();
  const { data, error, loading, reload } = useResource(assetsApi.list);
  const [panel, setPanel] = useState<{ asset: Asset | null } | null>(null);

  if (loading) return <Spinner />;
  if (error) return <ErrorBanner message={error} onRetry={reload} />;

  const assets = data?.data ?? [];
  const close = () => {
    setPanel(null);
    reload();
  };
  const addButton = canWrite ? (
    <button className="btn btn-primary" onClick={() => setPanel({ asset: null })}>
      Añadir activo
    </button>
  ) : undefined;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Inventario de activos"
        subtitle="Información y recursos que el SGSI debe proteger, ordenados por criticidad"
        action={assets.length > 0 ? addButton : undefined}
      />

      {assets.length === 0 ? (
        <EmptyState title="Aún no hay activos registrados" action={addButton}>
          Empieza por lo que más dolería perder: bases de datos de clientes, sistemas de facturación,
          servidores, proveedores críticos.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left">
                <th className="px-4 py-3 font-medium">Activo</th>
                <th className="hidden px-4 py-3 font-medium sm:table-cell">Tipo</th>
                <th className="hidden px-4 py-3 font-medium md:table-cell">Clasificación</th>
                <th className="px-4 py-3 font-medium">Criticidad</th>
                <th className="hidden px-4 py-3 font-medium lg:table-cell">C / I / D</th>
                <th className="hidden px-4 py-3 font-medium lg:table-cell">Responsable</th>
                {canWrite && (
                  <th className="px-4 py-3">
                    <span className="sr-only">Acciones</span>
                  </th>
                )}
              </tr>
            </thead>
            <tbody>
              {assets.map((asset) => (
                <tr key={asset.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3">
                    <span className="font-medium">{asset.name}</span>
                    {asset.status === 'retired' && <span className="badge badge-na ml-2">Retirado</span>}
                  </td>
                  <td className="hidden px-4 py-3 sm:table-cell">{typeLabels[asset.asset_type]}</td>
                  <td className="hidden px-4 py-3 md:table-cell">{classificationLabels[asset.classification]}</td>
                  <td className="px-4 py-3">
                    <CriticalityBadge level={asset.criticality} />
                  </td>
                  <td className="hidden px-4 py-3 tabular-nums lg:table-cell">
                    {asset.confidentiality} / {asset.integrity} / {asset.availability}
                  </td>
                  <td className="hidden px-4 py-3 lg:table-cell">
                    {asset.owner || <span className="text-muted-foreground">—</span>}
                  </td>
                  {canWrite && (
                    <td className="px-4 py-3 text-right">
                      <button className="btn btn-outline h-8 px-3" onClick={() => setPanel({ asset })}>
                        Editar
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {panel && (
        <Panel title={panel.asset ? 'Editar activo' : 'Nuevo activo'} onClose={() => setPanel(null)}>
          <AssetForm key={panel.asset?.id ?? 'new'} asset={panel.asset} onSaved={close} onDeleted={close} />
        </Panel>
      )}
    </div>
  );
}
