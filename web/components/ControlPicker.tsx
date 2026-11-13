'use client';

import { useEffect, useMemo, useState } from 'react';
import { loadCatalog, type CatalogControl } from '@/lib/api';

/**
 * Chooses Annex A controls: the selection is shown as removable chips and
 * the catalog is searched by id or title.
 */
export function ControlPicker({
  value,
  onChange,
  disabled = false,
}: {
  value: string[];
  onChange: (controls: string[]) => void;
  disabled?: boolean;
}) {
  const [catalog, setCatalog] = useState<CatalogControl[]>([]);
  const [failed, setFailed] = useState(false);
  const [query, setQuery] = useState('');

  useEffect(() => {
    let cancelled = false;
    loadCatalog()
      .then((controls) => !cancelled && setCatalog(controls))
      .catch(() => !cancelled && setFailed(true));
    return () => {
      cancelled = true;
    };
  }, []);

  const titles = useMemo(() => new Map(catalog.map((c) => [c.id, c.clause_es])), [catalog]);
  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return catalog;
    return catalog.filter((c) => `${c.id} ${c.clause_es}`.toLowerCase().includes(q));
  }, [catalog, query]);

  const toggle = (id: string) =>
    onChange(value.includes(id) ? value.filter((c) => c !== id) : [...value, id]);

  return (
    <div className="space-y-2">
      {value.length > 0 && (
        <ul className="flex flex-wrap gap-1.5" aria-label="Controles seleccionados">
          {value.map((id) => (
            <li key={id}>
              <button
                type="button"
                disabled={disabled}
                onClick={() => toggle(id)}
                title={titles.get(id)}
                aria-label={`Quitar ${id}`}
                className="badge gap-1 border bg-secondary font-mono hover:bg-accent disabled:opacity-60"
              >
                {id}
                {!disabled && <span aria-hidden="true">×</span>}
              </button>
            </li>
          ))}
        </ul>
      )}

      {!disabled && (
        <>
          <input
            type="search"
            className="input"
            placeholder="Buscar control por número o nombre…"
            aria-label="Buscar control"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <div className="max-h-44 overflow-y-auto rounded-md border" role="group" aria-label="Controles del Anexo A">
            {failed && (
              <p className="p-3 text-sm text-muted-foreground">No se pudo cargar el catálogo de controles.</p>
            )}
            {matches.map((c) => (
              <label key={c.id} className="flex cursor-pointer items-start gap-2 px-3 py-1.5 text-sm hover:bg-muted/50">
                <input
                  type="checkbox"
                  className="mt-0.5 h-4 w-4 shrink-0"
                  checked={value.includes(c.id)}
                  onChange={() => toggle(c.id)}
                />
                <span>
                  <span className="font-mono text-xs">{c.id}</span> {c.clause_es}
                </span>
              </label>
            ))}
            {!failed && catalog.length > 0 && matches.length === 0 && (
              <p className="p-3 text-sm text-muted-foreground">Ningún control coincide.</p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
