---
title: "Política de Control de Contraseñas"
version: "1.0"
date: "2026-11-11"
iso_control: "A.8.5 — Autenticación segura"
status: "approved"
classification: "internal"
owner: "[RESPONSABLE_SEGURIDAD]"
next_review: "2027-05-11"
---

# POL-SEC-001: Política de Control de Contraseñas

## 1. Objetivo

Establecer los requisitos mínimos de complejidad, rotación y gestión de contraseñas para todos los sistemas de información de **[EMPRESA]**, con el fin de reducir el riesgo de acceso no autorizado y garantizar la autenticación segura de usuarios conforme al control **A.8.5** de la norma ISO/IEC 27001:2022.

## 2. Alcance

Esta política aplica a:

- Todos los empleados, contratistas, pasantes y terceros con acceso a sistemas de información de [EMPRESA].
- Todas las cuentas de usuario en sistemas operativos, aplicaciones corporativas, bases de datos, servicios en la nube, correo electrónico, VPN y cualquier recurso protegido por credenciales.
- Cuentas de servicio, cuentas de administración y cuentas privilegiadas (PAM).

## 3. Definiciones

| Término | Definición |
|---------|------------|
| **Contraseña** | Cadena de caracteres secreta utilizada como factor de autenticación para verificar la identidad de un usuario. |
| **Cuenta privilegiada** | Cuenta con permisos elevados de administración sobre sistemas, redes o bases de datos. |
| **MFA** | Autenticación multifactor; uso de dos o más factores de verificación independientes (algo que sabes, algo que tienes, algo que eres). |
| **Historial de contraseñas** | Registro de contraseñas anteriores asociadas a una cuenta para impedir su reutilización. |
| **Bloqueo de cuenta** | Deshabilitación temporal del acceso a una cuenta tras superar el umbral de intentos fallidos de autenticación. |
| **GPO** | Group Policy Object; objeto de directiva de grupo en entornos Active Directory para la aplicación centralizada de políticas. |

## 4. Requisitos Técnicos

### 4.1 Complejidad de la Contraseña

Toda contraseña **DEBE** cumplir simultáneamente los siguientes criterios:

| Parámetro | Requisito |
|-----------|-----------|
| Longitud mínima | **8 caracteres** |
| Letras mayúsculas | Al menos **1** carácter en mayúscula (A–Z) |
| Letras minúsculas | Al menos **1** carácter en minúscula (a–z) |
| Dígitos numéricos | Al menos **1** dígito (0–9) |
| Caracteres especiales | Al menos **1** carácter especial (`! @ # $ % ^ & * ( ) _ + - = [ ] { }`) |
| Nombre de usuario | La contraseña **NO** puede contener el nombre de usuario ni partes del nombre completo del titular |
| Palabras de diccionario | Se recomienda **NO** utilizar palabras de diccionario completas sin alteración |

**Ejemplos válidos:** `C0mpl!ance2026`, `$eguR1d@d_TI`
**Ejemplos inválidos:** `password`, `12345678`, `empresa2026`

### 4.2 Rotación y Vigencia

| Parámetro | Valor |
|-----------|-------|
| Vigencia máxima de la contraseña | **45 días calendario** |
| Vigencia mínima de la contraseña | **1 día** (previene cambios consecutivos para evadir historial) |
| Notificación de expiración | **7 días** antes del vencimiento, el sistema notificará al usuario |
| Gracia post-expiración | **0 días** — la contraseña expirada obliga al cambio inmediato en el siguiente inicio de sesión |

### 4.3 Historial y Reutilización

| Parámetro | Valor |
|-----------|-------|
| Contraseñas recordadas en historial | **5 últimas contraseñas** |
| Reutilización | **PROHIBIDA** — no se permite usar ninguna de las 5 contraseñas anteriores |
| Similitud | Se recomienda que la nueva contraseña difiera en al menos **3 caracteres** respecto a la anterior |

### 4.4 Bloqueo de Cuenta por Intentos Fallidos

| Parámetro | Valor |
|-----------|-------|
| Umbral de intentos fallidos | **7 intentos consecutivos** |
| Duración del bloqueo | **30 minutos** (desbloqueo automático) |
| Reinicio del contador | Tras una autenticación exitosa o transcurridos **30 minutos** sin intentos |
| Cuentas de administrador | Bloqueo **permanente** hasta desbloqueo manual por parte del equipo de TI |

### 4.5 Autenticación Multifactor (MFA)

La implementación de MFA es **obligatoria** para:

- Acceso a paneles de administración de servidores y firewalls
- Acceso remoto vía VPN corporativa
- Consolas de administración de servicios en la nube (AWS, Azure, GCP)
- Cuentas de correo electrónico con permisos de administrador
- Sistemas de gestión financiera y contable

Para el resto de sistemas, la implementación de MFA es **recomendada** y se evaluará en el plan de mejora continua del SGSI.

### 4.6 Almacenamiento y Transmisión

- Las contraseñas **NUNCA** deben almacenarse en texto plano. Se requiere hashing con algoritmos seguros: **bcrypt**, **scrypt** o **Argon2id**.
- Las contraseñas en tránsito deben transmitirse exclusivamente sobre canales cifrados (**TLS 1.2** o superior).
- Queda **PROHIBIDO** almacenar contraseñas en hojas de cálculo, documentos de texto, correos electrónicos, notas adhesivas o cualquier medio no cifrado.
- El uso de gestores de contraseñas corporativos aprobados por TI (p. ej., Bitwarden, 1Password Business) es **permitido y recomendado**.

### 4.7 Cuentas de Servicio

- Las contraseñas de cuentas de servicio deben tener una longitud mínima de **16 caracteres**.
- Su rotación se realiza cada **90 días** o inmediatamente tras la salida de cualquier persona con conocimiento de la credencial.
- Deben almacenarse en un gestor de secretos (p. ej., HashiCorp Vault, AWS Secrets Manager).

## 5. Responsabilidades

| Rol | Responsabilidad |
|-----|-----------------|
| **Usuarios** | Cumplir con los requisitos de complejidad, no compartir contraseñas, reportar sospechas de compromiso |
| **Equipo de TI** | Implementar las configuraciones técnicas (GPO, políticas IAM), monitorear bloqueos de cuentas y gestionar desbloqueos |
| **Responsable de Seguridad** | Auditar el cumplimiento de esta política trimestral, revisar logs de bloqueos, actualizar los parámetros según evolución de amenazas |
| **Dirección** | Aprobar la política, asignar recursos para su implementación y promover la cultura de seguridad |
| **Recursos Humanos** | Incorporar esta política en la inducción de nuevos empleados y en los contratos de prestación de servicios |

## 6. Excepciones

Cualquier excepción a esta política debe:

1. Ser solicitada por escrito al Responsable de Seguridad.
2. Incluir una justificación técnica detallada y un análisis de riesgo.
3. Ser aprobada por la Dirección y el Responsable del SGSI.
4. Tener una vigencia máxima de **6 meses**, tras los cuales debe ser reevaluada.
5. Quedar registrada en el Registro de Excepciones del SGSI con su control compensatorio.

## 7. Cumplimiento y Sanciones

El incumplimiento de esta política puede resultar en:

- **Primera infracción:** Notificación escrita y capacitación obligatoria de sensibilización.
- **Segunda infracción:** Suspensión temporal de acceso a sistemas hasta completar una evaluación de seguridad.
- **Infracciones reiteradas o graves:** Acción disciplinaria conforme al reglamento interno de trabajo, que puede incluir terminación del contrato.
- **Terceros y contratistas:** Terminación del acuerdo de servicio y revisión de accesos otorgados.

En todos los casos, el incidente será registrado en el sistema de gestión de incidentes del SGSI.

## 8. Referencias Normativas

- ISO/IEC 27001:2022 — Control A.8.5: Autenticación segura
- ISO/IEC 27002:2022 — Sección 8.5: Guía de implementación
- NIST SP 800-63B — Digital Identity Guidelines: Authentication
- CIS Controls v8 — Control 5: Account Management
- Reglamento interno de trabajo de [EMPRESA]

## 9. Historial de Revisiones

| Versión | Fecha | Autor | Descripción del Cambio |
|---------|-------|-------|------------------------|
| 1.0 | 2026-11-11 | [RESPONSABLE_SEGURIDAD] | Creación inicial del documento |
| — | 2027-05-11 | — | Próxima revisión programada |

---

**Aprobado por:** [NOMBRE_DIRECCION] — Gerencia General de [EMPRESA]

**Fecha de aprobación:** 2026-11-11

**Clasificación:** Uso Interno
