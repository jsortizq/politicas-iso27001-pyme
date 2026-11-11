---
title: "Post-Mortem: Recuperación de Listas Negras Antispam"
version: "1.0"
date: "2026-11-11"
severity: critical
incident_id: "INC-2026-0047"
iso_controls:
  - A.5.24
  - A.5.26
  - A.8.21
classification: internal
author: "[RESPONSABLE_SEGURIDAD]"
approved_by: "[NOMBRE_DIRECCION]"
---

# Post-Mortem: Recuperación de Listas Negras Antispam

> **Clasificación:** Interno — No distribuir fuera de [EMPRESA]  
> **Severidad:** CRÍTICA  
> **Estado:** Cerrado — Medidas preventivas implementadas

---

## 1. Resumen Ejecutivo

El presente documento detalla el incidente de seguridad ocurrido cuando la dirección IP del servidor de correo de [EMPRESA] fue incluida en múltiples listas negras antispam (Spamhaus ZEN, Barracuda BRBL y SpamCop) debido al compromiso de una cuenta de correo interna utilizada para el envío masivo de spam.

El incidente fue detectado tras la notificación de [SOCIO_COMERCIAL], quien informó que todos los correos electrónicos provenientes de [EMPRESA] estaban siendo rechazados por sus servidores. La interrupción total del servicio de correo con socios clave se extendió por más de **72 horas**, generando un impacto operativo y financiero estimado en **$15.000 USD**.

La resolución completa requirió **7 días calendario** desde la detección hasta la restauración total del servicio de correo electrónico.

---

## 2. Datos del Incidente

| Campo | Detalle |
|-------|---------|
| **Fecha de detección** | 2026-03-15 a las 09:47 (UTC-5) |
| **Método de detección** | Notificación externa de [SOCIO_COMERCIAL] |
| **Fecha de resolución** | 2026-03-22 a las 14:30 (UTC-5) |
| **Duración total** | 7 días calendario |
| **Sistemas afectados** | Servidor de correo (mail.empresa.com), DNS público |
| **Cuentas comprometidas** | 1 (usuario del área comercial) |
| **Listas negras afectadas** | Spamhaus ZEN, Barracuda BRBL, SpamCop |
| **Clasificación** | Compromiso de credenciales + envío de spam |

---

## 3. Cronología Detallada

### Día 0 — Detección (15 de marzo)

| Hora | Evento |
|------|--------|
| 09:47 | [SOCIO_COMERCIAL] envía correo al Director Comercial informando que los mensajes de [EMPRESA] son rechazados con código `550 5.7.1 Service unavailable; client host blocked using zen.spamhaus.org` |
| 10:15 | El Director Comercial escala al equipo de TI |
| 10:30 | TI confirma que la IP `203.0.113.50` del servidor de correo aparece listada en Spamhaus ZEN |
| 10:45 | Verificación adicional revela inclusión en Barracuda BRBL y SpamCop |
| 11:00 | Se declara incidente de severidad CRÍTICA y se activa el equipo de respuesta |
| 11:30 | Revisión inicial de logs del servidor de correo: se detectan **47.000 mensajes salientes** en las últimas 48 horas desde la cuenta `comercial3@empresa.com` |
| 12:00 | Se notifica a la Dirección General sobre el incidente y el impacto estimado |

### Día 1 — Contención (16 de marzo)

| Hora | Evento |
|------|--------|
| 08:00 | Análisis forense de la cuenta `comercial3@empresa.com`: último acceso legítimo hace 5 días; accesos posteriores desde IP `45.xx.xx.xx` (geolocalización: Europa del Este) |
| 08:30 | Se deshabilita la cuenta comprometida |
| 09:00 | Se fuerza el restablecimiento de contraseña de la cuenta |
| 09:30 | Revisión del correo de la víctima: se identifica un correo de phishing recibido el 10 de marzo, simulando una notificación de Microsoft 365 con enlace a formulario de captura de credenciales |
| 10:00 | Se bloquean las IP de origen del atacante en el firewall perimetral |
| 11:00 | Se configura restricción temporal de envío: máximo 50 correos por hora por cuenta |
| 14:00 | Se notifica formalmente a [SOCIO_COMERCIAL] sobre el incidente y las acciones tomadas |
| 16:00 | Se establecen canales alternativos de comunicación (teléfono, WhatsApp Business) con los 5 socios comerciales principales |

### Día 2 — Cambio de IP (17 de marzo)

| Hora | Evento |
|------|--------|
| 09:00 | Se solicita al ISP la asignación de una nueva dirección IP pública para el servidor de correo |
| 14:00 | ISP asigna la nueva IP `203.0.113.75` |
| 15:00 | Reconfiguración del servidor de correo con la nueva IP |
| 16:00 | Verificación de la nueva IP en listas negras: **limpia** en todas las listas principales |
| 17:00 | Pruebas de envío limitadas con la nueva IP (10 correos a cuentas internas de prueba) |

### Día 3 — Fortalecimiento DNS (18 de marzo)

| Hora | Evento |
|------|--------|
| 08:00 | Auditoría completa de registros DNS del dominio `empresa.com` |
| 09:00 | **Hallazgo crítico:** el registro SPF contenía `v=spf1 ip4:203.0.113.0/24 +all` — el mecanismo `+all` anulaba toda protección, permitiendo que cualquier IP enviara correo en nombre del dominio |
| 09:30 | Actualización del registro SPF: `v=spf1 ip4:203.0.113.75 include:_spf.google.com -all` |
| 10:00 | Rotación de la clave DKIM: generación de nuevo par RSA de 2048 bits, publicación del registro TXT `selector2026._domainkey.empresa.com` |
| 11:00 | Configuración de DMARC por primera vez: `v=DMARC1; p=quarantine; rua=mailto:dmarc-reports@empresa.com; ruf=mailto:dmarc-forensics@empresa.com; pct=100` |
| 12:00 | Configuración de registro PTR (reverse DNS) para la nueva IP |
| 14:00 | Verificación con herramientas externas: MXToolbox, DMARCian, mail-tester.com — puntuación 9/10 |

### Día 4 — Solicitudes de Eliminación de Listas Negras (19 de marzo)

| Hora | Evento |
|------|--------|
| 08:00 | Solicitud de eliminación (delisting) en Spamhaus ZEN para la IP antigua `203.0.113.50` |
| 08:30 | Solicitud de eliminación en Barracuda BRBL |
| 09:00 | Solicitud de eliminación en SpamCop |
| 10:00 | Verificación en listas secundarias: SORBS, UCEPROTECT Level 1, Invaluement — IP antigua listada en UCEPROTECT L1 |
| 10:30 | Solicitud de eliminación en UCEPROTECT |
| 14:00 | Spamhaus confirma la eliminación de la IP antigua |
| 16:00 | Barracuda confirma la eliminación |

### Día 5 — Escaneo de Red y Verificación (20 de marzo)

| Hora | Evento |
|------|--------|
| 08:00 | Escaneo completo de la red interna con `nmap -sV -sC -O 192.168.1.0/24` para identificar otros equipos potencialmente comprometidos |
| 09:30 | Análisis con antimalware actualizado (ESET Endpoint) en todas las estaciones de trabajo (32 equipos) |
| 12:00 | Resultado del escaneo: **2 estaciones** con software potencialmente no deseado (PUP), **0 malware activo** adicional |
| 13:00 | Limpieza de las 2 estaciones identificadas |
| 14:00 | Verificación de que ninguna otra cuenta presenta actividad anómala de envío |
| 15:00 | SpamCop confirma la eliminación de la IP antigua |

### Día 7 — Restauración Completa (22 de marzo)

| Hora | Evento |
|------|--------|
| 08:00 | Pruebas de envío a los 5 socios comerciales principales |
| 09:00 | [SOCIO_COMERCIAL] confirma recepción correcta de correos |
| 10:00 | Verificación en MXToolbox: IP nueva sin listados, puntuación DNS 10/10 |
| 14:00 | Se notifica oficialmente a la Dirección que el servicio está completamente restaurado |
| 14:30 | Se cierra el incidente con estado **RESUELTO** |

---

## 4. Análisis de Causa Raíz

### 4.1 Causa Directa

Un empleado del área comercial fue víctima de un ataque de phishing que replicaba la página de inicio de sesión de Microsoft 365. Las credenciales capturadas fueron utilizadas por el atacante para enviar aproximadamente **47.000 correos de spam** en un período de 48 horas.

### 4.2 Causas Contribuyentes

| # | Causa | Nivel de Contribución |
|---|-------|-----------------------|
| 1 | **Ausencia de MFA** en el sistema de correo: las credenciales solas eran suficientes para acceder | ALTO |
| 2 | **Registro SPF permisivo** con mecanismo `+all` que invalidaba toda la protección SPF | ALTO |
| 3 | **Ausencia de política DMARC** que hubiera permitido detectar y rechazar correos no autenticados | ALTO |
| 4 | **Sin límite de tasa de envío** por cuenta: una sola cuenta pudo enviar 47.000 mensajes sin activar alertas | MEDIO |
| 5 | **Sin monitoreo de volumen de correo saliente**: no existían alertas por picos anómalos de envío | MEDIO |
| 6 | **Capacitación insuficiente** del personal en identificación de phishing | MEDIO |

### 4.3 Diagrama de Ishikawa (Resumen)

```
Personas          Procesos              Tecnología
   │                  │                      │
   ├─ Sin            ├─ Sin política       ├─ Sin MFA
   │  capacitación   │  de correo          │
   │  anti-phishing  │  documentada        ├─ SPF con +all
   │                 │                     │
   └─ Credenciales   ├─ Sin monitoreo     ├─ Sin DMARC
      débiles        │  de envío          │
                     │                     └─ Sin rate-limit
                     └─ Sin plan de
                        respuesta a
                        incidentes
```

---

## 5. Impacto

### 5.1 Impacto Operativo

- **Correo bloqueado:** más de 72 horas sin comunicación por email con socios clave
- **Órdenes de compra retrasadas:** 3 órdenes por valor total de $15.000 USD
- **Productividad:** equipo comercial (5 personas) improductivo parcialmente durante 3 días

### 5.2 Impacto Reputacional

- Pérdida de confianza temporal con [SOCIO_COMERCIAL] y otros 4 socios
- Necesidad de explicación formal del incidente a socios comerciales
- Percepción de vulnerabilidad ante clientes

### 5.3 Impacto Financiero Estimado

| Concepto | Monto (USD) |
|----------|-------------|
| Órdenes retrasadas | $15.000 |
| Horas-hombre de TI (respuesta al incidente, 80 horas) | $2.400 |
| Nueva IP pública (cargo del ISP) | $150 |
| Herramientas de diagnóstico y monitoreo | $500 |
| **Total estimado** | **$18.050** |

---

## 6. Lecciones Aprendidas

1. **La autenticación de un solo factor es insuficiente.** Una contraseña comprometida fue suficiente para causar un incidente de severidad crítica. MFA habría prevenido completamente el acceso no autorizado.

2. **Los registros DNS de correo son la primera línea de defensa.** Un SPF con `+all` es funcionalmente equivalente a no tener SPF. La configuración correcta de SPF, DKIM y DMARC debe ser validada por personal calificado.

3. **El monitoreo proactivo es más económico que la reacción.** Si hubiera existido una alerta por volumen anómalo de correo saliente, el incidente se habría detectado en horas, no en días.

4. **La capacitación del personal no es opcional.** El vector de ataque fue un simple correo de phishing. Una sesión trimestral de concientización reduce significativamente este riesgo.

5. **Un plan de respuesta a incidentes documentado acelera la recuperación.** La respuesta fue ad-hoc; un procedimiento establecido habría reducido el tiempo de resolución.

---

## 7. Medidas Preventivas Implementadas

### 7.1 Acciones Inmediatas (completadas)

- [x] Activación de MFA (Microsoft Authenticator) para todas las cuentas de correo
- [x] Corrección del registro SPF a modo estricto (`-all`)
- [x] Implementación de DMARC con política `p=quarantine`
- [x] Rotación de clave DKIM a RSA 2048 bits
- [x] Configuración de rate-limiting: máximo 200 correos por hora por cuenta
- [x] Alertas automáticas cuando una cuenta supera 100 correos en una hora
- [x] Restablecimiento forzado de contraseñas de todas las cuentas de correo

### 7.2 Acciones a Corto Plazo (30 días)

- [x] Capacitación obligatoria anti-phishing para todo el personal
- [x] Simulacro de phishing interno con herramienta GoPhish
- [x] Documentación de la Política de Uso de Correo Electrónico (ver `docs/es/politicas/uso-correo-electronico.md`)
- [x] Suscripción a servicio de monitoreo de reputación de IP (MXToolbox Monitoring)

### 7.3 Acciones a Mediano Plazo (90 días)

- [x] Migración de DMARC de `p=quarantine` a `p=reject` tras 60 días de monitoreo sin falsos positivos
- [x] Implementación de gateway de correo con análisis de contenido saliente
- [x] Auditoría trimestral de registros DNS de correo
- [ ] Evaluación de solución de SIEM para correlación de eventos de seguridad

---

## 8. Verificación de Controles ISO 27001:2022

| Control | Descripción | Estado Pre-Incidente | Estado Post-Incidente |
|---------|-------------|---------------------|-----------------------|
| A.5.24 | Planificación de gestión de incidentes | ❌ No existía | ✅ Procedimiento documentado |
| A.5.26 | Respuesta a incidentes | ❌ Ad-hoc | ✅ Plan formal con roles y escalamiento |
| A.8.5 | Autenticación segura | ⚠️ Solo contraseña | ✅ MFA obligatorio |
| A.8.21 | Seguridad de servicios de red | ❌ SPF permisivo, sin DMARC | ✅ SPF estricto + DKIM + DMARC |

---

## 9. Aprobación y Distribución

| Rol | Nombre | Fecha |
|-----|--------|-------|
| Elaborado por | [RESPONSABLE_SEGURIDAD] | 2026-03-25 |
| Revisado por | [RESPONSABLE_TI] | 2026-03-26 |
| Aprobado por | [NOMBRE_DIRECCION] | 2026-03-28 |

**Distribución autorizada:** Dirección General, Responsable de Seguridad, Responsable de TI, Auditoría Interna.

---

## 10. Historial de Revisiones

| Versión | Fecha | Autor | Cambios |
|---------|-------|-------|---------|
| 1.0 | 2026-03-28 | [RESPONSABLE_SEGURIDAD] | Versión inicial del post-mortem |
| 1.1 | 2026-06-15 | [RESPONSABLE_SEGURIDAD] | Actualización de estado de medidas a mediano plazo |
