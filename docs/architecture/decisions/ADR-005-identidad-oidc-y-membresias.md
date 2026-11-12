# ADR-005: Identidad con OIDC y membresías por organización

**Estado:** Aceptada  
**Fecha:** 2026-11-12  
**Autor:** Juan Sebastián Ortiz Quintero

## Contexto

La plataforma es multiempresa y sus usuarios objetivo incluyen consultores que
trabajan con varias organizaciones. Necesitamos autenticar personas sin
almacenar contraseñas, decidir a qué organización accede cada una y con qué
rol, y hacerlo sin debilitar el aislamiento por RLS (ADR-002).

## Decisión

1. **Autenticación delegada en un proveedor OpenID Connect** (Keycloak, Google,
   Microsoft Entra ID…) mediante el flujo de código de autorización con PKCE.
   La plataforma no guarda contraseñas. El cliente OIDC está implementado con
   la biblioteca estándar de Go (`internal/auth/oidc.go`).
2. **Una fila de `users` por persona y organización.** La identidad es el
   `sub` del proveedor; una persona que pertenece a tres organizaciones tiene
   tres filas, cada una con su rol.
3. **Invitación por correo, aceptada en el primer inicio de sesión**, solo si
   el proveedor declara el correo como verificado (`email_verified`). A partir
   de ahí la membresía queda ligada al `sub`, no al correo.
4. **La sesión guarda la organización activa; el rol no se guarda.** En cada
   petición se lee la membresía, de modo que una baja o un cambio de rol se
   aplican de inmediato a las sesiones abiertas.
5. **Búsqueda al iniciar sesión mediante una política RLS específica**
   (`auth_lookup_users`, migración 005). Es la única consulta que cruza
   organizaciones: tras verificar el ID token, la API fija `app.auth_sub` (y
   `app.auth_email` solo si el correo está verificado) durante una transacción
   y puede leer —no modificar— exclusivamente las filas de esa identidad.

## Verificaciones de seguridad del flujo

- Algoritmo de firma fijo (RS256); `none` y cualquier otro se rechazan.
- Se validan `iss`, `aud`, `azp` (si hay varias audiencias), `exp`, `iat` y
  `nonce`; las claves se obtienen del JWKS del proveedor (mínimo 2048 bits).
- `state` de un solo uso y ligado al navegador con una cookie HttpOnly, para
  impedir que un inicio de sesión ajeno se complete en el navegador de la víctima.
- El emisor del documento de descubrimiento debe coincidir con `OIDC_ISSUER`.

## Alternativas consideradas

- **Contraseñas propias:** obliga a gestionar hash, recuperación, bloqueo y
  MFA; el proveedor de identidad ya lo resuelve.
- **Biblioteca OIDC de terceros:** menos código propio, pero añade
  dependencias a un proyecto que hoy solo tiene una (el driver de PostgreSQL).
  A cambio, el código propio está cubierto por pruebas que manipulan los
  tokens (emisor, audiencia, caducidad, nonce, firma, algoritmo).
- **Tabla global de identidades + tabla de membresías:** más normalizada,
  pero exige una tabla sin RLS con datos personales. Se puede migrar a ese
  modelo más adelante sin cambiar la API.
- **Función `SECURITY DEFINER` para la búsqueda:** depende de que el dueño de
  las tablas pueda saltarse RLS, lo que no ocurre en todos los PostgreSQL
  gestionados.

## Consecuencias

- La política de búsqueda confía en que la API solo fija `app.auth_sub` tras
  verificar un token; es el mismo nivel de confianza que ya tiene
  `app.tenant_id`. RLS protege frente a errores de consulta, no frente a una
  API comprometida.
- Las invitaciones no funcionan con proveedores que no envían
  `email_verified = true`.
- Solo RS256. Un proveedor configurado con otro algoritmo (p. ej. ES256) no
  podrá usarse hasta añadirlo.
- Las sesiones siguen en memoria: se pierden al reiniciar y no se comparten
  entre instancias. Pasarlas a Redis es requisito para escalar horizontalmente.
- No hay cierre de sesión en el proveedor (RP-initiated logout): cerrar sesión
  en la plataforma no cierra la del proveedor de identidad.
- La plataforma no envía correos: quien invita debe avisar a la persona.
