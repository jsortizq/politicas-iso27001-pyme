'use client';

import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ApiError, authApi, canWrite, type User } from './api';

interface SessionValue {
  user: User;
  canWrite: boolean;
  logout: () => Promise<void>;
}

const SessionContext = createContext<SessionValue | null>(null);

/** Session of the signed-in user. Only usable below <SessionProvider>. */
export function useSession(): SessionValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error('useSession must be used inside SessionProvider');
  return value;
}

/**
 * Loads the session before rendering its children. Visitors without a
 * valid session are sent to /login; nothing below renders for them.
 */
export function SessionProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [problem, setProblem] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    authApi
      .session()
      .then((s) => !cancelled && setUser(s.user))
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) router.replace('/login');
        else setProblem('No se pudo verificar tu sesión. Comprueba que el servidor esté en marcha.');
      });
    return () => {
      cancelled = true;
    };
  }, [router]);

  const logout = useCallback(async () => {
    await authApi.logout().catch(() => undefined);
    router.replace('/login');
  }, [router]);

  if (problem) {
    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <div className="max-w-sm space-y-4 text-center">
          <p className="text-sm text-muted-foreground">{problem}</p>
          <button className="btn btn-outline" onClick={() => window.location.reload()}>
            Reintentar
          </button>
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="flex min-h-screen items-center justify-center" role="status" aria-label="Cargando">
        <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-primary" />
      </div>
    );
  }

  return (
    <SessionContext.Provider value={{ user, canWrite: canWrite(user), logout }}>
      {children}
    </SessionContext.Provider>
  );
}
