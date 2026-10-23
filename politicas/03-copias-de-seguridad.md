# Política de copias de seguridad

| Código | Versión | Aprobó | Fecha |
|---|---|---|---|
| SI-POL-03 | 1.0 | [GERENTE GENERAL] | [AAAA-MM-DD] |

## 1. Objetivo

Garantizar que la información de [EMPRESA] se pueda recuperar ante fallas de equipos, borrados accidentales, ransomware o desastres.

## 2. Regla 3‑2‑1

Toda información crítica debe tener:

- **3** copias (el original y dos respaldos),
- en **2** medios distintos (por ejemplo, disco del servidor y NAS),
- y **1** copia fuera de la sede o en la nube, desconectada o inmutable.

## 3. Qué se respalda y con qué frecuencia

| Información | Frecuencia | Retención | Responsable |
|---|---|---|---|
| Bases de datos (contable, ERP, nómina) | Completo semanal, diferencial diario, log cada 30 min | 30 días + cierre mensual por 1 año | Sistemas |
| Carpetas compartidas del servidor | Diario | 14 días | Sistemas |
| Correo electrónico | Lo provee el servicio en la nube; verificar retención | Según servicio | Sistemas |
| Configuración de equipos de red y PLC | Cada cambio y mensual | 12 versiones | Sistemas / mantenimiento |
| Soportes contables y facturas electrónicas | Diario | **10 años** (exigencia tributaria) | Contabilidad + sistemas |

## 4. Pruebas de restauración

- **Mensual:** se restaura un archivo o carpeta al azar y se verifica que abre.
- **Semestral:** se restaura una base de datos completa en un equipo de pruebas.
- Cada prueba se registra con fecha, qué se restauró, tiempo que tomó y resultado.

## 5. Seguridad de las copias

- Las copias externas se cifran.
- Las credenciales de los respaldos no son las mismas de los usuarios del dominio.
- Al menos una copia queda fuera del alcance de un ransomware que infecte la red (desconectada, inmutable o en nube con versiones).

## 6. Monitoreo

Sistemas revisa cada día hábil el resultado de los respaldos. Un respaldo fallido dos días seguidos se trata como incidente (SI-POL-05).
