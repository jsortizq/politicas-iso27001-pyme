'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { ApiError, authApi } from '@/lib/api';

// Development organization. Real organizations will come from the identity
// provider once the OIDC flow is implemented.
const DEV_TENANT_ID =
  process.env.NEXT_PUBLIC_DEV_TENANT_ID || '00000000-0000-4000-8000-000000000001';

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await authApi.login(email, password, DEV_TENANT_ID);
      router.replace('/dashboard');
    } catch (err) {
      if (err instanceof ApiError && err.status === 501) {
        setError(
          'El inicio de sesión con correo solo está disponible en modo desarrollo (AUTH_DEV_MODE=true en la API). El acceso con SSO aún no está implementado.',
        );
      } else if (err instanceof ApiError && err.status === 0) {
        setError('No se pudo conectar con el servidor. Comprueba que la API esté en marcha.');
      } else {
        setError('No se pudo iniciar sesión. Revisa tus datos e intenta de nuevo.');
      }
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

        <form onSubmit={handleSubmit} className="space-y-4 rounded-lg border p-6">
          {error && (
            <p role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
              {error}
            </p>
          )}
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
          <button type="submit" disabled={loading} className="btn btn-primary w-full">
            {loading ? 'Entrando…' : 'Entrar'}
          </button>
          <p className="text-xs text-muted-foreground">
            Modo desarrollo: se acepta cualquier correo y contraseña. El acceso con SSO (OIDC)
            llegará en una fase posterior.
          </p>
        </form>

        <p className="text-center text-xs text-muted-foreground">
          Plataforma protegida. El acceso no autorizado está prohibido.
        </p>
      </div>
    </main>
  );
}
