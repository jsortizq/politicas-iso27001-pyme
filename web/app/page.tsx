import Link from 'next/link';

export default function HomePage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-8">
      <div className="max-w-2xl text-center space-y-8">
        <div className="space-y-4">
          <h1 className="text-4xl font-bold tracking-tight sm:text-5xl">
            SGSI Platform
          </h1>
          <p className="text-xl text-muted-foreground">
            Sistema de Gestión de Seguridad de la Información
          </p>
          <p className="text-sm text-muted-foreground">
            ISO/IEC 27001:2022 + Amendment 1:2024
          </p>
        </div>

        <div className="flex flex-col sm:flex-row gap-4 justify-center">
          <Link
            href="/login"
            className="inline-flex items-center justify-center rounded-md bg-primary px-8 py-3 text-sm font-medium text-primary-foreground shadow hover:bg-primary/90 transition-colors"
          >
            Iniciar Sesión
          </Link>
        </div>

        <div className="pt-8 grid grid-cols-1 sm:grid-cols-3 gap-6 text-left">
          <div className="rounded-lg border p-4 space-y-2">
            <h3 className="font-semibold">93 Controles Annex A</h3>
            <p className="text-sm text-muted-foreground">
              Catálogo completo de controles ISO 27001:2022 con evaluación de tres dimensiones.
            </p>
          </div>
          <div className="rounded-lg border p-4 space-y-2">
            <h3 className="font-semibold">Multi-Tenant</h3>
            <p className="text-sm text-muted-foreground">
              Aislamiento por organización con Row-Level Security en PostgreSQL.
            </p>
          </div>
          <div className="rounded-lg border p-4 space-y-2">
            <h3 className="font-semibold">AI Copilot</h3>
            <p className="text-sm text-muted-foreground">
              Asistente inteligente para análisis de brechas y recomendaciones de controles.
            </p>
          </div>
        </div>
      </div>
    </main>
  );
}
