'use client';

import { useEffect, useState } from 'react';

interface SoASummary {
  total_controls: number;
  applicable: number;
  not_applicable: number;
  implemented: number;
  partial: number;
  not_implemented: number;
  avg_implementation_pct: number;
  avg_effectiveness_pct: number;
  compliance_score_pct: number;
}

function StatusBadge({ value, label }: { value: number; label: string }) {
  let colorClass = 'badge-na';
  if (value >= 80) colorClass = 'badge-full';
  else if (value >= 40) colorClass = 'badge-partial';
  else if (value > 0) colorClass = 'badge-none';

  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colorClass}`}>
      {label}: {value.toFixed(1)}%
    </span>
  );
}

export default function DashboardPage() {
  const [summary, setSummary] = useState<SoASummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchSummary() {
      try {
        const res = await fetch('/api/v1/soa/summary', {
          headers: { 'X-Tenant-ID': 'demo-tenant-00000000-0000-0000-0000-000000000000' },
        });
        if (!res.ok) throw new Error(`API error: ${res.status}`);
        const data = await res.json();
        setSummary(data);
      } catch (err) {
        setError('No se pudo conectar al API. Mostrando datos de ejemplo.');
        // Demo data when API is not available
        setSummary({
          total_controls: 93,
          applicable: 87,
          not_applicable: 6,
          implemented: 14,
          partial: 45,
          not_implemented: 28,
          avg_implementation_pct: 42.5,
          avg_effectiveness_pct: 35.2,
          compliance_score_pct: 38.85,
        });
      } finally {
        setLoading(false);
      }
    }
    fetchSummary();
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  if (!summary) return null;

  const cards = [
    { title: 'Controles Totales', value: summary.total_controls, sub: `${summary.applicable} aplicables` },
    { title: 'Implementados', value: summary.implemented, sub: `${summary.partial} parciales` },
    { title: 'Sin Implementar', value: summary.not_implemented, sub: `${summary.not_applicable} N/A` },
    { title: 'Score Cumplimiento', value: `${summary.compliance_score_pct.toFixed(1)}%`, sub: 'Promedio general' },
  ];

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Dashboard SGSI</h1>
        <p className="text-muted-foreground">
          Estado general del Sistema de Gestión de Seguridad de la Información
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-amber-200 bg-amber-50 dark:bg-amber-950/20 dark:border-amber-800 p-4">
          <p className="text-sm text-amber-800 dark:text-amber-400">{error}</p>
        </div>
      )}

      {/* KPI Cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {cards.map((card) => (
          <div key={card.title} className="rounded-lg border p-6 space-y-2">
            <p className="text-sm font-medium text-muted-foreground">{card.title}</p>
            <p className="text-3xl font-bold">{card.value}</p>
            <p className="text-xs text-muted-foreground">{card.sub}</p>
          </div>
        ))}
      </div>

      {/* Progress bars */}
      <div className="grid gap-6 md:grid-cols-2">
        <div className="rounded-lg border p-6 space-y-4">
          <h3 className="font-semibold">Implementación</h3>
          <div className="space-y-3">
            <div className="flex justify-between text-sm">
              <span>Progreso general</span>
              <StatusBadge value={summary.avg_implementation_pct} label="Impl" />
            </div>
            <div className="w-full bg-secondary rounded-full h-3">
              <div
                className="bg-primary h-3 rounded-full transition-all duration-500"
                style={{ width: `${Math.min(summary.avg_implementation_pct, 100)}%` }}
              />
            </div>
          </div>
        </div>

        <div className="rounded-lg border p-6 space-y-4">
          <h3 className="font-semibold">Efectividad</h3>
          <div className="space-y-3">
            <div className="flex justify-between text-sm">
              <span>Efectividad promedio</span>
              <StatusBadge value={summary.avg_effectiveness_pct} label="Efect" />
            </div>
            <div className="w-full bg-secondary rounded-full h-3">
              <div
                className="bg-green-500 h-3 rounded-full transition-all duration-500"
                style={{ width: `${Math.min(summary.avg_effectiveness_pct, 100)}%` }}
              />
            </div>
          </div>
        </div>
      </div>

      {/* Distribution chart placeholder */}
      <div className="rounded-lg border p-6 space-y-4">
        <h3 className="font-semibold">Distribución de Controles</h3>
        <div className="flex gap-2 h-8">
          {summary.applicable > 0 && (
            <>
              <div
                className="bg-green-500 rounded-l transition-all"
                style={{ width: `${(summary.implemented / summary.applicable) * 100}%` }}
                title={`Implementados: ${summary.implemented}`}
              />
              <div
                className="bg-amber-500 transition-all"
                style={{ width: `${(summary.partial / summary.applicable) * 100}%` }}
                title={`Parciales: ${summary.partial}`}
              />
              <div
                className="bg-red-500 rounded-r transition-all"
                style={{ width: `${(summary.not_implemented / summary.applicable) * 100}%` }}
                title={`Sin implementar: ${summary.not_implemented}`}
              />
            </>
          )}
        </div>
        <div className="flex gap-6 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <span className="w-3 h-3 rounded bg-green-500" /> Implementados ({summary.implemented})
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-3 h-3 rounded bg-amber-500" /> Parciales ({summary.partial})
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-3 h-3 rounded bg-red-500" /> Sin implementar ({summary.not_implemented})
          </span>
        </div>
      </div>
    </div>
  );
}
