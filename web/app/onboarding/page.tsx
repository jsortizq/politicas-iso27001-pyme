'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ApiError, authApi, errorMessage, roleLabels, type SessionInfo } from '@/lib/api';

/**
 * Shown to signed-in people who are not working in an organization yet:
 * right after their first sign-in, or after being removed from one. They
 * can enter one they belong to or create a new one.
 */
export default function OnboardingPage() {
  const router = useRouter();
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    authApi
      .session()
      .then(setSession)
      .catch((err) => {
        if (err instanceof ApiError && err.status === 401) router.replace('/login');
        else setError(errorMessage(err));
      });
  }, [router]);

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await action();
      window.location.assign('/dashboard');
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  };

  const logout = async () => {
    await authApi.logout().catch(() => undefined);
    router.replace('/login');
  };

  if (!session) {
    return (
      <main className="flex min-h-screen items-center justify-center p-6">
        {error ? (
          <p role="alert" className="text-sm text-red-700 dark:text-red-400">{error}</p>
        ) : (
          <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-primary" role="status" aria-label="Cargando" />
        )}
      </main>
    );
  }

  const others = session.organizations.filter((o) => o.tenant_id !== session.user?.tenant_id);

  return (
    <main className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-md space-y-6">
        <div className="space-y-2 text-center">
          <h1 className="text-3xl font-bold">
            {session.user ? 'Organizaciones' : 'Te damos la bienvenida'}
          </h1>
          <p className="text-muted-foreground">
            Has iniciado sesión como <span className="font-medium">{session.identity.email}</span>
          </p>
        </div>

        {error && (
          <p role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
            {error}
          </p>
        )}

        {others.length > 0 && (
          <section className="space-y-3 rounded-lg border p-6">
            <h2 className="font-semibold">Entrar en una organización</h2>
            <ul className="space-y-2">
              {others.map((org) => (
                <li key={org.tenant_id}>
                  <button
                    className="btn btn-outline h-auto w-full justify-between py-3 text-left"
                    disabled={busy}
                    onClick={() => run(() => authApi.switchOrganization(org.tenant_id))}
                  >
                    <span className="font-medium">{org.tenant_name}</span>
                    <span className="text-xs text-muted-foreground">{roleLabels[org.role]}</span>
                  </button>
                </li>
              ))}
            </ul>
          </section>
        )}

        {session.can_create_organization ? (
          <form
            className="space-y-4 rounded-lg border p-6"
            onSubmit={(event) => {
              event.preventDefault();
              run(() => authApi.createOrganization(name));
            }}
          >
            <h2 className="font-semibold">Crear una organización</h2>
            <label className="block space-y-1.5">
              <span className="text-sm font-medium">Nombre de la empresa</span>
              <input
                className="input"
                required
                minLength={2}
                maxLength={120}
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="p. ej. Ferretería El Tornillo S.A.S."
              />
            </label>
            <button type="submit" className="btn btn-primary w-full" disabled={busy}>
              {busy ? 'Creando…' : 'Crear y empezar'}
            </button>
            <p className="text-xs text-muted-foreground">
              Serás el propietario: podrás invitar a tu equipo y asignar sus roles.
            </p>
          </form>
        ) : (
          others.length === 0 && (
            <p className="rounded-lg border p-6 text-sm text-muted-foreground">
              Todavía no perteneces a ninguna organización. Pide a un administrador que te invite
              con este correo y vuelve a iniciar sesión.
            </p>
          )
        )}

        <div className="flex items-center justify-between text-sm">
          {session.user ? (
            <Link href="/dashboard" className="underline underline-offset-4">
              Volver al panel
            </Link>
          ) : (
            <span />
          )}
          <button className="underline underline-offset-4" onClick={logout}>
            Cerrar sesión
          </button>
        </div>
      </div>
    </main>
  );
}
