# ADR-002: PostgreSQL con RLS para aislamiento multiempresa

**Estado:** Aceptada  
**Fecha:** 2026-11-11  
**Autor:** Juan Sebastián Ortiz Quintero

## Contexto

La plataforma servirá a múltiples empresas (tenants) desde una misma instancia.
Cada empresa gestiona su propio SGSI con datos confidenciales: riesgos, evidencias,
evaluaciones y documentos internos. Un fallo de aislamiento sería un incidente
de seguridad grave.

Se evaluaron tres estrategias:
1. Base de datos separada por empresa.
2. Esquema separado por empresa.
3. Base compartida con Row-Level Security (RLS).

## Decisión

**Base compartida con RLS.** Cada tabla de datos de cliente lleva
`organization_id`. Las políticas RLS de PostgreSQL aplican filtrado automático.
La aplicación también valida permisos en cada solicitud.

## Justificación

- **Simplicidad operacional:** una sola base de datos simplifica migraciones,
  backups, monitoreo y despliegue. Para las primeras 50-100 empresas, el
  rendimiento es más que suficiente.
- **RLS como red de seguridad:** incluso si un bug en la aplicación olvida
  filtrar por empresa, la política RLS bloquea el acceso. Es defensa en
  profundidad, no la única barrera.
- **Migraciones uniformes:** un cambio de esquema se aplica una vez, no
  N veces (una por empresa).
- **PostgreSQL nativo:** RLS está disponible desde PostgreSQL 9.5 y es
  una funcionalidad estable y documentada.

## Implementación

```sql
-- Ejemplo de política RLS
ALTER TABLE risks ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON risks
  USING (organization_id = current_setting('app.current_org_id')::uuid);
```

El middleware de la API establece `app.current_org_id` en cada conexión
después de verificar la autenticación y la membresía del usuario.

## Consecuencias

- Cada tabla de datos de cliente debe incluir `organization_id`.
- Las consultas manuales (debug, soporte) deben establecer el contexto
  de tenant antes de ejecutarse.
- Las tablas de catálogo normativo (controles ISO, frameworks) son
  compartidas y no llevan `organization_id`.
- Se necesitan pruebas automáticas que verifiquen acceso cruzado.

## Alternativas descartadas

**BD separada por empresa:** costosa de operar, complica migraciones.
Se reconsiderará si un cliente enterprise requiere aislamiento físico.

**Esquema separado:** menos costosa que BD separada pero igualmente
compleja para migraciones. No ofrece ventajas claras sobre RLS para
el volumen esperado.
