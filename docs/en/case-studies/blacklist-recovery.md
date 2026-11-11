---
title: "Post-Mortem: Antispam Blacklist Recovery"
version: "1.0"
date: "2026-11-11"
severity: critical
incident_id: "INC-2026-0047"
iso_controls:
  - A.5.24
  - A.5.26
  - A.8.21
classification: internal
author: "[SECURITY_OFFICER]"
approved_by: "[MANAGEMENT_NAME]"
---

# Post-Mortem: Antispam Blacklist Recovery

> **Classification:** Internal — Do not distribute outside [EMPRESA]  
> **Severity:** CRITICAL  
> **Status:** Closed — Preventive measures implemented

---

## 1. Executive Summary

This document details the security incident in which the IP address of [EMPRESA]'s mail server was listed on multiple antispam blacklists (Spamhaus ZEN, Barracuda BRBL, and SpamCop) due to the compromise of an internal email account used for mass spam distribution.

The incident was detected following a notification from [SOCIO_COMERCIAL], who reported that all emails from [EMPRESA] were being rejected by their servers. The total email service disruption with key partners lasted over **72 hours**, causing an estimated operational and financial impact of **$15,000 USD**.

Full resolution required **7 calendar days** from detection to complete restoration of email service.

---

## 2. Incident Data

| Field | Detail |
|-------|--------|
| **Date detected** | 2026-03-15 at 09:47 (UTC-5) |
| **Detection method** | External notification from [SOCIO_COMERCIAL] |
| **Date resolved** | 2026-03-22 at 14:30 (UTC-5) |
| **Total duration** | 7 calendar days |
| **Affected systems** | Mail server (mail.empresa.com), public DNS |
| **Compromised accounts** | 1 (sales department user) |
| **Blacklists affected** | Spamhaus ZEN, Barracuda BRBL, SpamCop |
| **Classification** | Credential compromise + spam distribution |

---

## 3. Detailed Timeline

### Day 0 — Detection (March 15)

| Time | Event |
|------|-------|
| 09:47 | [SOCIO_COMERCIAL] sends email to the Commercial Director reporting that messages from [EMPRESA] are being rejected with code `550 5.7.1 Service unavailable; client host blocked using zen.spamhaus.org` |
| 10:15 | Commercial Director escalates to the IT team |
| 10:30 | IT confirms that IP `203.0.113.50` of the mail server is listed on Spamhaus ZEN |
| 10:45 | Additional verification reveals listings on Barracuda BRBL and SpamCop |
| 11:00 | CRITICAL severity incident is declared and response team is activated |
| 11:30 | Initial review of mail server logs: **47,000 outbound messages** detected in the last 48 hours from account `comercial3@empresa.com` |
| 12:00 | Senior Management notified of the incident and estimated impact |

### Day 1 — Containment (March 16)

| Time | Event |
|------|-------|
| 08:00 | Forensic analysis of `comercial3@empresa.com`: last legitimate access 5 days prior; subsequent access from IP `45.xx.xx.xx` (geolocation: Eastern Europe) |
| 08:30 | Compromised account disabled |
| 09:00 | Forced password reset for the compromised account |
| 09:30 | Review of victim's mailbox: phishing email identified received on March 10, impersonating a Microsoft 365 notification with a link to a credential harvesting form |
| 10:00 | Attacker source IPs blocked at the perimeter firewall |
| 11:00 | Temporary sending restriction configured: maximum 50 emails per hour per account |
| 14:00 | Formal notification sent to [SOCIO_COMERCIAL] about the incident and actions taken |
| 16:00 | Alternative communication channels established (phone, WhatsApp Business) with top 5 commercial partners |

### Day 2 — IP Change (March 17)

| Time | Event |
|------|-------|
| 09:00 | Request submitted to ISP for a new public IP address for the mail server |
| 14:00 | ISP assigns new IP `203.0.113.75` |
| 15:00 | Mail server reconfigured with the new IP |
| 16:00 | New IP verification against blacklists: **clean** across all major lists |
| 17:00 | Limited send tests with the new IP (10 emails to internal test accounts) |

### Day 3 — DNS Hardening (March 18)

| Time | Event |
|------|-------|
| 08:00 | Complete DNS audit of the `empresa.com` domain |
| 09:00 | **Critical finding:** SPF record contained `v=spf1 ip4:203.0.113.0/24 +all` — the `+all` mechanism nullified all protection, allowing any IP to send email on behalf of the domain |
| 09:30 | SPF record updated: `v=spf1 ip4:203.0.113.75 include:_spf.google.com -all` |
| 10:00 | DKIM key rotation: new 2048-bit RSA key pair generated, TXT record published at `selector2026._domainkey.empresa.com` |
| 11:00 | DMARC configured for the first time: `v=DMARC1; p=quarantine; rua=mailto:dmarc-reports@empresa.com; ruf=mailto:dmarc-forensics@empresa.com; pct=100` |
| 12:00 | PTR record (reverse DNS) configured for the new IP |
| 14:00 | Verification with external tools: MXToolbox, DMARCian, mail-tester.com — score 9/10 |

### Day 4 — Delisting Requests (March 19)

| Time | Event |
|------|-------|
| 08:00 | Delisting request submitted to Spamhaus ZEN for old IP `203.0.113.50` |
| 08:30 | Delisting request submitted to Barracuda BRBL |
| 09:00 | Delisting request submitted to SpamCop |
| 10:00 | Secondary list check: SORBS, UCEPROTECT Level 1, Invaluement — old IP listed on UCEPROTECT L1 |
| 10:30 | Delisting request submitted to UCEPROTECT |
| 14:00 | Spamhaus confirms removal of old IP |
| 16:00 | Barracuda confirms removal |

### Day 5 — Network Scan and Verification (March 20)

| Time | Event |
|------|-------|
| 08:00 | Full internal network scan with `nmap -sV -sC -O 192.168.1.0/24` to identify other potentially compromised machines |
| 09:30 | Antimalware scan (ESET Endpoint) on all workstations (32 machines) |
| 12:00 | Scan results: **2 workstations** with potentially unwanted programs (PUP), **0 additional active malware** |
| 13:00 | Cleanup of the 2 identified workstations |
| 14:00 | Verification that no other accounts show anomalous sending activity |
| 15:00 | SpamCop confirms removal of old IP |

### Day 7 — Full Restoration (March 22)

| Time | Event |
|------|-------|
| 08:00 | Send tests to top 5 commercial partners |
| 09:00 | [SOCIO_COMERCIAL] confirms correct email reception |
| 10:00 | MXToolbox verification: new IP with no listings, DNS score 10/10 |
| 14:00 | Official notification to Management that service is fully restored |
| 14:30 | Incident closed with status **RESOLVED** |

---

## 4. Root Cause Analysis

### 4.1 Direct Cause

A sales department employee fell victim to a phishing attack that replicated the Microsoft 365 login page. The captured credentials were used by the attacker to send approximately **47,000 spam emails** over a 48-hour period.

### 4.2 Contributing Causes

| # | Cause | Contribution Level |
|---|-------|--------------------|
| 1 | **No MFA** on the email system: credentials alone were sufficient for access | HIGH |
| 2 | **Permissive SPF record** with `+all` mechanism that effectively invalidated all SPF protection | HIGH |
| 3 | **No DMARC policy** that would have enabled detection and rejection of unauthenticated emails | HIGH |
| 4 | **No outbound rate limiting** per account: a single account was able to send 47,000 messages without triggering alerts | MEDIUM |
| 5 | **No outbound email volume monitoring**: no alerts existed for anomalous sending spikes | MEDIUM |
| 6 | **Insufficient staff training** in phishing identification | MEDIUM |

### 4.3 Ishikawa Diagram (Summary)

```
People              Processes              Technology
  │                    │                       │
  ├─ No anti-        ├─ No documented       ├─ No MFA
  │  phishing          │  email policy         │
  │  training          │                       ├─ SPF with +all
  │                    │                       │
  └─ Weak            ├─ No outbound         ├─ No DMARC
     credentials      │  monitoring           │
                      │                       └─ No rate-limit
                      └─ No incident
                         response plan
```

---

## 5. Impact

### 5.1 Operational Impact

- **Email blocked:** over 72 hours without email communication with key partners
- **Purchase orders delayed:** 3 orders totaling $15,000 USD
- **Productivity:** sales team (5 people) partially unproductive for 3 days

### 5.2 Reputational Impact

- Temporary loss of trust with [SOCIO_COMERCIAL] and 4 other partners
- Formal incident explanation required to commercial partners
- Perception of vulnerability among clients

### 5.3 Estimated Financial Impact

| Item | Amount (USD) |
|------|-------------|
| Delayed orders | $15,000 |
| IT staff hours (incident response, 80 hours) | $2,400 |
| New public IP (ISP charge) | $150 |
| Diagnostic and monitoring tools | $500 |
| **Total estimated** | **$18,050** |

---

## 6. Lessons Learned

1. **Single-factor authentication is insufficient.** A compromised password was enough to cause a critical-severity incident. MFA would have completely prevented unauthorized access.

2. **Email DNS records are the first line of defense.** An SPF record with `+all` is functionally equivalent to having no SPF at all. Correct SPF, DKIM, and DMARC configuration must be validated by qualified personnel.

3. **Proactive monitoring is cheaper than reaction.** Had an alert for anomalous outbound email volume existed, the incident would have been detected in hours, not days.

4. **Staff training is not optional.** The attack vector was a simple phishing email. Quarterly awareness sessions significantly reduce this risk.

5. **A documented incident response plan accelerates recovery.** The response was ad-hoc; an established procedure would have reduced resolution time.

---

## 7. Preventive Measures Implemented

### 7.1 Immediate Actions (completed)

- [x] MFA activation (Microsoft Authenticator) for all email accounts
- [x] SPF record corrected to strict mode (`-all`)
- [x] DMARC implementation with `p=quarantine` policy
- [x] DKIM key rotation to RSA 2048 bits
- [x] Rate-limiting configured: maximum 200 emails per hour per account
- [x] Automatic alerts when an account exceeds 100 emails in one hour
- [x] Forced password reset for all email accounts

### 7.2 Short-Term Actions (30 days)

- [x] Mandatory anti-phishing training for all staff
- [x] Internal phishing simulation using GoPhish
- [x] Email Usage Policy documentation (see `docs/en/policies/email-usage.md`)
- [x] Subscription to IP reputation monitoring service (MXToolbox Monitoring)

### 7.3 Medium-Term Actions (90 days)

- [x] DMARC migration from `p=quarantine` to `p=reject` after 60 days of monitoring with no false positives
- [x] Implementation of email gateway with outbound content analysis
- [x] Quarterly DNS email record audit
- [ ] Evaluation of SIEM solution for security event correlation

---

## 8. ISO 27001:2022 Control Verification

| Control | Description | Pre-Incident Status | Post-Incident Status |
|---------|-------------|--------------------|-----------------------|
| A.5.24 | Incident management planning | ❌ Did not exist | ✅ Documented procedure |
| A.5.26 | Incident response | ❌ Ad-hoc | ✅ Formal plan with roles and escalation |
| A.8.5 | Secure authentication | ⚠️ Password only | ✅ Mandatory MFA |
| A.8.21 | Security of network services | ❌ Permissive SPF, no DMARC | ✅ Strict SPF + DKIM + DMARC |

---

## 9. Approval and Distribution

| Role | Name | Date |
|------|------|------|
| Prepared by | [SECURITY_OFFICER] | 2026-03-25 |
| Reviewed by | [IT_MANAGER] | 2026-03-26 |
| Approved by | [MANAGEMENT_NAME] | 2026-03-28 |

**Authorized distribution:** Senior Management, Security Officer, IT Manager, Internal Audit.

---

## 10. Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-03-28 | [SECURITY_OFFICER] | Initial post-mortem version |
| 1.1 | 2026-06-15 | [SECURITY_OFFICER] | Medium-term measures status update |
