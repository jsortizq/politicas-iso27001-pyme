'use client';

import Link from 'next/link';
import { assetsApi, documentsApi, evidenceApi, risksApi, soaApi, type RiskRating } from '@/lib/api';
import { useResource } from '@/lib/useResource';
import { EmptyState, ErrorBanner, PageHeader, ProgressBar, Spinner } from '@/components/ui/feedback';

const loadOverview = async () => {
  const [summary, risks, assets, documents, evidence] = await Promise.all([
    soaApi.summary(),
    risksApi.list(),
    assetsApi.list(),
    documentsApi.list(),
    evidenceApi.list(),
  ]);
  return {
    summary,
    risks: risks.data,
    assets: assets.data,
    documents: documents.data,
    evidence: evidence.data,
  };
};

const ratingLabels: Record<RiskRating, string> = {
  critical: 'Críticos',
  high: 'Altos',
  medium: 'Medios',
  low: 'Bajos',
};

const ratingColors: Record<RiskRating, string> = {
  critical: 'bg-red-600',
  high: 'bg-orange-500',
  medium: 'bg-amber-400',
  low: 'bg-green-500',
};

export default function DashboardPage() {
  const { data, error, loading, reload } = useResource(loadOverview);

  if (loading) return <Spinner />;
  if (error || !data) return <ErrorBanner message={error ?? 'Sin datos.'} onRetry={reload} />;

  const { summary, risks, assets, documents, evidence } = data;
  const openRisks = risks.filter((r) => r.status !== 'closed');
  const byRating = (rating: RiskRating) => openRisks.filter((r) => r.risk_rating === rating).length;
  const criticalAssets = assets.filter((a) => a.status === 'active' && a.criticality >= 4).length;

  const activeDocuments = documents.filter((d) => d.status !== 'obsolete');
  const approvedDocuments = activeDocuments.filter((d) => d.status === 'approved').length;
  const overdueReviews = activeDocuments.filter((d) => d.review_overdue).length;
  const expiredEvidence = evidence.filter((e) => e.expired).length;

  const cards = [
    {
      title: 'Cumplimiento',
      value: `${summary.compliance_score_pct.toFixed(1)}%`,
      sub: 'Promedio de implementación y efectividad',
      href: '/dashboard/soa',
    },
    {
      title: 'Controles implementados',
      value: `${summary.implemented} / ${summary.applicable}`,
      sub: `${summary.partial} parciales · ${summary.not_implemented} sin iniciar`,
      href: '/dashboard/soa',
    },
    {
      title: 'Riesgos abiertos',
      value: openRisks.length,
      sub: `${byRating('critical')} críticos · ${byRating('high')} altos`,
      href: '/dashboard/risks',
    },
    {
      title: 'Activos',
      value: assets.length,
      sub: `${criticalAssets} de criticidad alta`,
      href: '/dashboard/assets',
    },
    {
      title: 'Documentos aprobados',
      value: `${approvedDocuments} / ${activeDocuments.length}`,
      sub: overdueReviews > 0 ? `${overdueReviews} con revisión vencida` : 'Sin revisiones vencidas',
      href: '/dashboard/documents',
    },
    {
      title: 'Evidencias',
      value: evidence.length,
      sub: expiredEvidence > 0 ? `${expiredEvidence} vencidas` : 'Ninguna vencida',
      href: '/dashboard/evidence',
    },
  ];

  return (
    <div className="space-y-8">
      <PageHeader
        title="Dashboard SGSI"
        subtitle="Estado general del Sistema de Gestión de Seguridad de la Información"
      />

      {summary.total_controls === 0 && (
        <EmptyState
          title="Aún no hay controles evaluados"
          action={
            <Link href="/dashboard/soa" className="btn btn-primary">
              Ir a Controles SoA
            </Link>
          }
        >
          Carga los 93 controles del Anexo A de ISO/IEC 27001:2022 para empezar tu Declaración de
          Aplicabilidad.
        </EmptyState>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {cards.map((card) => (
          <Link
            key={card.title}
            href={card.href}
            className="space-y-2 rounded-lg border p-6 transition-colors hover:bg-accent/50"
          >
            <p className="text-sm font-medium text-muted-foreground">{card.title}</p>
            <p className="text-3xl font-bold tabular-nums">{card.value}</p>
            <p className="text-xs text-muted-foreground">{card.sub}</p>
          </Link>
        ))}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <section className="space-y-5 rounded-lg border p-6">
          <h2 className="font-semibold">Controles aplicables</h2>
          {[
            { label: 'Implementación promedio', value: summary.avg_implementation_pct },
            { label: 'Efectividad promedio', value: summary.avg_effectiveness_pct },
          ].map((row) => (
            <div key={row.label} className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>{row.label}</span>
                <span className="tabular-nums text-muted-foreground">{row.value.toFixed(1)}%</span>
              </div>
              <ProgressBar value={row.value} />
            </div>
          ))}
          <p className="text-xs text-muted-foreground">
            {summary.applicable} aplicables · {summary.not_applicable} no aplicables ·{' '}
            {summary.total_controls} en total
          </p>
        </section>

        <section className="space-y-4 rounded-lg border p-6">
          <h2 className="font-semibold">Riesgos abiertos por nivel</h2>
          {openRisks.length === 0 ? (
            <p className="text-sm text-muted-foreground">No hay riesgos abiertos registrados.</p>
          ) : (
            <ul className="space-y-3">
              {(Object.keys(ratingLabels) as RiskRating[]).map((rating) => {
                const count = byRating(rating);
                return (
                  <li key={rating} className="flex items-center gap-3 text-sm">
                    <span className="w-16">{ratingLabels[rating]}</span>
                    <div className="h-2 flex-1 rounded-full bg-secondary">
                      <div
                        className={`${ratingColors[rating]} h-2 rounded-full`}
                        style={{ width: `${(count / openRisks.length) * 100}%` }}
                      />
                    </div>
                    <span className="w-6 text-right tabular-nums">{count}</span>
                  </li>
                );
              })}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}
