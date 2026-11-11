import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'SGSI Platform — ISO 27001',
  description: 'Sistema de Gestión de Seguridad de la Información basado en ISO/IEC 27001:2022',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="es" suppressHydrationWarning>
      <body className="min-h-screen bg-background antialiased">
        <div className="relative flex min-h-screen flex-col">
          {children}
        </div>
      </body>
    </html>
  );
}
