'use client';

import { useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { SessionProvider, useSession } from '@/lib/session';

const navigation = [
  { name: 'Dashboard', href: '/dashboard' },
  { name: 'Controles SoA', href: '/dashboard/soa' },
  { name: 'Activos', href: '/dashboard/assets' },
  { name: 'Riesgos', href: '/dashboard/risks' },
];

// Planned modules, shown so the roadmap is visible but not clickable.
const upcoming = ['Documentos', 'Auditorías'];

const roleLabels: Record<string, string> = {
  owner: 'Propietario',
  admin: 'Administrador',
  editor: 'Editor',
  auditor: 'Auditor (solo lectura)',
  viewer: 'Lector (solo lectura)',
};

function Nav({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();
  return (
    <nav className="flex-1 space-y-1 px-2 py-4" aria-label="Principal">
      {navigation.map((item) => {
        const active = pathname === item.href;
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
      {upcoming.map((name) => (
        <span
          key={name}
          className="flex items-center justify-between px-3 py-2 text-sm text-muted-foreground"
        >
          {name}
          <span className="text-xs">Próximamente</span>
        </span>
      ))}
    </nav>
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
      <aside className="hidden border-r md:flex md:w-64 md:flex-col">
        <Link href="/dashboard" className="border-b px-4 py-5 text-lg font-bold">
          SGSI Platform
        </Link>
        <Nav />
        <UserBox />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b px-4 py-3 md:hidden">
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
          <div id="mobile-menu" className="border-b md:hidden">
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
