/** Formatting helpers shared by the pages. */

const dateOnly = new Intl.DateTimeFormat('es', { dateStyle: 'medium' });
const dateTime = new Intl.DateTimeFormat('es', { dateStyle: 'medium', timeStyle: 'short' });

/**
 * Formats a calendar date ("2026-11-12"). It is built from its parts so the
 * day never shifts with the viewer's time zone.
 */
export function formatDay(day?: string): string {
  if (!day) return '—';
  const [y, m, d] = day.slice(0, 10).split('-').map(Number);
  return dateOnly.format(new Date(y, m - 1, d));
}

/** Formats an instant (ISO timestamp) in the viewer's time zone. */
export function formatInstant(timestamp?: string): string {
  return timestamp ? dateTime.format(new Date(timestamp)) : '—';
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Today's date as YYYY-MM-DD in the viewer's time zone. */
export function todayLocal(): string {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}
