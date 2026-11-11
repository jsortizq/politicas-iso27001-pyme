---
title: "Política de Uso de Correo Electrónico"
version: "1.0"
date: "2026-11-11"
iso_control: "A.5.10"
status: "approved"
classification: "internal"
owner: "[RESPONSABLE_TI]"
approved_by: "[NOMBRE_DIRECCION]"
---

# Política de Uso de Correo Electrónico

## 1. Objetivo

Establecer las normas, controles técnicos y responsabilidades para el uso del servicio de correo electrónico corporativo de **[EMPRESA]**, con el fin de proteger la confidencialidad, integridad y disponibilidad de la información transmitida, prevenir el uso indebido del sistema y evitar el ingreso de las direcciones IP y dominios corporativos en listas negras de reputación antispam.

## 2. Alcance

Esta política aplica a:

- Todos los empleados, contratistas, pasantes y terceros autorizados que utilicen el correo electrónico corporativo de [EMPRESA].
- Todas las cuentas de correo bajo el dominio corporativo `@[EMPRESA].com` y subdominios asociados.
- Los servidores de correo (MTA/MDA), gateways de seguridad y servicios de correo en la nube utilizados por la organización.
- Dispositivos corporativos y personales (BYOD) desde los cuales se acceda al correo corporativo.

## 3. Definiciones

| Término | Definición |
|---------|-----------|
| **SPF** (Sender Policy Framework) | Registro DNS TXT que autoriza qué servidores pueden enviar correo en nombre del dominio. |
| **DKIM** (DomainKeys Identified Mail) | Firma criptográfica añadida a los encabezados del correo que verifica la autenticidad del remitente y la integridad del mensaje. |
| **DMARC** (Domain-based Message Authentication, Reporting & Conformance) | Política DNS que indica a los servidores receptores cómo tratar correos que fallen las validaciones SPF y/o DKIM, e incluye mecanismos de reporte. |
| **Lista Negra (Blacklist)** | Base de datos pública que registra direcciones IP o dominios identificados como fuentes de spam o actividad maliciosa. Ejemplos: Spamhaus ZEN, FabelSources, UceprotectL3. |
| **MTA** | Mail Transfer Agent — servidor encargado del envío y enrutamiento de correo electrónico. |
| **Envío masivo** | Distribución de correo a más de 50 destinatarios simultáneos o más de 200 mensajes por hora desde una misma cuenta o servidor. |
| **Phishing** | Técnica de ingeniería social que suplanta identidades legítimas para obtener credenciales o información confidencial. |

## 4. Requisitos Técnicos de Infraestructura

### 4.1. Registros DNS Obligatorios

El equipo de TI **debe** mantener configurados y verificados trimestralmente los siguientes registros DNS para cada dominio de correo de [EMPRESA]:

#### SPF (Sender Policy Framework)

```dns
[EMPRESA].com.  IN  TXT  "v=spf1 include:_spf.[PROVEEDOR].com ip4:[IP_SERVIDOR_SMTP] -all"
```

- Se **debe** utilizar el calificador `-all` (hard fail) para rechazar correos de servidores no autorizados.
- Se prohíbe el uso de `+all` o `?all` en producción.
- El registro SPF no debe exceder 10 búsquedas DNS (lookups) según RFC 7208.

#### DKIM (DomainKeys Identified Mail)

```dns
selector._domainkey.[EMPRESA].com.  IN  TXT  "v=DKIM1; k=rsa; p=[CLAVE_PUBLICA_BASE64]"
```

- Longitud mínima de clave RSA: **2048 bits**.
- Rotación de claves DKIM: cada **6 meses** como máximo.
- Se debe firmar al menos los encabezados: `From`, `To`, `Subject`, `Date`, `Message-ID`.

#### DMARC (Domain-based Message Authentication, Reporting & Conformance)

```dns
_dmarc.[EMPRESA].com.  IN  TXT  "v=DMARC1; p=quarantine; rua=mailto:dmarc-reports@[EMPRESA].com; ruf=mailto:dmarc-forensic@[EMPRESA].com; pct=100; adkim=s; aspf=s"
```

- La política **mínima** aceptable es `p=quarantine`. Se recomienda migrar a `p=reject` una vez verificado el flujo de correo legítimo.
- Los reportes agregados (`rua`) deben ser revisados **semanalmente** por el equipo de TI.
- El alineamiento estricto (`adkim=s; aspf=s`) es obligatorio.

### 4.2. Protección contra Listas Negras

[EMPRESA] reconoce que la inclusión de sus IPs o dominios en listas negras antispam tiene un impacto operativo severo. Las siguientes listas son monitoreadas activamente:

| Lista Negra | Tipo | Impacto |
|-------------|------|---------|
| **Spamhaus ZEN** | Compuesta (SBL+XBL+PBL+DBL) | Crítico — utilizada por el 80%+ de servidores de correo a nivel mundial |
| **FabelSources** | IP y dominio | Alto — bloquea entregas a proveedores europeos y latinoamericanos |
| **UceprotectL3** | Rango de red completo | Severo — puede bloquear rangos /24 completos afectando a terceros |

**Controles preventivos:**

- Monitoreo diario automatizado del estado de las IPs de salida en al menos las 3 listas anteriores.
- Límite de envío por cuenta: máximo **100 mensajes por hora** y **500 por día** para cuentas estándar.
- Cuentas de servicio (notificaciones, facturación) deben utilizar IPs de envío dedicadas y separadas de las cuentas de usuario.
- Activación de rate-limiting en el MTA para prevenir envíos masivos accidentales.

### 4.3. Límites Técnicos del Sistema

| Parámetro | Valor |
|-----------|-------|
| Tamaño máximo de adjuntos | **25 MB** por mensaje |
| Tamaño máximo de buzón | **10 GB** por usuario estándar, **25 GB** para directivos |
| Retención en bandeja de entrada | **12 meses** — correos anteriores deben archivarse |
| Retención de correos eliminados | **30 días** en papelera antes de purga automática |
| Sesiones simultáneas máximas | **5** por cuenta |
| Tiempo de expiración de sesión inactiva | **30 minutos** |

## 5. Normas de Uso

### 5.1. Uso Aceptable

- El correo corporativo es una herramienta de trabajo y debe utilizarse exclusivamente para actividades laborales de [EMPRESA].
- Todo correo que contenga **información clasificada como confidencial o restringida** debe enviarse con cifrado:
  - Adjuntos sensibles: archivo comprimido con contraseña (ZIP/AES-256) comunicada por canal separado (teléfono, mensajería cifrada).
  - Alternativa: cifrado S/MIME con certificado digital emitido por la PKI corporativa o CA de confianza.
- Los usuarios deben verificar los destinatarios antes de enviar, especialmente en respuestas a hilos con múltiples participantes.
- Se debe incluir la **firma corporativa estándar** y el **aviso de confidencialidad** aprobados por [EMPRESA] al pie de cada mensaje externo.

### 5.2. Mantenimiento de Buzón

- Los usuarios son responsables de mantener su buzón por debajo del límite de almacenamiento.
- Los correos con más de **12 meses de antigüedad** deben ser archivados en la solución de archivo corporativo o eliminados si no tienen valor de retención legal o empresarial.
- TI ejecutará una campaña trimestral de limpieza con notificación previa de 15 días.

## 6. Prohibiciones Específicas

Queda **terminantemente prohibido**:

1. **Envíos masivos no autorizados**: Distribución de correos a más de 50 destinatarios externos sin autorización escrita del área de Marketing y supervisión de TI. Esto incluye newsletters, promociones, invitaciones masivas y cadenas.
2. **Uso personal comercial**: Utilizar el correo corporativo para negocios personales, ventas, publicidad de terceros o actividades ajenas a [EMPRESA].
3. **Reenvío automático a cuentas externas**: Configurar reglas de reenvío a dominios personales (Gmail, Outlook.com, Yahoo, etc.) sin autorización de Seguridad de la Información.
4. **Suplantación de identidad**: Modificar encabezados, alias o campos "From" para aparentar ser otro usuario o entidad.
5. **Transmisión de malware**: Enviar archivos ejecutables (.exe, .bat, .ps1, .vbs, .js) o archivos con macros habilitadas sin cifrar y sin justificación documentada.
6. **Evasión de controles**: Intentar evadir filtros antispam, DLP (Data Loss Prevention) o gateways de seguridad mediante codificación, ofuscación o fraccionamiento de adjuntos.
7. **Almacenamiento de credenciales**: Guardar contraseñas, tokens de acceso, claves API o certificados privados en el cuerpo del correo o en borradores.

## 7. Gestión de Incidentes de Correo

### 7.1. Reporte de Phishing y Spam

Todo usuario que reciba un correo sospechoso de phishing o spam debe:

1. **No abrir** enlaces ni adjuntos del mensaje sospechoso.
2. **No reenviar** el correo a otros usuarios.
3. Utilizar el botón de **"Reportar phishing"** del cliente de correo (si está disponible) o reenviar como adjunto (.eml) a: `seguridad@[EMPRESA].com`.
4. Notificar por canal alterno (chat corporativo o teléfono) al equipo de TI si el correo solicita acciones urgentes financieras.

### 7.2. Respuesta ante Inclusión en Listas Negras

Si se detecta que una IP o dominio de [EMPRESA] aparece en una lista negra:

1. **Detección** (SLA: < 4 horas): El sistema de monitoreo alerta al equipo de TI.
2. **Contención** (SLA: < 8 horas): Identificar y bloquear la fuente del envío abusivo. Aislar la cuenta o servidor comprometido.
3. **Remediación** (SLA: < 24 horas): Solicitar la eliminación (delist) de la lista negra correspondiente. Documentar la causa raíz.
4. **Post-mortem** (SLA: < 72 horas): Generar informe técnico con timeline, impacto, causa raíz y acciones preventivas.

Referencia: ver el caso documentado en `docs/es/casos-reales/recuperacion-listas-negras.md`.

### 7.3. Compromiso de Cuenta

Si se sospecha que una cuenta de correo ha sido comprometida:

1. Cambio inmediato de contraseña de la cuenta afectada.
2. Revocación de todas las sesiones activas y tokens de aplicación.
3. Revisión de reglas de reenvío y configuraciones de la cuenta.
4. Análisis de los últimos 30 días de actividad de la cuenta (logs de acceso, correos enviados).
5. Notificación a los contactos que pudieran haber recibido correos fraudulentos desde la cuenta.

## 8. Responsabilidades

| Rol | Responsabilidad |
|-----|----------------|
| **Todos los usuarios** | Cumplir esta política, reportar incidentes, mantener su buzón y no realizar envíos masivos no autorizados. |
| **Equipo de TI** | Administrar la infraestructura de correo, mantener SPF/DKIM/DMARC, monitorear listas negras, aplicar límites técnicos y responder a incidentes. |
| **Responsable de Seguridad de la Información** | Definir y actualizar esta política, auditar el cumplimiento, investigar incidentes de seguridad relacionados con correo. |
| **Dirección** | Aprobar esta política, asignar recursos para su implementación y respaldar las acciones disciplinarias derivadas de incumplimientos. |
| **Marketing / Comunicaciones** | Coordinar con TI los envíos masivos autorizados, utilizando plataformas dedicadas (Mailchimp, SendGrid, etc.) con IPs independientes. |

## 9. Sanciones por Incumplimiento

El incumplimiento de esta política puede resultar en:

- **Leve** (primera infracción sin impacto): Amonestación verbal y capacitación obligatoria.
- **Moderada** (reincidencia o impacto menor): Amonestación escrita y restricción temporal de funcionalidades de correo.
- **Grave** (envío masivo que cause blacklisting, fuga de datos o compromiso de cuenta por negligencia): Suspensión de la cuenta de correo, proceso disciplinario formal según el reglamento interno de [EMPRESA] y posibles acciones legales.

## 10. Revisión y Actualización

Esta política será revisada al menos **una vez al año** o ante:

- Cambios en la infraestructura de correo electrónico.
- Incidentes de seguridad significativos relacionados con correo.
- Modificaciones en la normativa ISO/IEC 27001:2022 o legislación aplicable.
- Cambios en el proveedor de servicios de correo.

## 11. Historial de Revisiones

| Versión | Fecha | Autor | Descripción del Cambio |
|---------|-------|-------|------------------------|
| 1.0 | 2026-11-11 | [RESPONSABLE_SEGURIDAD] | Emisión inicial de la política |

---

**Clasificación:** Interna  
**Control ISO 27001:2022:** A.5.10 — Uso aceptable de información y otros activos asociados  
**Documento:** POL-CORREO-001  
**Próxima revisión:** 2027-11-11
