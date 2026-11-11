'use client';

import { useEffect, useState } from 'react';
import type { SoAEntry } from '@/lib/api';

// Demo data matching the 93 Annex A controls structure
const demoEntries: SoAEntry[] = [
  { id: '1', tenant_id: 'demo', control_id: 'A.5.1', control_title: 'Políticas de seguridad de la información', applicable: true, justification: 'Marco normativo requerido', implementation_pct: 100, implementation_evidence: 'Política marco publicada', effectiveness_pct: 85, effectiveness_evidence: 'Revisión anual completada', responsible_role: 'CISO', created_at: '', updated_at: '' },
  { id: '2', tenant_id: 'demo', control_id: 'A.5.2', control_title: 'Roles y responsabilidades', applicable: true, justification: 'Organización de seguridad', implementation_pct: 80, implementation_evidence: 'Roles definidos en organigrama', effectiveness_pct: 70, effectiveness_evidence: 'Última revisión hace 6 meses', responsible_role: 'CISO', created_at: '', updated_at: '' },
  { id: '3', tenant_id: 'demo', control_id: 'A.5.3', control_title: 'Segregación de funciones', applicable: true, justification: 'Prevención de fraude', implementation_pct: 60, implementation_evidence: 'Parcialmente documentado', effectiveness_pct: 50, effectiveness_evidence: 'Pendiente de auditoría', responsible_role: 'HR', created_at: '', updated_at: '' },
  { id: '4', tenant_id: 'demo', control_id: 'A.6.1', control_title: 'Verificación de antecedentes', applicable: true, justification: 'Requisito contractual', implementation_pct: 100, implementation_evidence: 'Proceso HR establecido', effectiveness_pct: 90, effectiveness_evidence: 'Auditoría Q3 conforme', responsible_role: 'HR', created_at: '', updated_at: '' },
  { id: '5', tenant_id: 'demo', control_id: 'A.7.1', control_title: 'Perímetro de seguridad física', applicable: false, justification: 'Empresa 100% remota', implementation_pct: 0, implementation_evidence: '', effectiveness_pct: 0, effectiveness_evidence: '', responsible_role: 'N/A', created_at: '', updated_at: '' },
  { id: '6', tenant_id: 'demo', control_id: 'A.8.1', control_title: 'Dispositivos de usuario', applicable: true, justification: 'Control de endpoints', implementation_pct: 75, implementation_evidence: 'MDM desplegado en 75% de equipos', effectiveness_pct: 60, effectiveness_evidence: 'Falta cobertura en BYOD', responsible_role: 'IT', created_at: '', updated_at: '' },
  { id: '7', tenant_id: 'demo', control_id: 'A.8.9', control_title: 'Gestión de configuración', applicable: true, justification: 'Hardening de servidores', implementation_pct: 40, implementation_evidence: 'Baselines definidos parcialmente', effectiveness_pct: 30, effectiveness_evidence: 'Sin validación automatizada', responsible_role: 'IT', created_at: '', updated_at: '' },
  { id: '8', tenant_id: 'demo', control_id: 'A.8.15', control_title: 'Registro de eventos', applicable: true, justification: 'Trazabilidad y detección', implementation_pct: 90, implementation_evidence: 'ELK stack en producción', effectiveness_pct: 80, effectiveness_evidence: 'Alertas configuradas', responsible_role: 'SecOps', created_at: '', updated_at: '' },
];

function StatusBadge({ pct }: { pct: number }) {
  if (pct >= 100) return <span className="badge-full inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">Completo</span>;
  if (pct >= 50) return <span className="badge-partial inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">Parcial</span>;
  if (pct > 0) return <span className="badge-none inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">Inicial</span>;
  return <span className="badge-na inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">N/A</span>;
}

function ProgressBar({ value, className = '' }: { value: number; className?: string }) {
  let color = 'bg-red-500';
  if (value >= 80) color = 'bg-green-500';
  else if (value >= 50) color = 'bg-amber-500';

  return (
    <div className={`w-full bg-secondary rounded-full h-2 ${className}`}>
      <div className={`${color} h-2 rounded-full transition-all`} style={{ width: `${Math.min(value, 100)}%` }} />
    </div>
  );
}

export default function SoAPage() {
  const [entries, setEntries] = useState<SoAEntry[]>([]);
  const [filter, setFilter] = useState<'all' | 'applicable' | 'not_applicable'>('all');

  useEffect(() => {
    // TODO: Replace with API call when backend is connected
    setEntries(demoEntries);
  }, []);

  const filtered = entries.filter((e) => {
    if (filter === 'applicable') return e.applicable;
    if (filter === 'not_applicable') return !e.applicable;
    return true;
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Statement of Applicability</h1>
          <p className="text-muted-foreground">
            Evaluación de controles ISO 27001:2022 Annex A
          </p>
        </div>
        <div className="flex gap-2">
          {(['all', 'applicable', 'not_applicable'] as const).map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`px-3 py-1.5 text-sm rounded-md transition-colors ${
                filter === f
                  ? 'bg-primary text-primary-foreground'
                  : 'border hover:bg-accent'
              }`}
            >
              {f === 'all' ? 'Todos' : f === 'applicable' ? 'Aplicables' : 'No Aplicables'}
            </button>
          ))}
        </div>
      </div>

      {/* Table */}
      <div className="rounded-lg border overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="text-left px-4 py-3 font-medium">Control</th>
                <th className="text-left px-4 py-3 font-medium hidden md:table-cell">Título</th>
                <th className="text-center px-4 py-3 font-medium">Estado</th>
                <th className="text-center px-4 py-3 font-medium">Impl.</th>
                <th className="text-center px-4 py-3 font-medium hidden sm:table-cell">Efect.</th>
                <th className="text-left px-4 py-3 font-medium hidden lg:table-cell">Responsable</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((entry) => (
                <tr key={entry.id} className="border-b hover:bg-muted/30 transition-colors">
                  <td className="px-4 py-3 font-mono text-xs">{entry.control_id}</td>
                  <td className="px-4 py-3 hidden md:table-cell">
                    <span className="line-clamp-1">{entry.control_title}</span>
                  </td>
                  <td className="px-4 py-3 text-center">
                    <StatusBadge pct={entry.applicable ? entry.implementation_pct : -1} />
                  </td>
                  <td className="px-4 py-3">
                    {entry.applicable ? (
                      <div className="flex items-center gap-2">
                        <ProgressBar value={entry.implementation_pct} />
                        <span className="text-xs text-muted-foreground w-8 text-right">{entry.implementation_pct}%</span>
                      </div>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 hidden sm:table-cell">
                    {entry.applicable ? (
                      <div className="flex items-center gap-2">
                        <ProgressBar value={entry.effectiveness_pct} />
                        <span className="text-xs text-muted-foreground w-8 text-right">{entry.effectiveness_pct}%</span>
                      </div>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-xs hidden lg:table-cell">{entry.responsible_role}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        Mostrando {filtered.length} de {entries.length} controles • Datos de demostración
      </p>
    </div>
  );
}
