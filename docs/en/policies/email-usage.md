---
title: "Email Usage Policy"
version: "1.0"
date: "2026-11-11"
iso_control: "A.5.10"
status: "approved"
classification: "internal"
owner: "[IT_MANAGER]"
approved_by: "[EXECUTIVE_DIRECTOR]"
---

# Email Usage Policy

## 1. Objective

To establish the rules, technical controls, and responsibilities for the use of **[EMPRESA]**'s corporate email service, in order to protect the confidentiality, integrity, and availability of transmitted information, prevent misuse of the system, and avoid the inclusion of corporate IP addresses and domains in antispam reputation blacklists.

## 2. Scope

This policy applies to:

- All employees, contractors, interns, and authorized third parties who use [EMPRESA]'s corporate email.
- All email accounts under the corporate domain `@[EMPRESA].com` and associated subdomains.
- Mail servers (MTA/MDA), security gateways, and cloud email services used by the organization.
- Corporate and personal devices (BYOD) from which corporate email is accessed.

## 3. Definitions

| Term | Definition |
|------|-----------|
| **SPF** (Sender Policy Framework) | DNS TXT record that authorizes which servers can send email on behalf of the domain. |
| **DKIM** (DomainKeys Identified Mail) | Cryptographic signature added to email headers that verifies sender authenticity and message integrity. |
| **DMARC** (Domain-based Message Authentication, Reporting & Conformance) | DNS policy that instructs receiving servers on how to handle emails that fail SPF and/or DKIM validation, including reporting mechanisms. |
| **Blacklist** | Public database that records IP addresses or domains identified as sources of spam or malicious activity. Examples: Spamhaus ZEN, FabelSources, UceprotectL3. |
| **MTA** | Mail Transfer Agent — server responsible for sending and routing email. |
| **Bulk sending** | Distribution of email to more than 50 simultaneous recipients or more than 200 messages per hour from a single account or server. |
| **Phishing** | Social engineering technique that impersonates legitimate identities to obtain credentials or confidential information. |

## 4. Technical Infrastructure Requirements

### 4.1. Mandatory DNS Records

The IT team **must** maintain and verify quarterly the following DNS records for each [EMPRESA] mail domain:

#### SPF (Sender Policy Framework)

```dns
[EMPRESA].com.  IN  TXT  "v=spf1 include:_spf.[PROVIDER].com ip4:[SMTP_SERVER_IP] -all"
```

- The `-all` qualifier (hard fail) **must** be used to reject emails from unauthorized servers.
- The use of `+all` or `?all` in production is prohibited.
- The SPF record must not exceed 10 DNS lookups per RFC 7208.

#### DKIM (DomainKeys Identified Mail)

```dns
selector._domainkey.[EMPRESA].com.  IN  TXT  "v=DKIM1; k=rsa; p=[BASE64_PUBLIC_KEY]"
```

- Minimum RSA key length: **2048 bits**.
- DKIM key rotation: every **6 months** at most.
- At minimum, the following headers must be signed: `From`, `To`, `Subject`, `Date`, `Message-ID`.

#### DMARC (Domain-based Message Authentication, Reporting & Conformance)

```dns
_dmarc.[EMPRESA].com.  IN  TXT  "v=DMARC1; p=quarantine; rua=mailto:dmarc-reports@[EMPRESA].com; ruf=mailto:dmarc-forensic@[EMPRESA].com; pct=100; adkim=s; aspf=s"
```

- The **minimum** acceptable policy is `p=quarantine`. Migration to `p=reject` is recommended once legitimate mail flow has been verified.
- Aggregate reports (`rua`) must be reviewed **weekly** by the IT team.
- Strict alignment (`adkim=s; aspf=s`) is mandatory.

### 4.2. Blacklist Protection

[EMPRESA] acknowledges that the inclusion of its IPs or domains in antispam blacklists has a severe operational impact. The following lists are actively monitored:

| Blacklist | Type | Impact |
|-----------|------|--------|
| **Spamhaus ZEN** | Composite (SBL+XBL+PBL+DBL) | Critical — used by 80%+ of mail servers worldwide |
| **FabelSources** | IP and domain | High — blocks deliveries to European and Latin American providers |
| **UceprotectL3** | Full network range | Severe — can block entire /24 ranges affecting third parties |

**Preventive controls:**

- Automated daily monitoring of outbound IP status against at least the 3 lists above.
- Per-account sending limit: maximum **100 messages per hour** and **500 per day** for standard accounts.
- Service accounts (notifications, billing) must use dedicated sending IPs separate from user accounts.
- Rate-limiting enabled on the MTA to prevent accidental mass mailings.

### 4.3. System Technical Limits

| Parameter | Value |
|-----------|-------|
| Maximum attachment size | **25 MB** per message |
| Maximum mailbox size | **10 GB** for standard users, **25 GB** for executives |
| Inbox retention | **12 months** — older emails must be archived |
| Deleted email retention | **30 days** in trash before automatic purge |
| Maximum concurrent sessions | **5** per account |
| Inactive session timeout | **30 minutes** |

## 5. Usage Rules

### 5.1. Acceptable Use

- Corporate email is a work tool and must be used exclusively for [EMPRESA] business activities.
- Any email containing **information classified as confidential or restricted** must be sent encrypted:
  - Sensitive attachments: password-protected compressed archive (ZIP/AES-256) with password communicated via a separate channel (phone, encrypted messaging).
  - Alternative: S/MIME encryption with a digital certificate issued by the corporate PKI or a trusted CA.
- Users must verify recipients before sending, especially in replies to threads with multiple participants.
- The **standard corporate signature** and **confidentiality notice** approved by [EMPRESA] must be included at the bottom of every external message.

### 5.2. Mailbox Maintenance

- Users are responsible for keeping their mailbox below the storage limit.
- Emails older than **12 months** must be archived in the corporate archiving solution or deleted if they have no legal or business retention value.
- IT will execute a quarterly cleanup campaign with 15-day prior notification.

## 6. Specific Prohibitions

The following are **strictly prohibited**:

1. **Unauthorized bulk sending**: Distribution of emails to more than 50 external recipients without written authorization from the Marketing department and IT oversight. This includes newsletters, promotions, mass invitations, and chain emails.
2. **Personal commercial use**: Using corporate email for personal businesses, sales, third-party advertising, or activities unrelated to [EMPRESA].
3. **Automatic forwarding to external accounts**: Configuring forwarding rules to personal domains (Gmail, Outlook.com, Yahoo, etc.) without Information Security authorization.
4. **Identity spoofing**: Modifying headers, aliases, or "From" fields to impersonate another user or entity.
5. **Malware transmission**: Sending executable files (.exe, .bat, .ps1, .vbs, .js) or macro-enabled files without encryption and without documented justification.
6. **Control evasion**: Attempting to bypass antispam filters, DLP (Data Loss Prevention), or security gateways through encoding, obfuscation, or attachment splitting.
7. **Credential storage**: Storing passwords, access tokens, API keys, or private certificates in the email body or in drafts.

## 7. Email Incident Management

### 7.1. Phishing and Spam Reporting

Any user who receives a suspected phishing or spam email must:

1. **Not open** links or attachments in the suspicious message.
2. **Not forward** the email to other users.
3. Use the **"Report phishing"** button in the email client (if available) or forward as an attachment (.eml) to: `security@[EMPRESA].com`.
4. Notify the IT team via an alternate channel (corporate chat or phone) if the email requests urgent financial actions.

### 7.2. Blacklist Inclusion Response

If an [EMPRESA] IP or domain is detected on a blacklist:

1. **Detection** (SLA: < 4 hours): The monitoring system alerts the IT team.
2. **Containment** (SLA: < 8 hours): Identify and block the abusive sending source. Isolate the compromised account or server.
3. **Remediation** (SLA: < 24 hours): Request delisting from the corresponding blacklist. Document the root cause.
4. **Post-mortem** (SLA: < 72 hours): Generate a technical report with timeline, impact, root cause, and preventive actions.

Reference: see the documented case in `docs/en/case-studies/blacklist-recovery.md`.

### 7.3. Account Compromise

If a corporate email account is suspected to be compromised:

1. Immediate password change for the affected account.
2. Revocation of all active sessions and application tokens.
3. Review of forwarding rules and account settings.
4. Analysis of the last 30 days of account activity (access logs, sent emails).
5. Notification to contacts who may have received fraudulent emails from the account.

## 8. Responsibilities

| Role | Responsibility |
|------|---------------|
| **All users** | Comply with this policy, report incidents, maintain their mailbox, and refrain from unauthorized bulk sending. |
| **IT Team** | Administer the email infrastructure, maintain SPF/DKIM/DMARC, monitor blacklists, enforce technical limits, and respond to incidents. |
| **Information Security Officer** | Define and update this policy, audit compliance, and investigate email-related security incidents. |
| **Management** | Approve this policy, allocate resources for its implementation, and support disciplinary actions arising from violations. |
| **Marketing / Communications** | Coordinate with IT for authorized bulk mailings, using dedicated platforms (Mailchimp, SendGrid, etc.) with independent IPs. |

## 9. Non-Compliance Sanctions

Violation of this policy may result in:

- **Minor** (first offense without impact): Verbal warning and mandatory training.
- **Moderate** (repeat offense or minor impact): Written warning and temporary restriction of email functionalities.
- **Severe** (bulk sending causing blacklisting, data breach, or account compromise due to negligence): Email account suspension, formal disciplinary proceedings per [EMPRESA]'s internal regulations, and potential legal action.

## 10. Review and Updates

This policy shall be reviewed at least **once per year** or upon:

- Changes to the email infrastructure.
- Significant email-related security incidents.
- Modifications to ISO/IEC 27001:2022 or applicable legislation.
- Changes in the email service provider.

## 11. Revision History

| Version | Date | Author | Change Description |
|---------|------|--------|--------------------|
| 1.0 | 2026-11-11 | [SECURITY_OFFICER] | Initial policy release |

---

**Classification:** Internal  
**ISO 27001:2022 Control:** A.5.10 — Acceptable use of information and other associated assets  
**Document:** POL-EMAIL-001  
**Next review:** 2027-11-11
