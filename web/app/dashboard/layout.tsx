import Link from 'next/link';

const navigation = [
  { name: 'Dashboard', href: '/dashboard' },
  { name: 'Controles SoA', href: '/dashboard/soa' },
  { name: 'Riesgos', href: '/dashboard/risks' },
  { name: 'Documentos', href: '/dashboard/documents' },
  { name: 'Auditorías', href: '/dashboard/audits' },
];

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen">
      {/* Sidebar */}
      <aside className="hidden md:flex md:w-64 md:flex-col border-r">
        <div className="flex flex-col flex-grow pt-5 overflow-y-auto">
          <div className="flex items-center flex-shrink-0 px-4 pb-4 border-b">
            <Link href="/dashboard" className="text-lg font-bold">
              SGSI Platform
            </Link>
          </div>
          <nav className="flex-1 px-2 py-4 space-y-1">
            {navigation.map((item) => (
              <Link
                key={item.name}
                href={item.href}
                className="flex items-center px-3 py-2 text-sm font-medium rounded-md hover:bg-accent hover:text-accent-foreground transition-colors"
              >
                {item.name}
              </Link>
            ))}
          </nav>
          <div className="flex-shrink-0 p-4 border-t">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-full bg-primary/10 flex items-center justify-center text-sm font-medium text-primary">
                A
              </div>
              <div className="text-sm">
                <p className="font-medium">Admin</p>
                <p className="text-muted-foreground text-xs">admin@empresa.com</p>
              </div>
            </div>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-y-auto">
        {/* Top bar for mobile */}
        <header className="md:hidden flex items-center justify-between border-b px-4 py-3">
          <Link href="/dashboard" className="text-lg font-bold">
            SGSI
          </Link>
          <button className="p-2 rounded-md hover:bg-accent">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
            </svg>
          </button>
        </header>

        <div className="p-6 lg:p-8">
          {children}
        </div>
      </main>
    </div>
  );
}
