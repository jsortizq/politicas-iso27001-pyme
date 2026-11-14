'use client';

import { useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { roleLabels } from '@/lib/api';
import { SessionProvider, useSession } from '@/lib/session';

function Nav({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();
  const { canViewAudit } = useSession();

  const navigation = [
    { name: 'Dashboard', href: '/dashboard' },
    { name: 'Controles SoA', href: '/dashboard/soa' },
    { name: 'Activos', href: '/dashboard/assets' },
    { name: 'Riesgos', href: '/dashboard/risks' },
    { name: 'Documentos', href: '/dashboard/documents' },
    { name: 'Evidencias', href: '/dashboard/evidence' },
    { name: 'Auditorías', href: '/dashboard/audits' },
    // Members and the activity log are for owners, administrators and auditors.
    ...(canViewAudit
      ? [
          { name: 'Miembros', href: '/dashboard/members' },
          { name: 'Actividad', href: '/dashboard/activity' },
        ]
      : []),
  ];

  return (
    <nav className="flex-1 space-y-1 px-2 py-4" aria-label="Principal">
      {navigation.map((item) => {
        // Sub-pages (e.g. an audit's report) keep their section highlighted.
        const active =
          pathname === item.href || (item.href !== '/dashboard' && pathname.startsWith(`${item.href}/`));
        return (
          <Link
            key={item.href}
            href={item.href}
            onClick={onNavigate}
            aria-current={active ? 'page' : undefined}
            className={`flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors ${
              active ? 'bg-accent text-accent-foreground' : 'hover:bg-accent hover:text-accent-foreground'
            }`}
          >
            {item.name}
          </Link>
        );
      })}
    </nav>
  );
}

function OrganizationBox() {
  const { user, organizations, organizationName, switchOrganization } = useSession();
  const [switching, setSwitching] = useState(false);

  return (
    <div className="space-y-1 border-b px-4 py-3">
      <p className="text-xs text-muted-foreground">Organización</p>
      {organizations.length > 1 ? (
        <select
          className="input h-9"
          aria-label="Cambiar de organización"
          value={user.tenant_id}
          disabled={switching}
          onChange={(e) => {
            setSwitching(true);
            switchOrganization(e.target.value).catch(() => setSwitching(false));
          }}
        >
          {organizations.map((org) => (
            <option key={org.tenant_id} value={org.tenant_id}>
              {org.tenant_name}
            </option>
          ))}
        </select>
      ) : (
        <p className="truncate text-sm font-medium" title={organizationName}>
          {organizationName}
        </p>
      )}
      <Link href="/onboarding" className="block text-xs text-muted-foreground underline underline-offset-4">
        Otras organizaciones
      </Link>
    </div>
  );
}

function UserBox() {
  const { user, logout } = useSession();
  return (
    <div className="space-y-3 border-t p-4">
      <div className="flex items-center gap-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
          {(user.name || user.email).charAt(0).toUpperCase()}
        </div>
        <div className="min-w-0 text-sm">
          <p className="truncate font-medium">{user.name || user.email}</p>
          <p className="truncate text-xs text-muted-foreground">{roleLabels[user.role] ?? user.role}</p>
        </div>
      </div>
      <button className="btn btn-outline h-8 w-full" onClick={logout}>
        Cerrar sesión
      </button>
    </div>
  );
}

function Shell({ children }: { children: React.ReactNode }) {
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="flex min-h-screen">
      <aside className="hidden border-r md:flex md:w-64 md:flex-col print:!hidden">
        <Link href="/dashboard" className="border-b px-4 py-5 text-lg font-bold">
          SGSI Platform
        </Link>
        <OrganizationBox />
        <Nav />
        <UserBox />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b px-4 py-3 md:hidden print:hidden">
          <Link href="/dashboard" className="text-lg font-bold">
            SGSI
          </Link>
          <button
            className="btn btn-outline h-9 px-3"
            aria-expanded={menuOpen}
            aria-controls="mobile-menu"
            onClick={() => setMenuOpen((open) => !open)}
          >
            {menuOpen ? 'Cerrar' : 'Menú'}
          </button>
        </header>
        {menuOpen && (
          <div id="mobile-menu" className="border-b md:hidden print:hidden">
            <OrganizationBox />
            <Nav onNavigate={() => setMenuOpen(false)} />
            <UserBox />
          </div>
        )}

        <main className="min-w-0 flex-1 p-4 sm:p-6 lg:p-8">{children}</main>
      </div>
    </div>
  );
}

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <SessionProvider>
      <Shell>{children}</Shell>
    </SessionProvider>
  );
}
