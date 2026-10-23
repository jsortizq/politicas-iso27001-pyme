# Política de control de acceso y contraseñas

| Código | Versión | Aprobó | Fecha |
|---|---|---|---|
| SI-POL-02 | 1.0 | [GERENTE GENERAL] | [AAAA-MM-DD] |

## 1. Objetivo

Asegurar que solo las personas autorizadas accedan a los sistemas y a la información de [EMPRESA], y únicamente a lo que necesitan para su trabajo.

## 2. Cuentas de usuario

1. Cada persona tiene **su propia cuenta**. No se permiten cuentas compartidas, salvo cuentas de servicio documentadas.
2. El jefe de área solicita por escrito (correo o formato) la creación de la cuenta y los permisos.
3. Sistemas crea la cuenta con el **mínimo privilegio** necesario.
4. Las cuentas de administrador se usan solo para tareas de administración, nunca para correo o navegación.

## 3. Altas, cambios y retiros

| Evento | Quién avisa | Plazo para sistemas |
|---|---|---|
| Ingreso | Talento humano / jefe de área | Antes del primer día |
| Cambio de cargo | Jefe de área | 2 días hábiles; se retiran los permisos anteriores |
| Retiro | Talento humano | **El mismo día** del retiro |

## 4. Contraseñas

- Mínimo **12 caracteres**. Se recomiendan frases fáciles de recordar: `Cucuta-tiene-34-grados!`.
- No se reutiliza la contraseña del trabajo en servicios personales.
- No se escribe en papeles, ni se comparte por correo, chat o teléfono. Sistemas **nunca** la pedirá.
- Se cambia inmediatamente si se sospecha que alguien la conoce.
- Se bloquea la cuenta tras **10 intentos fallidos** durante 15 minutos.
- No se exige cambio periódico obligatorio (recomendación actual de NIST SP 800‑63B), salvo sospecha de compromiso.

## 5. Autenticación de dos factores (2FA)

Obligatoria para:

- Correo electrónico corporativo
- Acceso remoto (VPN o escritorio remoto)
- Banca en línea y plataformas de facturación electrónica
- Cuentas de administrador

## 6. Revisión de accesos

Cada **seis meses** sistemas envía a cada jefe de área la lista de usuarios y permisos de su equipo. El jefe confirma o pide ajustes en 5 días hábiles. Se conserva evidencia de la revisión.

## 7. Equipos desatendidos

- La sesión se bloquea al levantarse del puesto (Windows + L).
- Bloqueo automático de pantalla a los 5 minutos de inactividad.
