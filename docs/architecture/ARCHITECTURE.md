# Arquitectura — ISO 27001 PyME SaaS

> Plataforma SaaS multiempresa para la gestión del Sistema de Gestión de Seguridad
> de la Información (SGSI) conforme a ISO/IEC 27001:2022 + Enmienda 1:2024.

**Versión:** 0.1.0  
**Fecha:** 2026-11-11  
**Autor:** Juan Sebastián Ortiz Quintero  
**Estado:** Borrador inicial

---

## 1. Visión del producto

Construir una plataforma SaaS que permita a PyMEs y consultores gestionar su SGSI
de forma estructurada: evaluación de controles, gestión de riesgos, documentación,
evidencias y auditorías, con un copiloto de IA integrado que asiste pero no decide.

**Dos capas fundamentales:**

- **Motor SGSI:** controla requisitos, riesgos, controles, evidencias, documentos,
  auditorías y seguimiento. Es la autoridad.
- **Copiloto IA:** analiza, redacta, relaciona y recomienda. No sustituye
  decisiones ni aprobaciones humanas.

**Mercado inicial:** PyMEs hispanohablantes y consultores ISO en Latinoamérica.

---

## 2. Stack tecnológico

| Capa                | Tecnología                          | Justificación                                    |
|---------------------|-------------------------------------|--------------------------------------------------|
| Frontend            | Next.js + React + TypeScript        | SSR, rutas API, ecosistema maduro                |
| Estilos             | Tailwind CSS                        | Utilidades, accesibilidad, temas                 |
| Backend             | Go (monolito modular)               | Rendimiento, tipado, concurrencia, CLI existente |
| Base de datos       | PostgreSQL                          | RLS para multitenancy, jsonb, madurez            |
| Autenticación       | OIDC (Keycloak o proveedor externo) | Estándar abierto, MFA, federación                |
| Almacenamiento      | S3 compatible (MinIO en dev)        | Evidencias y documentos fuera de la BD           |
| Cola de trabajos    | Redis + workers Go                  | Procesos IA, informes, análisis asíncronos       |
| Búsqueda semántica  | pgvector                            | RAG para el copiloto, sin servicio externo       |
| Contenedores        | Docker + Docker Compose             | Entorno reproducible, dev = producción           |
| CI/CD               | GitHub Actions                      | Ya configurado en el repositorio                 |

**Versiones objetivo:**
- Go: ≥ 1.23 (migrar desde 1.21 actual)
- PostgreSQL: ≥ 16
- Node.js: ≥ 20 LTS

---

## 3. Decisión arquitectónica: monolito modular

**No microservicios.** Para el volumen inicial, un monolito bien separado por
dominios es más sencillo de desarrollar, desplegar y mantener.

Cuatro binarios que comparten paquetes de dominio:

```
cmd/api/         → API HTTP (la plataforma web)
cmd/worker/      → Procesos en segundo plano (IA, informes)
cmd/iso-cli/     → CLI independiente (herramienta existente)
web/             → Frontend Next.js (aplicación separada)
```

La separación lógica interna usa `internal/modules/`, donde cada módulo tiene
sus propios modelos, servicios, repositorios y handlers HTTP.

Cuando el volumen lo justifique, se pueden extraer módulos como servicios
independientes sin reescribir la lógica de dominio.

---

## 4. Estructura del repositorio

```
politicas-iso27001-pyme/
├── cmd/
│   ├── api/main.go              # Punto de entrada API
│   ├── worker/main.go           # Workers asíncronos
│   └── iso-cli/main.go          # CLI existente (preservar)
├── internal/
│   ├── platform/                # Infraestructura transversal
│   │   ├── config/              # Configuración por entorno
│   │   ├── database/            # Conexión, migraciones, RLS
│   │   ├── auth/                # OIDC, tokens, sesiones
│   │   ├── tenancy/             # Contexto multiempresa
│   │   ├── storage/             # Adaptador S3/MinIO
│   │   ├── logging/             # Registro estructurado
│   │   └── http/                # Middleware, router, errores
│   ├── modules/                 # Dominios de negocio
│   │   ├── organizations/       # Empresas, sedes, suscripciones
│   │   ├── users/               # Usuarios, roles, permisos
│   │   ├── frameworks/          # Catálogo ISO versionado
│   │   ├── context/             # Alcance SGSI, interesados
│   │   ├── assets/              # Inventario de activos
│   │   ├── risks/               # Evaluación y tratamiento
│   │   ├── controls/            # Evaluación y SoA
│   │   ├── documents/           # Políticas, versiones, aprobaciones
│   │   ├── evidence/            # Archivos, hashes, trazabilidad
│   │   ├── audits/              # Programación, hallazgos, acciones
│   │   ├── incidents/           # Registro y respuesta
│   │   ├── reports/             # Dashboard e informes
│   │   ├── ai/                  # Gateway IA, RAG, copiloto
│   │   └── billing/             # Planes y suscripciones SaaS
│   └── shared/                  # Tipos y utilidades compartidas
├── web/                         # Frontend Next.js
│   ├── app/                     # App Router
│   ├── components/              # Componentes React
│   ├── features/                # Lógica por módulo
│   ├── hooks/                   # Hooks reutilizables
│   ├── lib/                     # Clientes API, utilidades
│   └── public/                  # Estáticos
├── migrations/                  # SQL versionado (ej: goose)
├── catalog/                     # Catálogo normativo de referencia
│   └── iso27001/2022/           # 93 controles + cláusulas 4-10
├── templates/                   # Plantillas de políticas
│   ├── es/
│   └── en/
├── tests/
│   ├── integration/             # Pruebas contra PostgreSQL
│   ├── e2e/                     # Playwright
│   └── security/                # Acceso cruzado entre empresas
├── deploy/
│   ├── docker/
│   └── terraform/
├── docs/
│   ├── architecture/            # Este documento y ADRs
│   ├── api/                     # OpenAPI specs
│   └── manuals/                 # Guías de usuario
├── docker-compose.yml
├── Makefile
├── go.mod
└── README.md
```

---

## 5. Modelo de datos multiempresa

### Estrategia de aislamiento

Base PostgreSQL compartida con separación lógica:
- Cada registro lleva `organization_id`.
- Políticas RLS con denegación por defecto.
- La API verifica pertenencia del usuario a la organización en cada solicitud.
- Las pruebas automáticas verifican intentos de acceso cruzado.

### Entidades principales

**Identidad y organizaciones:**
`users`, `organizations`, `memberships`, `roles`, `permissions`, `subscriptions`

**Marco normativo (centralizado, no por empresa):**
`frameworks`, `framework_versions`, `requirements`, `controls`

**SGSI por empresa:**
`organization_context`, `organization_scope`, `interested_parties`,
`assets`, `risk_assessments`, `risks`, `risk_treatments`,
`control_assessments`, `soa_entries`

**Documentación y evidencia:**
`documents`, `document_versions`, `document_approvals`,
`evidences`, `evidence_links`, `evidence_reviews`

**Operación:**
`audits`, `audit_findings`, `corrective_actions`,
`incidents`, `management_reviews`, `tasks`, `audit_events`

**Inteligencia artificial:**
`ai_sessions`, `ai_messages`, `ai_document_chunks`, `ai_usage`

### Evaluación de controles (tres dimensiones separadas)

| Dimensión      | Valores posibles                       |
|----------------|----------------------------------------|
| Aplicabilidad  | Pendiente, Aplicable, No aplicable     |
| Implementación | No iniciado, En progreso, Implementado |
| Efectividad    | No evaluado, Efectivo, No efectivo     |

Un control no aplicable requiere justificación y aprobación.
Un control implementado no equivale automáticamente a efectivo.

---

## 6. API REST

Versionada desde el inicio. Documentada con OpenAPI.

Ejemplos de endpoints:

```
GET    /api/v1/me
POST   /api/v1/organizations
GET    /api/v1/organizations/{id}/dashboard
GET    /api/v1/organizations/{id}/assets
POST   /api/v1/organizations/{id}/risks
GET    /api/v1/organizations/{id}/controls
PUT    /api/v1/organizations/{id}/soa/{controlId}
POST   /api/v1/organizations/{id}/evidence
POST   /api/v1/organizations/{id}/audits
POST   /api/v1/organizations/{id}/ai/assist
```

El `organization_id` del path se valida contra la membresía del usuario
autenticado. Nunca se confía en el valor que envía el cliente.

---

## 7. Diseño de la IA

### AI Gateway interno

Ningún módulo se conecta directamente a un proveedor de modelos. El gateway:
- Permite cambiar de proveedor sin tocar los módulos.
- Controla costos con cuotas por empresa.
- Registra modelo, versión del prompt, fuentes, usuario, consumo y fecha.
- Aplica aislamiento: documentos de una empresa nunca responden a otra.

### RAG con pgvector

- Los documentos se procesan en chunks con embeddings.
- Cada chunk lleva `organization_id`.
- La búsqueda semántica filtra por empresa antes de recuperar.
- Los resultados muestran las fuentes utilizadas.

### Reglas de seguridad IA

1. Aislamiento estricto por `organization_id` en toda consulta.
2. Documentos subidos se tratan como datos, no como instrucciones.
3. La IA no puede aprobar controles, cerrar incidentes ni aceptar riesgos.
4. Los resultados importantes requieren revisión humana.
5. Toda generación registra trazabilidad completa.

---

## 8. Seguridad de la plataforma

| Área           | Implementación                                    |
|----------------|---------------------------------------------------|
| Identidad      | OIDC, MFA, sesiones seguras                       |
| Autorización   | RBAC + permisos por recurso                       |
| Separación     | PostgreSQL RLS + validación en API                |
| Cifrado        | TLS en tránsito, cifrado en reposo                |
| Secretos       | Gestor de secretos (nunca en Git)                 |
| Evidencias     | Archivos privados, SHA-256, enlaces temporales    |
| Auditoría      | Registro inmutable de operaciones sensibles       |
| Disponibilidad | Backups automatizados, pruebas de restauración    |
| Aplicación     | Rate limiting, validación de entrada, OWASP Top 10|
| Desarrollo     | Análisis de dependencias y vulnerabilidades en CI |

### Registro de auditoría inmutable

Cada operación sensible registra:
- Usuario, empresa, fecha/hora
- Registro afectado, estado anterior, estado nuevo
- Origen de la operación (UI, API, CLI, IA)

Las aprobaciones y cambios de evaluación generan eventos que no pueden
modificarse desde los flujos habituales de la aplicación.

---

## 9. Cobertura normativa

La plataforma cubre:
- **93 controles** del Anexo A de ISO/IEC 27001:2022
- **Cláusulas 4 a 10** (requisitos de gestión del SGSI)
- **Enmienda 1:2024** (consideración de cambio climático en contexto)

El catálogo de referencia es centralizado y versionado. Cada empresa determina
su propia aplicabilidad, justificación, responsables y evidencias.

No se copia texto protegido de la norma ISO. Las descripciones son originales.

---

## 10. Fases de implementación

| Fase | Objetivo                              | Resultado                   |
|------|---------------------------------------|-----------------------------|
| 0    | Refactorización, pruebas, catálogo    | Base técnica confiable      |
| 1    | Login, empresas, permisos             | SaaS multiempresa funcional |
| 2    | Contexto, activos, riesgos, SoA       | Motor de gestión SGSI       |
| 3    | Documentos y evidencias               | Trazabilidad documental     |
| 4    | Dashboard, informes, auditorías       | Gestión operativa           |
| 5    | IA y recuperación documental          | Copiloto ISO                |
| 6    | Integraciones técnicas                | Verificaciones automáticas  |
| 7    | Facturación y administración          | Producto comercial          |

### MVP (primera versión funcional)

1. Registro y creación de organización
2. Invitación de usuarios y permisos
3. Configuración del alcance SGSI
4. Inventario de activos
5. Evaluación de riesgos
6. Consulta y evaluación de los 93 controles (SoA)
7. Adjuntar evidencias
8. Crear y aprobar políticas
9. Dashboard de avance
10. Exportar informe para revisión

---

## 11. Migración del código actual

El código existente se preserva y evoluciona:

- `cmd/iso-cli/main.go` sigue como punto de entrada CLI.
- Las funciones de `main.go` se extraen a paquetes en `internal/`.
- La CLI seguirá funcionando como herramienta independiente.
- `SOA_Matrix.json` se transforma en datos iniciales del catálogo versionado.
- Las políticas existentes se conservan como plantillas.

---

## 12. Problemas conocidos del código actual

| Hallazgo                          | Acción                                        |
|-----------------------------------|-----------------------------------------------|
| SoA con 14 controles              | Crear catálogo de referencia con los 93        |
| Resumen SoA inconsistente         | Calcular indicadores desde los registros       |
| `checkDiskSpace()` no mide disco  | Implementar medición real con `syscall.Statfs` |
| Permisos de dirs: control erróneo | Log dirs se etiquetan como A.8.13, debe ser A.8.15 |
| A.5.1 apunta a política errónea   | Vincular a política general, no a contraseñas  |
| Código concentrado en `main.go`   | Separar en paquetes de dominio                 |
| `go.mod` declara Go 1.21          | Actualizar a ≥ 1.23                            |
| CI no ejecuta `go test`           | Agregar etapa de pruebas unitarias             |

---

## 13. Estándares de calidad (Definition of Done)

Cada tarea se acepta cuando cumple:

- [ ] Código probado (unitario + integración donde aplique)
- [ ] Migraciones de BD versionadas
- [ ] API documentada en OpenAPI
- [ ] Validación de permisos y multitenancy
- [ ] Pruebas de aislamiento entre empresas
- [ ] Registro de auditoría para operaciones sensibles
- [ ] Manejo de errores (sin panics, errores descriptivos)
- [ ] Documentación actualizada
- [ ] Pipeline CI aprobado (`go test -race`, `go vet`, lint)

---

## Estado de implementación (2026-11-12)

| Área                                   | Estado                                              |
|----------------------------------------|-----------------------------------------------------|
| Catálogo de 93 controles               | Hecho (`catalog/`, tabla `control_catalog`)         |
| Multiempresa con RLS                   | Hecho, con pruebas de acceso cruzado en PostgreSQL  |
| SoA, activos y riesgos (API + web)     | Hecho                                               |
| Registro de auditoría                  | Hecho: se escribe en cada cambio; API y pantalla    |
| Autenticación                          | SSO con OIDC (ADR-005) y login de desarrollo        |
| Organizaciones, miembros y roles       | Hecho: alta, invitación, roles, cambio de empresa   |
| Sesiones                               | En memoria; pendiente pasarlas a Redis              |
| Documentos y evidencias (S3)           | Pendiente                                           |
| Auditorías internas e informes         | Pendiente (tablas creadas, sin API)                 |
| Workers (Redis) y copiloto IA          | Pendiente                                           |

Diferencias con la estructura objetivo de la sección 4: los paquetes viven en
`internal/{api,auth,database,tenant,...}` en lugar de `internal/platform` e
`internal/modules`, y las tablas usan `tenant_id` en lugar de `organization_id`.

## Registro de decisiones (ADR)

Las decisiones de arquitectura se documentan en archivos individuales
en `docs/architecture/decisions/`. Véase [ADR-001](decisions/ADR-001-monolito-modular.md).
La identidad y las membresías están en [ADR-005](decisions/ADR-005-identidad-oidc-y-membresias.md).
