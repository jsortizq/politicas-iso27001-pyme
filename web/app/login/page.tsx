'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ApiError, authApi, SSO_LOGIN_URL, type AuthConfig } from '@/lib/api';

// Organization used by the development login.
const DEV_TENANT_ID =
  process.env.NEXT_PUBLIC_DEV_TENANT_ID || '00000000-0000-4000-8000-000000000001';

const ssoErrors: Record<string, string> = {
  sso_failed: 'No se pudo completar el inicio de sesión. Inténtalo de nuevo.',
  sso_denied: 'El proveedor de identidad canceló o denegó el acceso.',
  sso_unavailable: 'El proveedor de identidad no está disponible en este momento.',
};

export default function LoginPage() {
  const router = useRouter();
  const [config, setConfig] = useState<AuthConfig | null>(null);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const code = new URLSearchParams(window.location.search).get('error');
    if (code) setError(ssoErrors[code] ?? ssoErrors.sso_failed);

    authApi
      .config()
      .then(setConfig)
      .catch(() =>
        setError('No se pudo conectar con el servidor. Comprueba que la API esté en marcha.'),
      );
  }, []);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await authApi.devLogin(email, password, DEV_TENANT_ID);
      router.replace('/dashboard');
    } catch (err) {
      setError(
        err instanceof ApiError && err.status === 0
          ? 'No se pudo conectar con el servidor. Comprueba que la API esté en marcha.'
          : 'No se pudo iniciar sesión. Revisa tus datos e intenta de nuevo.',
      );
      setLoading(false);
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-md space-y-8">
        <div className="space-y-2 text-center">
          <h1 className="text-3xl font-bold">Iniciar sesión</h1>
          <p className="text-muted-foreground">Accede a tu sistema SGSI</p>
        </div>

        <div className="space-y-5 rounded-lg border p-6">
          {error && (
            <p role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
              {error}
            </p>
          )}

          {config?.sso && (
            // A real navigation, not fetch: the browser must follow the
            // redirects to the identity provider and back.
            <a href={SSO_LOGIN_URL} className="btn btn-primary w-full">
              Continuar con SSO
            </a>
          )}

          {config?.sso && config.dev_login && (
            <div className="flex items-center gap-3 text-xs uppercase text-muted-foreground">
              <span className="h-px flex-1 bg-border" />
              o en modo desarrollo
              <span className="h-px flex-1 bg-border" />
            </div>
          )}

          {config?.dev_login && (
            <form onSubmit={handleSubmit} className="space-y-4">
              <label className="block space-y-1.5">
                <span className="text-sm font-medium">Correo</span>
                <input
                  type="email"
                  required
                  autoComplete="username"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="admin@empresa.com"
                  className="input"
                />
              </label>
              <label className="block space-y-1.5">
                <span className="text-sm font-medium">Contraseña</span>
                <input
                  type="password"
                  required
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="input"
                />
              </label>
              <button
                type="submit"
                disabled={loading}
                className={`btn w-full ${config.sso ? 'btn-outline' : 'btn-primary'}`}
              >
                {loading ? 'Entrando…' : 'Entrar'}
              </button>
              <p className="text-xs text-muted-foreground">
                Modo desarrollo: se acepta cualquier correo y contraseña.
              </p>
            </form>
          )}

          {config && !config.sso && !config.dev_login && (
            <p className="text-sm text-muted-foreground">
              No hay ningún método de acceso configurado. Define <code>OIDC_ISSUER</code> para
              activar el SSO o, solo en tu equipo, <code>AUTH_DEV_MODE=true</code>.
            </p>
          )}

          {!config && !error && (
            <div className="flex justify-center py-4" role="status" aria-label="Cargando">
              <div className="h-6 w-6 animate-spin rounded-full border-b-2 border-primary" />
            </div>
          )}
        </div>

        <p className="text-center text-xs text-muted-foreground">
          Plataforma protegida. El acceso no autorizado está prohibido.
        </p>
      </div>
    </main>
  );
}
