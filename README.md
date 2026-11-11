<div align="center">

# 🛡️ Políticas ISO 27001:2022 para PyMEs

### Compliance as Code — Framework Bilingüe de Seguridad de la Información

[![ISO 27001:2022](https://img.shields.io/badge/ISO%2027001-2022-blue?style=for-the-badge&logo=iso)](https://www.iso.org/standard/27001)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)
[![Bilingual](https://img.shields.io/badge/Lang-ES%20%7C%20EN-orange?style=for-the-badge&logo=translate)](docs/)

---

**[Español](#español)** · **[English](#english)**

</div>

---

## Español

### ¿Qué es este proyecto?

Un framework de **"Compliance as Code"** que combina documentación de gobernanza de seguridad de la información con una herramienta CLI desarrollada en **Go** para automatizar auditorías técnicas, diseñado específicamente para **pequeñas y medianas empresas (PyMEs)** que buscan alinearse con **ISO/IEC 27001:2022**.

### ¿Por qué existe?

La mayoría de las PyMEs no cuentan con un equipo de seguridad dedicado. Este repositorio entrega:

- **Políticas listas para usar** basadas en controles reales del Anexo A de ISO 27001:2022
- **Herramienta CLI** que automatiza la verificación de cumplimiento técnico
- **Matriz SoA** (Statement of Applicability) en formato JSON para trazabilidad
- **Caso de estudio real** anonimizado como referencia de respuesta a incidentes
- **Documentación bilingüe** (ES/EN) para equipos multinacionales

### Estructura del Repositorio

```
politicas-iso27001-pyme/
├── README.md                          # Este archivo
├── LICENSE                            # MIT License
├── SOA_Matrix.json                    # Matriz de Aplicabilidad (Anexo A)
├── go.mod                             # Módulo Go
├── cmd/
│   └── iso-cli/
│       └── main.go                    # CLI: check, validate-docs, export-soa
├── docs/
│   ├── es/
│   │   ├── politicas/
│   │   │   ├── control-contrasenas.md
│   │   │   ├── gestion-backups.md
│   │   │   ├── uso-correo-electronico.md
│   │   │   └── teletrabajo-activos.md
│   │   └── casos-reales/
│   │       └── recuperacion-listas-negras.md
│   └── en/
│       ├── policies/
│       │   ├── password-control.md
│       │   ├── backup-management.md
│       │   ├── email-usage.md
│       │   └── remote-work-assets.md
│       └── case-studies/
│           └── blacklist-recovery.md
├── scripts/
│   └── install.sh                     # Script de instalación
└── .github/
    └── workflows/
        └── ci.yml                     # Pipeline CI/CD
```

### Guía de Adopción en 4 Semanas

| Semana | Actividad | Entregable |
|--------|-----------|------------|
| **1** | Revisión de políticas y personalización (`[EMPRESA]` → tu razón social) | Políticas aprobadas por Dirección |
| **2** | Implementación técnica: contraseñas, backups, SPF/DKIM/DMARC | Controles operativos activos |
| **3** | Ejecución del CLI `iso-cli check` en infraestructura | Informe de brechas técnicas |
| **4** | Revisión de hallazgos, plan de remediación y simulacro de auditoría | Matriz SoA actualizada + evidencia |

### Uso del CLI

#### Instalación

```bash
# Clonar el repositorio
git clone https://github.com/jsortizq/politicas-iso27001-pyme.git
cd politicas-iso27001-pyme

# Compilar
go build -o iso-cli ./cmd/iso-cli/

# O instalar directamente
go install ./cmd/iso-cli/
```

#### Comandos Disponibles

```bash
# Auditoría técnica local
./iso-cli check --backup-dir /var/backups --log-dir /var/log --max-backup-age 7

# Validar documentación (detecta variables sin rellenar)
./iso-cli validate-docs --path ./docs/

# Exportar estado de cumplimiento
./iso-cli export-soa --input SOA_Matrix.json --output reporte-cumplimiento.json
```

#### Ejemplo de Salida

```
$ ./iso-cli check --backup-dir /var/backups --max-backup-age 7

╔══════════════════════════════════════════════════════════╗
║           ISO 27001 — Auditoría Técnica Local           ║
╠══════════════════════════════════════════════════════════╣
║  [✓] Backup más reciente: 2 días (máx. permitido: 7)   ║
║  [✓] Logs de acceso presentes y no vacíos               ║
║  [✗] Permisos de /var/backups: 0777 (esperado: 0750)   ║
║  [✓] Espacio en disco suficiente (>20%)                 ║
╠══════════════════════════════════════════════════════════╣
║  Resultado: 3/4 controles aprobados — 1 hallazgo        ║
╚══════════════════════════════════════════════════════════╝
```

---

## English

### What is this project?

A **"Compliance as Code"** framework that combines information security governance documentation with a **Go** CLI tool to automate technical audits, designed specifically for **small and medium-sized enterprises (SMEs)** seeking alignment with **ISO/IEC 27001:2022**.

### Why does it exist?

Most SMEs lack a dedicated security team. This repository delivers:

- **Ready-to-use policies** based on real Annex A controls from ISO 27001:2022
- **CLI tool** that automates technical compliance verification
- **SoA Matrix** (Statement of Applicability) in JSON format for traceability
- **Real anonymized case study** as an incident response reference
- **Bilingual documentation** (ES/EN) for multinational teams

### Quick Start

```bash
git clone https://github.com/jsortizq/politicas-iso27001-pyme.git
cd politicas-iso27001-pyme
go build -o iso-cli ./cmd/iso-cli/
```

### 4-Week Adoption Roadmap

| Week | Activity | Deliverable |
|------|----------|-------------|
| **1** | Policy review and customization (`[EMPRESA]` → your company name) | Policies approved by Management |
| **2** | Technical implementation: passwords, backups, SPF/DKIM/DMARC | Active operational controls |
| **3** | Run `iso-cli check` against infrastructure | Technical gap report |
| **4** | Review findings, remediation plan and audit simulation | Updated SoA Matrix + evidence |

### CLI Commands

```bash
# Local technical audit
./iso-cli check --backup-dir /var/backups --log-dir /var/log --max-backup-age 7

# Validate documentation (detects unfilled variables)
./iso-cli validate-docs --path ./docs/

# Export compliance status
./iso-cli export-soa --input SOA_Matrix.json --output compliance-report.json
```

---

## Contribuir / Contributing

Las contribuciones son bienvenidas. Por favor abre un issue antes de enviar un PR.

Contributions are welcome. Please open an issue before submitting a PR.

## Licencia / License

Este proyecto está licenciado bajo la [Licencia MIT](LICENSE).

This project is licensed under the [MIT License](LICENSE).

---

<div align="center">

**Desarrollado por [jsortizq](https://github.com/jsortizq)** · ISO/IEC 27001:2022 Compliance Framework

</div>
