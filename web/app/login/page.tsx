'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';

export default function LoginPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  const handleLogin = async () => {
    setLoading(true);
    // TODO: Replace with real OIDC flow (ISO-012)
    // For now, simulate login and redirect to dashboard
    await new Promise((resolve) => setTimeout(resolve, 500));
    router.push('/dashboard');
  };

  return (
    <main className="flex min-h-screen items-center justify-center p-8">
      <div className="w-full max-w-md space-y-8">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold">Iniciar Sesión</h1>
          <p className="text-muted-foreground">
            Accede a tu sistema SGSI
          </p>
        </div>

        <div className="rounded-lg border p-6 space-y-6">
          {/* OIDC login button — will connect to real provider in ISO-012 */}
          <button
            onClick={handleLogin}
            disabled={loading}
            className="w-full inline-flex items-center justify-center rounded-md bg-primary px-4 py-3 text-sm font-medium text-primary-foreground shadow hover:bg-primary/90 disabled:opacity-50 transition-colors"
          >
            {loading ? (
              <span className="flex items-center gap-2">
                <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Conectando...
              </span>
            ) : (
              'Iniciar sesión con SSO'
            )}
          </button>

          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <span className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-background px-2 text-muted-foreground">
                O continuar con
              </span>
            </div>
          </div>

          <div className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="email" className="text-sm font-medium">
                Email
              </label>
              <input
                id="email"
                type="email"
                placeholder="admin@empresa.com"
                className="flex h-10 w-full rounded-md border border-border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="password" className="text-sm font-medium">
                Contraseña
              </label>
              <input
                id="password"
                type="password"
                placeholder="••••••••"
                className="flex h-10 w-full rounded-md border border-border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>
            <button
              onClick={handleLogin}
              disabled={loading}
              className="w-full inline-flex items-center justify-center rounded-md border border-border px-4 py-2 text-sm font-medium shadow-sm hover:bg-accent disabled:opacity-50 transition-colors"
            >
              Acceder
            </button>
          </div>
        </div>

        <p className="text-center text-xs text-muted-foreground">
          Plataforma protegida. El acceso no autorizado está prohibido.
        </p>
      </div>
    </main>
  );
}
