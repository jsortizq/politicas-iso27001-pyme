---
title: "Password Control Policy"
version: "1.0"
date: "2026-11-11"
iso_control: "A.8.5 — Secure Authentication"
status: "approved"
classification: "internal"
owner: "[SECURITY_OFFICER]"
next_review: "2027-05-11"
---

# POL-SEC-001: Password Control Policy

## 1. Objective

To establish the minimum requirements for password complexity, rotation, and management across all information systems of **[EMPRESA]**, in order to reduce the risk of unauthorized access and ensure secure user authentication in accordance with control **A.8.5** of ISO/IEC 27001:2022.

## 2. Scope

This policy applies to:

- All employees, contractors, interns, and third parties with access to [EMPRESA]'s information systems.
- All user accounts on operating systems, corporate applications, databases, cloud services, email, VPN, and any credential-protected resource.
- Service accounts, administrative accounts, and privileged accounts (PAM).

## 3. Definitions

| Term | Definition |
|------|------------|
| **Password** | A secret character string used as an authentication factor to verify a user's identity. |
| **Privileged account** | An account with elevated administrative permissions over systems, networks, or databases. |
| **MFA** | Multi-factor authentication; the use of two or more independent verification factors (something you know, something you have, something you are). |
| **Password history** | A record of previous passwords associated with an account to prevent their reuse. |
| **Account lockout** | Temporary disabling of account access after exceeding the failed authentication attempt threshold. |
| **GPO** | Group Policy Object; a policy object in Active Directory environments for centralized policy enforcement. |

## 4. Technical Requirements

### 4.1 Password Complexity

Every password **MUST** simultaneously meet the following criteria:

| Parameter | Requirement |
|-----------|-------------|
| Minimum length | **8 characters** |
| Uppercase letters | At least **1** uppercase character (A–Z) |
| Lowercase letters | At least **1** lowercase character (a–z) |
| Numeric digits | At least **1** digit (0–9) |
| Special characters | At least **1** special character (`! @ # $ % ^ & * ( ) _ + - = [ ] { }`) |
| Username | The password **MUST NOT** contain the username or parts of the account holder's full name |
| Dictionary words | It is **recommended** NOT to use complete, unaltered dictionary words |

**Valid examples:** `C0mpl!ance2026`, `$ecuR1ty_IT`
**Invalid examples:** `password`, `12345678`, `company2026`

### 4.2 Rotation and Validity

| Parameter | Value |
|-----------|-------|
| Maximum password age | **45 calendar days** |
| Minimum password age | **1 day** (prevents consecutive changes to circumvent history) |
| Expiration notification | **7 days** before expiry, the system will notify the user |
| Post-expiration grace period | **0 days** — an expired password forces an immediate change at the next login |

### 4.3 History and Reuse

| Parameter | Value |
|-----------|-------|
| Passwords retained in history | **Last 5 passwords** |
| Reuse | **PROHIBITED** — none of the 5 previous passwords may be reused |
| Similarity | It is recommended that the new password differ by at least **3 characters** from the previous one |

### 4.4 Account Lockout on Failed Attempts

| Parameter | Value |
|-----------|-------|
| Failed attempt threshold | **7 consecutive attempts** |
| Lockout duration | **30 minutes** (automatic unlock) |
| Counter reset | After a successful authentication or after **30 minutes** with no attempts |
| Administrator accounts | **Permanent** lockout until manually unlocked by the IT team |

### 4.5 Multi-Factor Authentication (MFA)

MFA implementation is **mandatory** for:

- Access to server and firewall administration panels
- Remote access via corporate VPN
- Cloud service administration consoles (AWS, Azure, GCP)
- Email accounts with administrator permissions
- Financial and accounting management systems

For all other systems, MFA implementation is **recommended** and will be evaluated in the ISMS continuous improvement plan.

### 4.6 Storage and Transmission

- Passwords **MUST NEVER** be stored in plain text. Hashing with secure algorithms is required: **bcrypt**, **scrypt**, or **Argon2id**.
- Passwords in transit must be transmitted exclusively over encrypted channels (**TLS 1.2** or higher).
- Storing passwords in spreadsheets, text documents, emails, sticky notes, or any unencrypted medium is **PROHIBITED**.
- Use of IT-approved corporate password managers (e.g., Bitwarden, 1Password Business) is **permitted and recommended**.

### 4.7 Service Accounts

- Service account passwords must have a minimum length of **16 characters**.
- Rotation occurs every **90 days** or immediately upon departure of any person with knowledge of the credential.
- They must be stored in a secrets manager (e.g., HashiCorp Vault, AWS Secrets Manager).

## 5. Responsibilities

| Role | Responsibility |
|------|----------------|
| **Users** | Comply with complexity requirements, never share passwords, report suspected compromises |
| **IT Team** | Implement technical configurations (GPO, IAM policies), monitor account lockouts, and manage unlocks |
| **Security Officer** | Audit compliance with this policy quarterly, review lockout logs, update parameters based on evolving threats |
| **Management** | Approve the policy, allocate resources for implementation, and promote a security culture |
| **Human Resources** | Incorporate this policy into new employee onboarding and service agreements |

## 6. Exceptions

Any exception to this policy must:

1. Be requested in writing to the Security Officer.
2. Include a detailed technical justification and risk analysis.
3. Be approved by Management and the ISMS Officer.
4. Have a maximum validity of **6 months**, after which it must be reassessed.
5. Be recorded in the ISMS Exceptions Register along with its compensating control.

## 7. Compliance and Sanctions

Non-compliance with this policy may result in:

- **First offense:** Written notification and mandatory security awareness training.
- **Second offense:** Temporary suspension of system access until a security assessment is completed.
- **Repeated or serious offenses:** Disciplinary action in accordance with internal labor regulations, which may include contract termination.
- **Third parties and contractors:** Termination of the service agreement and review of granted access.

In all cases, the incident will be recorded in the ISMS incident management system.

## 8. Normative References

- ISO/IEC 27001:2022 — Control A.8.5: Secure Authentication
- ISO/IEC 27002:2022 — Section 8.5: Implementation Guidance
- NIST SP 800-63B — Digital Identity Guidelines: Authentication
- CIS Controls v8 — Control 5: Account Management
- [EMPRESA] Internal Labor Regulations

## 9. Revision History

| Version | Date | Author | Change Description |
|---------|------|--------|--------------------|
| 1.0 | 2026-11-11 | [SECURITY_OFFICER] | Initial document creation |
| — | 2027-05-11 | — | Next scheduled review |

---

**Approved by:** [MANAGEMENT_NAME] — General Management of [EMPRESA]

**Approval date:** 2026-11-11

**Classification:** Internal Use
