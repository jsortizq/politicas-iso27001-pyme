'use client';

import { useState } from 'react';
import { ApiError, errorMessage, membersApi, roleLabels, type Member, type Role } from '@/lib/api';
import { useSession } from '@/lib/session';
import { useResource } from '@/lib/useResource';
import { ErrorBanner, Field, PageHeader, Panel, Spinner } from '@/components/ui/feedback';

const roleHelp: Record<Role, string> = {
  owner: 'Control total, incluida la gestión de otros propietarios.',
  admin: 'Gestiona el SGSI y a los miembros (excepto propietarios).',
  editor: 'Crea y modifica controles, activos y riesgos.',
  auditor: 'Solo lectura, con acceso a miembros y al registro de actividad.',
  viewer: 'Solo lectura del SGSI.',
};

const allRoles = Object.keys(roleLabels) as Role[];

/** Messages for the refusals specific to member management. */
function memberError(err: unknown): string {
  if (err instanceof ApiError && err.code === 'last_owner') {
    return 'La organización debe conservar al menos un propietario.';
  }
  if (err instanceof ApiError && err.code === 'member_exists') {
    return 'Ya hay un miembro con ese correo.';
  }
  return errorMessage(err);
}

const dateFormat = new Intl.DateTimeFormat('es', { dateStyle: 'medium' });

function InviteForm({ roles, onDone }: { roles: Role[]; onDone: () => void }) {
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [role, setRole] = useState<Role>('viewer');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await membersApi.invite(email, name, role);
      onDone();
    } catch (err) {
      setError(memberError(err));
      setBusy(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {error && <ErrorBanner message={error} />}
      <fieldset disabled={busy} className="space-y-5">
        <Field
          label="Correo"
          hint="Debe ser el correo con el que esa persona inicia sesión. Entrará en la organización en su próximo inicio de sesión; la plataforma no envía correos de invitación."
        >
          <input type="email" required className="input" value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Nombre (opcional)">
          <input className="input" maxLength={200} value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Rol" hint={roleHelp[role]}>
          <select className="input" value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {roles.map((r) => (
              <option key={r} value={r}>
                {roleLabels[r]}
              </option>
            ))}
          </select>
        </Field>
      </fieldset>
      <button type="submit" className="btn btn-primary" disabled={busy}>
        {busy ? 'Invitando…' : 'Invitar'}
      </button>
    </form>
  );
}

export default function MembersPage() {
  const { user, canManageMembers, canViewAudit } = useSession();
  const { data, error, loading, reload } = useResource(membersApi.list);
  const [inviting, setInviting] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  if (!canViewAudit) {
    return <ErrorBanner message="Tu rol no permite ver los miembros de la organización." />;
  }
  if (loading) return <Spinner />;
  if (error) return <ErrorBanner message={error} onRetry={reload} />;

  const members = data?.data ?? [];
  const isOwner = user.role === 'owner';
  // Administrators cannot grant the owner role.
  const grantable = allRoles.filter((r) => isOwner || r !== 'owner');
  // Nobody edits themselves; only owners edit owners.
  const editable = (m: Member) => canManageMembers && m.id !== user.id && (isOwner || m.role !== 'owner');

  const act = async (id: string, action: () => Promise<unknown>) => {
    setBusyId(id);
    setActionError(null);
    try {
      await action();
      await reload();
    } catch (err) {
      setActionError(memberError(err));
    } finally {
      setBusyId(null);
      setConfirmRemove(null);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Miembros"
        subtitle="Personas con acceso a esta organización y lo que puede hacer cada una"
        action={
          canManageMembers ? (
            <button className="btn btn-primary" onClick={() => setInviting(true)}>
              Invitar miembro
            </button>
          ) : undefined
        }
      />

      {actionError && <ErrorBanner message={actionError} />}

      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/50 text-left">
              <th className="px-4 py-3 font-medium">Miembro</th>
              <th className="px-4 py-3 font-medium">Rol</th>
              <th className="hidden px-4 py-3 font-medium sm:table-cell">Estado</th>
              <th className="hidden px-4 py-3 font-medium md:table-cell">Último acceso</th>
              {canManageMembers && (
                <th className="px-4 py-3">
                  <span className="sr-only">Acciones</span>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.id} className="border-b last:border-0 hover:bg-muted/30">
                <td className="px-4 py-3">
                  <span className="font-medium">{m.name || m.email}</span>
                  {m.id === user.id && <span className="ml-2 text-xs text-muted-foreground">(tú)</span>}
                  {m.name && <span className="block text-xs text-muted-foreground">{m.email}</span>}
                  {m.status === 'invited' && (
                    <span className="badge badge-partial mt-1.5 sm:hidden">Invitación pendiente</span>
                  )}
                </td>
                <td className="px-4 py-3">
                  {editable(m) ? (
                    <select
                      className="input h-9 w-40"
                      aria-label={`Rol de ${m.email}`}
                      value={m.role}
                      disabled={busyId === m.id}
                      onChange={(e) => act(m.id, () => membersApi.setRole(m.id, e.target.value as Role))}
                    >
                      {grantable.map((r) => (
                        <option key={r} value={r}>
                          {roleLabels[r]}
                        </option>
                      ))}
                    </select>
                  ) : (
                    roleLabels[m.role]
                  )}
                </td>
                <td className="hidden px-4 py-3 sm:table-cell">
                  {m.status === 'invited' ? (
                    <span className="badge badge-partial">Invitación pendiente</span>
                  ) : (
                    <span className="badge badge-full">Activo</span>
                  )}
                </td>
                <td className="hidden px-4 py-3 text-muted-foreground md:table-cell">
                  {m.last_login ? dateFormat.format(new Date(m.last_login)) : '—'}
                </td>
                {canManageMembers && (
                  <td className="px-4 py-3 text-right">
                    {editable(m) &&
                      (confirmRemove === m.id ? (
                        <span className="inline-flex items-center gap-2">
                          <button
                            className="btn btn-danger h-8 px-3"
                            disabled={busyId === m.id}
                            onClick={() => act(m.id, () => membersApi.remove(m.id))}
                          >
                            Sí, quitar
                          </button>
                          <button className="btn btn-outline h-8 px-3" onClick={() => setConfirmRemove(null)}>
                            No
                          </button>
                        </span>
                      ) : (
                        <button className="btn btn-outline h-8 px-3" onClick={() => setConfirmRemove(m.id)}>
                          Quitar
                        </button>
                      ))}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <section className="rounded-lg border p-4 sm:p-6">
        <h2 className="font-semibold">Qué puede hacer cada rol</h2>
        <dl className="mt-3 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[8rem_1fr]">
          {allRoles.map((r) => (
            <div key={r} className="contents">
              <dt className="font-medium">{roleLabels[r]}</dt>
              <dd className="text-muted-foreground">{roleHelp[r]}</dd>
            </div>
          ))}
        </dl>
      </section>

      {inviting && (
        <Panel title="Invitar miembro" onClose={() => setInviting(false)}>
          <InviteForm
            roles={grantable}
            onDone={() => {
              setInviting(false);
              reload();
            }}
          />
        </Panel>
      )}
    </div>
  );
}
