'use client';

import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ApiError, authApi, type Organization, type User } from './api';

interface SessionValue {
  user: User;
  /** Organizations this person belongs to, including the active one. */
  organizations: Organization[];
  organizationName: string;
  canCreateOrganization: boolean;
  /** owner, admin, editor: may change SGSI records. */
  canWrite: boolean;
  /** owner, admin: may invite, change and remove members. */
  canManageMembers: boolean;
  /** owner, admin, auditor: may see members and the activity log. */
  canViewAudit: boolean;
  logout: () => Promise<void>;
  switchOrganization: (tenantId: string) => Promise<void>;
}

const SessionContext = createContext<SessionValue | null>(null);

/** Session of the signed-in member. Only usable below <SessionProvider>. */
export function useSession(): SessionValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error('useSession must be used inside SessionProvider');
  return value;
}

/**
 * Loads the session before rendering its children. Visitors without a
 * session go to /login; people who have not joined or created an
 * organization go to /onboarding. Nothing below renders for either.
 */
export function SessionProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [value, setValue] = useState<SessionValue | null>(null);
  const [problem, setProblem] = useState<string | null>(null);

  const logout = useCallback(async () => {
    await authApi.logout().catch(() => undefined);
    router.replace('/login');
  }, [router]);

  const switchOrganization = useCallback(async (tenantId: string) => {
    await authApi.switchOrganization(tenantId);
    // Full reload: every page must refetch its data for the new organization.
    window.location.assign('/dashboard');
  }, []);

  useEffect(() => {
    let cancelled = false;
    authApi
      .session()
      .then((s) => {
        if (cancelled) return;
        if (!s.user) {
          router.replace('/onboarding');
          return;
        }
        const role = s.user.role;
        setValue({
          user: s.user,
          organizations: s.organizations,
          organizationName:
            s.organizations.find((o) => o.tenant_id === s.user!.tenant_id)?.tenant_name ?? '',
          canCreateOrganization: s.can_create_organization,
          canWrite: ['owner', 'admin', 'editor'].includes(role),
          canManageMembers: ['owner', 'admin'].includes(role),
          canViewAudit: ['owner', 'admin', 'auditor'].includes(role),
          logout,
          switchOrganization,
        });
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) router.replace('/login');
        else setProblem('No se pudo verificar tu sesión. Comprueba que el servidor esté en marcha.');
      });
    return () => {
      cancelled = true;
    };
  }, [router, logout, switchOrganization]);

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

  if (!value) {
    return (
      <div className="flex min-h-screen items-center justify-center" role="status" aria-label="Cargando">
        <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-primary" />
      </div>
    );
  }

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}
