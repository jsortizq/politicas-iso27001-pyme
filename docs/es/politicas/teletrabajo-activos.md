---
title: "Política de Teletrabajo, Gestión de Activos y Escritorio Limpio"
version: "1.0"
date: "2026-11-11"
iso_control: "A.6.7 / A.7.7 / A.8.1"
status: "approved"
classification: "internal"
author: "[RESPONSABLE_SEGURIDAD]"
approved_by: "[NOMBRE_DIRECCION]"
---

# Política de Teletrabajo, Gestión de Activos y Escritorio Limpio

## 1. Objetivo

Establecer los requisitos de seguridad de la información que deben cumplirse cuando el personal de **[EMPRESA]** realice actividades laborales fuera de las instalaciones corporativas, así como definir las directrices para la gestión segura de activos de información y la aplicación de prácticas de escritorio y pantalla limpia, en conformidad con los controles **A.6.7**, **A.7.7** y **A.8.1** de ISO/IEC 27001:2022.

## 2. Alcance

Esta política aplica a:

- Todo el personal de **[EMPRESA]** que realice teletrabajo, trabajo remoto o trabajo móvil, sea de forma regular o esporádica.
- Contratistas y terceros con acceso a sistemas de información de la organización desde ubicaciones externas.
- Todos los dispositivos (corporativos y personales bajo esquema BYOD) utilizados para acceder a recursos de **[EMPRESA]**.
- Todas las ubicaciones físicas donde se procese información corporativa, incluyendo oficinas, domicilios particulares y espacios públicos.

## 3. Definiciones

| Término | Definición |
|---------|-----------|
| **Teletrabajo** | Actividad laboral realizada fuera de las instalaciones de [EMPRESA] de forma regular o esporádica, utilizando tecnologías de la información. |
| **Activo de información** | Cualquier elemento que contenga, procese o transmita información de valor para la organización: equipos, software, documentos, datos. |
| **BYOD** | *Bring Your Own Device*. Uso de dispositivos personales del empleado para fines laborales. |
| **VPN** | *Virtual Private Network*. Red privada virtual que cifra las comunicaciones entre el dispositivo remoto y la red corporativa. |
| **Escritorio limpio** | Práctica de no dejar información sensible visible o accesible en el área de trabajo al ausentarse. |
| **Pantalla limpia** | Práctica de bloquear la sesión del equipo al ausentarse del puesto de trabajo. |
| **Cifrado de disco completo** | Tecnología que cifra la totalidad del almacenamiento del dispositivo, protegiendo los datos en caso de pérdida o robo. |

## 4. Requisitos de Seguridad para Teletrabajo

### 4.1 Conexión a la Red Corporativa

- **Es obligatorio el uso de la VPN corporativa** para todo acceso a sistemas internos, sin excepción.
- La VPN debe utilizar cifrado mínimo **AES-256** con protocolo **IKEv2/IPsec** o **WireGuard**.
- Queda **prohibido** acceder a sistemas corporativos desde redes Wi-Fi públicas o abiertas sin la VPN activa.
- El *split tunneling* está **deshabilitado** por defecto; todo el tráfico debe transitar por la VPN cuando se acceda a recursos internos.
- La sesión VPN se desconectará automáticamente tras **30 minutos** de inactividad.

### 4.2 Cifrado de Dispositivos

- Todo dispositivo portátil (laptop, tablet) que acceda a información de **[EMPRESA]** debe tener **cifrado de disco completo** habilitado:
  - **Windows**: BitLocker con algoritmo XTS-AES 256-bit. La clave de recuperación debe almacenarse en el directorio corporativo (Active Directory / Azure AD).
  - **macOS**: FileVault 2 con clave de recuperación institucional custodiada por el equipo de TI.
  - **Linux**: LUKS (dm-crypt) con passphrase mínima de 20 caracteres.
- Los dispositivos de almacenamiento externo (USB, discos portátiles) utilizados para información corporativa deben estar cifrados con **BitLocker To Go** o equivalente aprobado.
- Queda **prohibido** deshabilitar o suspender el cifrado de disco sin autorización escrita del Responsable de Seguridad.

### 4.3 Entorno Físico de Trabajo Remoto

- El teletrabajador debe disponer de un espacio de trabajo que permita privacidad visual y auditiva durante el manejo de información confidencial.
- Las reuniones virtuales con contenido clasificado no deben realizarse en espacios compartidos donde terceros puedan escuchar o visualizar la pantalla.
- En espacios públicos (aeropuertos, cafeterías, transporte), es **obligatorio** el uso de **filtros de privacidad de pantalla** en laptops y tablets.
- El equipo no debe dejarse desatendido en vehículos, áreas comunes de hoteles o cualquier lugar de acceso público.

### 4.4 Autenticación y Control de Acceso

- Todo acceso remoto requiere **autenticación multifactor (MFA)**.
- Las credenciales corporativas no deben almacenarse en navegadores de dispositivos personales.
- Está **prohibido** compartir credenciales VPN o tokens MFA con terceros, incluyendo familiares.

## 5. Gestión de Activos de Información

### 5.1 Inventario y Etiquetado

- Todo activo de TI asignado al personal debe registrarse en el **inventario corporativo de activos** con los siguientes datos mínimos:
  - Número de serie del equipo
  - Modelo y fabricante
  - Fecha de asignación
  - Nombre del custodio responsable
  - Estado de cifrado (habilitado/deshabilitado)
  - Ubicación habitual (oficina / domicilio / móvil)
- Cada activo debe llevar una **etiqueta de propiedad** visible con el código de inventario de **[EMPRESA]**.
- La verificación del inventario se realizará de forma **semestral**.

### 5.2 Uso Aceptable de Activos

- Los activos corporativos son para uso **exclusivamente laboral**, salvo autorización explícita.
- Queda **prohibido**:
  - Instalar software no autorizado o de procedencia dudosa.
  - Modificar la configuración de seguridad del equipo (firewall, antivirus, cifrado).
  - Conectar el equipo corporativo a redes inseguras sin VPN.
  - Prestar el equipo a personas no autorizadas, incluyendo familiares.

### 5.3 Devolución de Activos

- Al término de la relación laboral o al cambiar de rol, el colaborador debe devolver todos los activos corporativos en un plazo máximo de **3 días hábiles**.
- El equipo de TI realizará un borrado seguro certificado (**NIST 800-88**) antes de reasignar el equipo.

## 6. Política de Escritorio Limpio y Pantalla Limpia

### 6.1 Escritorio Limpio

- Al ausentarse del puesto de trabajo (incluido el domicilio en teletrabajo), todo documento impreso con información clasificada como **Confidencial** o **Restringida** debe guardarse bajo llave.
- No se permite dejar notas adhesivas con contraseñas, códigos PIN o información sensible visibles en el monitor, escritorio o alrededores.
- Los documentos impresos que ya no sean necesarios deben destruirse mediante **trituradora de corte cruzado** (nivel P-4 mínimo según DIN 66399).
- Las pizarras y superficies de escritura deben limpiarse después de sesiones de trabajo con información sensible.

### 6.2 Pantalla Limpia

- La sesión del equipo debe bloquearse automáticamente tras **5 minutos** de inactividad.
  - **Windows**: Configuración de GPO con bloqueo de sesión obligatorio.
  - **macOS**: Configuración de Screen Lock en Preferencias de Seguridad.
  - **Linux**: Configuración de screensaver con bloqueo.
- Antes de ausentarse del puesto, el colaborador debe bloquear la sesión manualmente:
  - **Windows**: `Win + L`
  - **macOS**: `Ctrl + Cmd + Q`
  - **Linux**: `Super + L` (según distribución)
- Está **prohibido** desactivar el bloqueo automático de pantalla o configurar tiempos superiores a 5 minutos.
- No deben quedar ventanas abiertas con información sensible al finalizar la jornada; cerrar todas las aplicaciones y sesiones.

## 7. Dispositivos Personales (BYOD)

### 7.1 Condiciones para el Uso de BYOD

El uso de dispositivos personales para acceder a recursos de **[EMPRESA]** está **permitido únicamente** si se cumplen todas las condiciones siguientes:

1. El dispositivo cuenta con **sistema operativo actualizado** (última versión mayor o la inmediatamente anterior con parches de seguridad al día).
2. Tiene instalado el **software antivirus/antimalware** aprobado por TI.
3. Tiene habilitado el **cifrado de disco completo**.
4. El acceso se realiza **exclusivamente a través de la VPN corporativa**.
5. El colaborador ha firmado el **Acuerdo de Uso de Dispositivos Personales**.
6. El equipo de TI ha verificado y registrado el dispositivo.

### 7.2 Restricciones BYOD

- **[EMPRESA]** no es responsable de la pérdida de datos personales durante la aplicación de medidas de seguridad corporativas.
- En caso de desvinculación laboral o incidente de seguridad, **[EMPRESA]** se reserva el derecho de solicitar el **borrado remoto** de los datos corporativos del dispositivo.
- Queda **prohibido** almacenar información clasificada como Confidencial en dispositivos personales fuera de aplicaciones corporativas autorizadas (sandboxing).

## 8. Reporte de Incidentes

### 8.1 Dispositivos Perdidos o Robados

- Todo dispositivo corporativo o BYOD autorizado que sea **perdido o robado** debe reportarse al equipo de TI y al Responsable de Seguridad en un plazo máximo de **1 hora** desde el momento en que se detecta la pérdida.
- El reporte debe incluir:
  - Descripción del dispositivo (modelo, número de serie)
  - Hora y lugar aproximado de la pérdida
  - Si el dispositivo tenía cifrado de disco activo
  - Si había sesiones activas al momento de la pérdida
  - Si se presentó denuncia policial (obligatoria en caso de robo)
- Canales de reporte:
  - **Correo**: seguridad@[EMPRESA].com
  - **Teléfono**: línea directa de TI (disponible 24/7)
  - **Ticket**: sistema de gestión de incidentes

### 8.2 Respuesta ante Pérdida/Robo

- TI ejecutará inmediatamente el **bloqueo remoto** del dispositivo.
- Se revocarán las credenciales de acceso VPN y corporativas asociadas al dispositivo.
- Si el equipo contenía datos sin cifrar, se activará el **protocolo de notificación de brecha** según la legislación aplicable.
- Se documentará el incidente en el registro de incidentes de seguridad de la información.

## 9. Responsabilidades

| Rol | Responsabilidad |
|-----|----------------|
| **Dirección General** | Aprobar esta política y asignar recursos para su implementación. |
| **Responsable de Seguridad de la Información** | Supervisar el cumplimiento, actualizar la política y gestionar incidentes. |
| **Responsable de TI** | Implementar controles técnicos: VPN, cifrado, inventario, borrado remoto, GPOs. |
| **Recursos Humanos** | Incluir esta política en la inducción y gestionar la devolución de activos. |
| **Todo el personal** | Cumplir esta política, reportar incidentes y proteger los activos asignados. |

## 10. Sanciones por Incumplimiento

El incumplimiento de esta política será tratado según la gravedad:

| Nivel | Ejemplo | Consecuencia |
|-------|---------|-------------|
| **Leve** | No bloquear la pantalla al ausentarse. | Amonestación verbal y capacitación obligatoria. |
| **Moderado** | Conectarse sin VPN o desactivar el cifrado. | Amonestación escrita y restricción temporal de acceso remoto. |
| **Grave** | No reportar pérdida de dispositivo en el plazo establecido. | Suspensión de privilegios de teletrabajo y proceso disciplinario. |
| **Muy grave** | Compartir credenciales VPN con terceros o filtrar información. | Proceso disciplinario que puede resultar en terminación del contrato y acciones legales. |

## 11. Revisiones y Actualizaciones

Esta política será revisada **anualmente** o ante cambios significativos en la infraestructura, la legislación o el panorama de amenazas.

## Historial de Revisiones

| Versión | Fecha | Descripción del Cambio | Autor |
|---------|-------|----------------------|-------|
| 1.0 | 2026-11-11 | Creación inicial del documento. | [RESPONSABLE_SEGURIDAD] |

---

> **Clasificación**: Interna  
> **Controles ISO 27001:2022**: A.6.7 (Trabajo remoto), A.7.7 (Escritorio limpio y pantalla limpia), A.8.1 (Dispositivos de punto final de usuario)  
> **Próxima revisión**: 2027-11-11
