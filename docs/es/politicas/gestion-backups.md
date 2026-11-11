---
title: "Política de Gestión de Copias de Seguridad"
version: "1.0"
date: "2026-11-11"
iso_control: "A.8.13"
status: "approved"
classification: "internal"
owner: "[RESPONSABLE_TI]"
approved_by: "[NOMBRE_DIRECCION]"
next_review: "2027-05-11"
---

# Política de Gestión de Copias de Seguridad (Backups)

**Organización:** [EMPRESA]
**Código:** POL-BKP-001 | **Versión:** 1.0 | **Clasificación:** Uso Interno

---

## 1. Objetivo

Establecer los requisitos, procedimientos y responsabilidades para la creación, almacenamiento, verificación y restauración de copias de seguridad de la información de [EMPRESA], garantizando la disponibilidad, integridad y confidencialidad de los datos ante incidentes de pérdida, corrupción o desastre.

## 2. Alcance

Esta política aplica a:

- Todos los sistemas de información en producción de [EMPRESA], incluyendo servidores físicos y virtuales, bases de datos, aplicaciones críticas de negocio y servicios en la nube.
- Todos los repositorios de datos compartidos (file servers, NAS, almacenamiento en nube corporativo).
- Estaciones de trabajo que contengan información clasificada como **confidencial** o **restringida**.
- Configuraciones de dispositivos de red (routers, switches, firewalls).
- Todo el personal de TI responsable de la administración de infraestructura y cualquier tercero que gestione backups bajo contrato de servicio.

## 3. Definiciones

| Término | Definición |
|---------|-----------|
| **Backup incremental** | Copia que registra únicamente los cambios realizados desde la última copia (incremental o completa). |
| **Backup completo (full)** | Copia íntegra de todos los datos del sistema o volumen en un momento dado. |
| **RPO (Recovery Point Objective)** | Cantidad máxima de datos que la organización puede permitirse perder, medida en tiempo. |
| **RTO (Recovery Time Objective)** | Tiempo máximo aceptable para restaurar un servicio tras una interrupción. |
| **Regla 3-2-1** | Estrategia de respaldo que exige al menos 3 copias de los datos, en 2 tipos de medio diferentes, con 1 copia almacenada fuera del sitio principal. |
| **Restauración de prueba** | Proceso planificado de recuperación de datos desde una copia de seguridad con el fin de validar su integridad y funcionalidad. |
| **Cifrado en reposo** | Protección criptográfica aplicada a datos almacenados para impedir su lectura sin la clave autorizada. |

## 4. Requisitos Técnicos

### 4.1 Estrategia de Respaldo — Regla 3-2-1

Todos los datos críticos de [EMPRESA] deben cumplir la regla 3-2-1:

1. **3 copias** de cada conjunto de datos: la copia de producción más dos copias de respaldo.
2. **2 tipos de medio** diferentes: por ejemplo, almacenamiento en disco local y almacenamiento en nube o cinta.
3. **1 copia fuera del sitio principal**: almacenada en una ubicación geográficamente separada (data center remoto, proveedor de nube o bóveda física).

### 4.2 Periodicidad de las Copias de Seguridad

| Tipo de Backup | Frecuencia | Ventana de Ejecución | Retención |
|----------------|-----------|----------------------|-----------|
| **Incremental** | Diario (lunes a sábado) | 22:00 – 04:00 (horario local) | 7 días |
| **Completo (full)** | Semanal (domingos) | 00:00 – 08:00 (horario local) | 4 semanas |
| **Completo mensual** | Primer domingo de cada mes | 00:00 – 10:00 (horario local) | 12 meses |
| **Completo anual** | 31 de diciembre | Según disponibilidad | 5 años (o según requisito legal) |

Los horarios podrán ajustarse según la carga del sistema, siempre que se mantenga la frecuencia establecida y se documente cualquier cambio.

### 4.3 Objetivos de Recuperación

| Parámetro | Objetivo | Justificación |
|-----------|----------|---------------|
| **RPO** | ≤ 24 horas | La pérdida máxima aceptable es la producción de un día laborable. |
| **RTO** | ≤ 4 horas para sistemas críticos; ≤ 24 horas para no críticos | Basado en el análisis de impacto al negocio (BIA) de [EMPRESA]. |

Estos valores deben ser revisados anualmente o tras cambios significativos en la infraestructura.

### 4.4 Cifrado y Protección de las Copias

- Todas las copias de seguridad deben estar cifradas en reposo utilizando **AES-256** como algoritmo mínimo.
- Las copias transmitidas a sitios remotos deben viajar por canales cifrados (**TLS 1.2+** o túnel VPN **IPSec/IKEv2**).
- Las claves de cifrado deben almacenarse separadas de las copias de seguridad, en un gestor de claves (KMS) o bóveda segura. Nunca en el mismo medio que los datos respaldados.
- El acceso a las copias de seguridad se limita exclusivamente al personal autorizado de TI mediante control de acceso basado en roles (RBAC).

### 4.5 Almacenamiento Remoto (Offsite)

- La copia offsite debe estar ubicada en una instalación a una distancia mínima de **50 km** del sitio principal, o en una región geográfica diferente si se utiliza un proveedor de nube.
- El proveedor de almacenamiento remoto debe cumplir con los requisitos de seguridad de [EMPRESA] y, preferiblemente, contar con certificación **ISO 27001** o **SOC 2 Tipo II**.
- La transferencia al sitio remoto debe completarse dentro de las **6 horas** siguientes a la finalización del backup.

### 4.6 Monitoreo y Alertas

- Todos los trabajos de backup deben ser monitoreados de forma automatizada.
- Los fallos de backup deben generar una alerta inmediata (correo electrónico o sistema de tickets) al equipo de TI.
- Los trabajos fallidos deben ser reiniciados manualmente o automáticamente dentro de las **2 horas** siguientes a la detección del fallo.
- Se debe mantener un registro (log) de todos los trabajos de backup ejecutados, incluyendo: fecha, hora de inicio, hora de fin, tamaño, resultado (éxito/fallo) y tipo de backup.

## 5. Procedimiento de Restauración de Prueba

### 5.1 Frecuencia

Se debe realizar al menos **una restauración de prueba aleatoria cada 30 días naturales**. Esta restauración debe:

- Seleccionar de forma aleatoria un sistema o conjunto de datos diferente cada vez.
- Cubrir todos los sistemas críticos al menos **una vez cada trimestre**.
- Documentarse por completo en el **Registro de Restauraciones de Prueba**.

### 5.2 Procedimiento

1. **Selección:** El responsable de TI selecciona aleatoriamente (o mediante rotación programada) el sistema y la copia a restaurar.
2. **Preparación:** Se dispone un entorno aislado (sandbox o servidor de prueba) que no afecte a la producción.
3. **Ejecución:** Se restaura la copia seleccionada en el entorno de prueba.
4. **Validación:** Se verifica:
   - Integridad de los datos (checksums SHA-256 comparados con el respaldo original).
   - Funcionalidad de la aplicación o accesibilidad de los archivos restaurados.
   - Que el tiempo de restauración esté dentro del RTO definido.
5. **Documentación:** Se registra en el formulario de prueba:
   - Fecha de la prueba.
   - Sistema restaurado y fecha del backup utilizado.
   - Tiempo total de restauración.
   - Resultado: **exitoso** / **exitoso con observaciones** / **fallido**.
   - Acciones correctivas si corresponde.
6. **Escalamiento:** Si la restauración falla, se inicia un ticket de incidencia de prioridad **alta** y se ejecuta un nuevo backup completo del sistema afectado dentro de las 24 horas.

### 5.3 Criterios de Éxito

La prueba se considera exitosa cuando:

- Los datos restaurados son idénticos al punto de respaldo (verificación por checksum).
- Las aplicaciones dependientes funcionan correctamente con los datos restaurados.
- El tiempo de restauración no excede el RTO establecido.

## 6. Sistemas Incluidos en el Alcance

A modo de referencia mínima, los siguientes sistemas deben contar con backup obligatorio:

- Servidores de bases de datos (SQL Server, PostgreSQL, MySQL, MongoDB).
- Servidores de aplicaciones y portales web.
- Servidor de correo electrónico y su almacenamiento.
- Directorio Activo / LDAP y configuraciones de GPO.
- Archivos compartidos (file server, NAS, SharePoint, OneDrive corporativo).
- Configuraciones de red (routers, switches, firewalls).
- Máquinas virtuales completas (snapshots a nivel de hipervisor cuando aplique).
- Repositorios de código fuente.

## 7. Responsabilidades

| Rol | Responsabilidad |
|-----|----------------|
| **Dirección General** | Aprobar esta política y asignar los recursos necesarios para su cumplimiento. |
| **Responsable del SGSI** | Supervisar el cumplimiento de esta política en el marco del Sistema de Gestión de Seguridad de la Información. |
| **Responsable de TI** | Implementar, operar y monitorear la infraestructura de backups. Ejecutar las restauraciones de prueba mensuales. |
| **Administradores de Sistemas** | Configurar los trabajos de backup según los parámetros de esta política. Atender alertas de fallo. |
| **Todos los empleados** | Almacenar información corporativa en las ubicaciones designadas para garantizar su inclusión en los respaldos. |
| **Proveedores externos** | Cumplir con los requisitos de cifrado y retención si gestionan respaldos bajo contrato. |

## 8. Incumplimiento

El incumplimiento de esta política podrá derivar en:

- Acción disciplinaria conforme al reglamento interno de [EMPRESA].
- Revisión de accesos y privilegios del personal involucrado.
- Notificación a la Dirección General si el incumplimiento genera un riesgo material para la organización.

## 9. Excepciones

Toda excepción a esta política debe:

1. Ser solicitada por escrito al Responsable del SGSI.
2. Incluir una justificación de negocio y un análisis de riesgo asociado.
3. Ser aprobada por la Dirección General.
4. Documentarse en el registro de excepciones con fecha de expiración (máximo 6 meses, renovable).
5. Ser revisada en la próxima auditoría interna.

## 10. Referencias Normativas

- ISO/IEC 27001:2022 — Control A.8.13 (Copias de seguridad de la información)
- ISO/IEC 27002:2022 — Sección 8.13 (Guía de implementación)
- NIST SP 800-34 Rev. 1 — Contingency Planning Guide

## 11. Historial de Revisiones

| Versión | Fecha | Autor | Descripción del Cambio |
|---------|-------|-------|----------------------|
| 1.0 | 2026-11-11 | [RESPONSABLE_TI] | Creación inicial del documento |
| — | — | — | — |

---

*Este documento es propiedad de [EMPRESA] y está clasificado como de **uso interno**. Su distribución fuera de la organización requiere autorización expresa de la Dirección General.*
