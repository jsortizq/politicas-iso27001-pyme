---
title: "Backup Management Policy"
version: "1.0"
date: "2026-11-11"
iso_control: "A.8.13"
status: "approved"
classification: "internal"
owner: "[IT_MANAGER]"
approved_by: "[EXECUTIVE_DIRECTOR]"
next_review: "2027-05-11"
---

# Backup Management Policy

**Organization:** [EMPRESA]
**Code:** POL-BKP-001 | **Version:** 1.0 | **Classification:** Internal Use

---

## 1. Objective

To establish the requirements, procedures, and responsibilities for creating, storing, verifying, and restoring backups of [EMPRESA]'s information, ensuring the availability, integrity, and confidentiality of data in the event of loss, corruption, or disaster.

## 2. Scope

This policy applies to:

- All production information systems at [EMPRESA], including physical and virtual servers, databases, business-critical applications, and cloud services.
- All shared data repositories (file servers, NAS, corporate cloud storage).
- Workstations containing information classified as **confidential** or **restricted**.
- Network device configurations (routers, switches, firewalls).
- All IT personnel responsible for infrastructure administration and any third party managing backups under a service contract.

## 3. Definitions

| Term | Definition |
|------|-----------|
| **Incremental backup** | A copy that records only the changes made since the last backup (incremental or full). |
| **Full backup** | A complete copy of all data in a system or volume at a given point in time. |
| **RPO (Recovery Point Objective)** | The maximum amount of data the organization can afford to lose, measured in time. |
| **RTO (Recovery Time Objective)** | The maximum acceptable time to restore a service after a disruption. |
| **3-2-1 Rule** | A backup strategy requiring at least 3 copies of data, on 2 different media types, with 1 copy stored offsite. |
| **Test restoration** | A planned process of recovering data from a backup to validate its integrity and functionality. |
| **Encryption at rest** | Cryptographic protection applied to stored data to prevent unauthorized reading without the proper key. |

## 4. Technical Requirements

### 4.1 Backup Strategy — 3-2-1 Rule

All critical data at [EMPRESA] must comply with the 3-2-1 rule:

1. **3 copies** of each data set: the production copy plus two backup copies.
2. **2 different media types**: for example, local disk storage and cloud storage or tape.
3. **1 offsite copy**: stored at a geographically separate location (remote data center, cloud provider, or physical vault).

### 4.2 Backup Frequency

| Backup Type | Frequency | Execution Window | Retention |
|-------------|-----------|------------------|-----------|
| **Incremental** | Daily (Monday–Saturday) | 22:00–04:00 (local time) | 7 days |
| **Full (weekly)** | Weekly (Sundays) | 00:00–08:00 (local time) | 4 weeks |
| **Full (monthly)** | First Sunday of each month | 00:00–10:00 (local time) | 12 months |
| **Full (annual)** | December 31 | As available | 5 years (or as required by law) |

Schedules may be adjusted based on system load, provided the established frequency is maintained and any changes are documented.

### 4.3 Recovery Objectives

| Parameter | Target | Justification |
|-----------|--------|---------------|
| **RPO** | ≤ 24 hours | Maximum acceptable loss is one business day's production. |
| **RTO** | ≤ 4 hours for critical systems; ≤ 24 hours for non-critical | Based on [EMPRESA]'s Business Impact Analysis (BIA). |

These values must be reviewed annually or after significant infrastructure changes.

### 4.4 Encryption and Protection

- All backups must be encrypted at rest using **AES-256** as the minimum algorithm.
- Backups transmitted to remote sites must travel over encrypted channels (**TLS 1.2+** or **IPSec/IKEv2** VPN tunnel).
- Encryption keys must be stored separately from the backup data, in a key management system (KMS) or secure vault. Never on the same media as the backed-up data.
- Access to backups is restricted exclusively to authorized IT personnel through role-based access control (RBAC).

### 4.5 Offsite Storage

- The offsite copy must be located at a facility at least **50 km** from the primary site, or in a different geographic region if a cloud provider is used.
- The remote storage provider must meet [EMPRESA]'s security requirements and preferably hold **ISO 27001** or **SOC 2 Type II** certification.
- Transfer to the remote site must be completed within **6 hours** of backup completion.

### 4.6 Monitoring and Alerts

- All backup jobs must be monitored automatically.
- Backup failures must trigger an immediate alert (email or ticketing system) to the IT team.
- Failed jobs must be restarted manually or automatically within **2 hours** of failure detection.
- A log of all executed backup jobs must be maintained, including: date, start time, end time, size, result (success/failure), and backup type.

## 5. Test Restoration Procedure

### 5.1 Frequency

At least **one random test restoration must be performed every 30 calendar days**. This restoration must:

- Randomly select a different system or data set each time.
- Cover all critical systems at least **once per quarter**.
- Be fully documented in the **Test Restoration Register**.

### 5.2 Procedure

1. **Selection:** The IT manager randomly selects (or via scheduled rotation) the system and backup copy to restore.
2. **Preparation:** An isolated environment (sandbox or test server) is provisioned that does not affect production.
3. **Execution:** The selected backup is restored in the test environment.
4. **Validation:** The following is verified:
   - Data integrity (SHA-256 checksums compared with the original backup).
   - Application functionality or accessibility of restored files.
   - That the restoration time falls within the defined RTO.
5. **Documentation:** The test form records:
   - Test date.
   - Restored system and backup date used.
   - Total restoration time.
   - Result: **successful** / **successful with observations** / **failed**.
   - Corrective actions if applicable.
6. **Escalation:** If the restoration fails, a **high-priority** incident ticket is opened and a new full backup of the affected system is executed within 24 hours.

### 5.3 Success Criteria

A test is considered successful when:

- Restored data is identical to the backup point (checksum verification).
- Dependent applications function correctly with the restored data.
- Restoration time does not exceed the established RTO.

## 6. Systems in Scope

As a minimum reference, the following systems must have mandatory backups:

- Database servers (SQL Server, PostgreSQL, MySQL, MongoDB).
- Application servers and web portals.
- Email server and its storage.
- Active Directory / LDAP and GPO configurations.
- Shared files (file server, NAS, SharePoint, corporate OneDrive).
- Network configurations (routers, switches, firewalls).
- Complete virtual machines (hypervisor-level snapshots where applicable).
- Source code repositories.

## 7. Responsibilities

| Role | Responsibility |
|------|---------------|
| **Executive Management** | Approve this policy and allocate necessary resources for compliance. |
| **ISMS Manager** | Oversee compliance with this policy within the Information Security Management System framework. |
| **IT Manager** | Implement, operate, and monitor the backup infrastructure. Execute monthly test restorations. |
| **System Administrators** | Configure backup jobs according to this policy's parameters. Respond to failure alerts. |
| **All Employees** | Store corporate information in designated locations to ensure inclusion in backups. |
| **External Providers** | Comply with encryption and retention requirements if managing backups under contract. |

## 8. Non-Compliance

Non-compliance with this policy may result in:

- Disciplinary action in accordance with [EMPRESA]'s internal regulations.
- Review of the involved personnel's access and privileges.
- Notification to Executive Management if non-compliance poses a material risk to the organization.

## 9. Exceptions

All exceptions to this policy must:

1. Be requested in writing to the ISMS Manager.
2. Include a business justification and associated risk analysis.
3. Be approved by Executive Management.
4. Be documented in the exception register with an expiration date (maximum 6 months, renewable).
5. Be reviewed in the next internal audit.

## 10. Normative References

- ISO/IEC 27001:2022 — Control A.8.13 (Information backup)
- ISO/IEC 27002:2022 — Section 8.13 (Implementation guidance)
- NIST SP 800-34 Rev. 1 — Contingency Planning Guide

## 11. Revision History

| Version | Date | Author | Change Description |
|---------|------|--------|-------------------|
| 1.0 | 2026-11-11 | [IT_MANAGER] | Initial document creation |
| — | — | — | — |

---

*This document is the property of [EMPRESA] and is classified as **internal use**. Distribution outside the organization requires express authorization from Executive Management.*
