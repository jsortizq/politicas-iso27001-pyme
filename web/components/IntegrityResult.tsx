import type { Integrity } from '@/lib/api';

export type IntegrityState = Integrity | 'checking' | 'error' | null;

/** Shows the outcome of re-hashing a stored file against its recorded SHA-256. */
export function IntegrityResult({ result }: { result: IntegrityState }) {
  if (!result || result === 'checking') return null;
  if (result === 'error') {
    return <p role="status" className="text-sm text-red-700 dark:text-red-400">No se pudo comprobar el archivo.</p>;
  }
  if (result.intact) {
    return (
      <p role="status" className="text-sm text-green-700 dark:text-green-400">
        Íntegro: el archivo almacenado coincide con la huella registrada al subirlo.
      </p>
    );
  }
  return (
    <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
      {result.missing
        ? 'El archivo no está en el almacenamiento.'
        : 'El archivo almacenado NO coincide con la huella registrada: fue alterado o está dañado.'}
    </p>
  );
}
