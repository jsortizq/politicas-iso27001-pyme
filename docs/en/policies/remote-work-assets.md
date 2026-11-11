---
title: "Remote Work, Asset Management and Clean Desk Policy"
version: "1.0"
date: "2026-11-11"
iso_control: "A.6.7 / A.7.7 / A.8.1"
status: "approved"
classification: "internal"
author: "[SECURITY_OFFICER]"
approved_by: "[EXECUTIVE_MANAGEMENT]"
---

# Remote Work, Asset Management and Clean Desk Policy

## 1. Objective

To establish the information security requirements that must be met when **[EMPRESA]** personnel perform work activities outside corporate facilities, as well as to define guidelines for the secure management of information assets and the enforcement of clean desk and clean screen practices, in accordance with controls **A.6.7**, **A.7.7** and **A.8.1** of ISO/IEC 27001:2022.

## 2. Scope

This policy applies to:

- All **[EMPRESA]** personnel who perform telework, remote work, or mobile work, whether on a regular or occasional basis.
- Contractors and third parties with access to the organization's information systems from external locations.
- All devices (corporate and personal under BYOD scheme) used to access **[EMPRESA]** resources.
- All physical locations where corporate information is processed, including offices, private residences, and public spaces.

## 3. Definitions

| Term | Definition |
|------|-----------|
| **Remote Work** | Work activity performed outside [EMPRESA] facilities on a regular or occasional basis, using information technologies. |
| **Information Asset** | Any element that contains, processes, or transmits information of value to the organization: equipment, software, documents, data. |
| **BYOD** | *Bring Your Own Device*. Use of an employee's personal devices for work purposes. |
| **VPN** | *Virtual Private Network*. An encrypted tunnel between the remote device and the corporate network. |
| **Clean Desk** | Practice of not leaving sensitive information visible or accessible in the work area when stepping away. |
| **Clean Screen** | Practice of locking the computer session when stepping away from the workstation. |
| **Full Disk Encryption** | Technology that encrypts the entire device storage, protecting data in case of loss or theft. |

## 4. Remote Work Security Requirements

### 4.1 Corporate Network Connection

- **Use of the corporate VPN is mandatory** for all access to internal systems, without exception.
- The VPN must use minimum **AES-256** encryption with **IKEv2/IPsec** or **WireGuard** protocol.
- Accessing corporate systems from public or open Wi-Fi networks without an active VPN is **prohibited**.
- Split tunneling is **disabled** by default; all traffic must transit through the VPN when accessing internal resources.
- The VPN session will automatically disconnect after **30 minutes** of inactivity.

### 4.2 Device Encryption

- Every portable device (laptop, tablet) accessing **[EMPRESA]** information must have **full disk encryption** enabled:
  - **Windows**: BitLocker with XTS-AES 256-bit algorithm. The recovery key must be stored in the corporate directory (Active Directory / Azure AD).
  - **macOS**: FileVault 2 with institutional recovery key held by the IT team.
  - **Linux**: LUKS (dm-crypt) with a minimum 20-character passphrase.
- External storage devices (USB drives, portable disks) used for corporate information must be encrypted with **BitLocker To Go** or an approved equivalent.
- **Disabling or suspending disk encryption** without written authorization from the Security Officer is **prohibited**.

### 4.3 Physical Work Environment

- Remote workers must have a workspace that provides visual and auditory privacy when handling confidential information.
- Virtual meetings with classified content must not be held in shared spaces where third parties could hear or view the screen.
- In public spaces (airports, cafés, public transport), the use of **screen privacy filters** on laptops and tablets is **mandatory**.
- Equipment must not be left unattended in vehicles, hotel common areas, or any publicly accessible location.

### 4.4 Authentication and Access Control

- All remote access requires **multi-factor authentication (MFA)**.
- Corporate credentials must not be stored in browsers on personal devices.
- **Sharing VPN credentials or MFA tokens** with third parties, including family members, is **prohibited**.

## 5. Information Asset Management

### 5.1 Inventory and Tagging

- Every IT asset assigned to personnel must be registered in the **corporate asset inventory** with the following minimum data:
  - Equipment serial number
  - Model and manufacturer
  - Assignment date
  - Name of the responsible custodian
  - Encryption status (enabled/disabled)
  - Usual location (office / home / mobile)
- Each asset must bear a visible **property tag** with the **[EMPRESA]** inventory code.
- Inventory verification will be conducted **semi-annually**.

### 5.2 Acceptable Use of Assets

- Corporate assets are for **work use only**, unless explicitly authorized otherwise.
- The following are **prohibited**:
  - Installing unauthorized or unverified software.
  - Modifying the equipment's security configuration (firewall, antivirus, encryption).
  - Connecting corporate equipment to insecure networks without VPN.
  - Lending equipment to unauthorized individuals, including family members.

### 5.3 Asset Return

- Upon termination of employment or role change, the employee must return all corporate assets within a maximum of **3 business days**.
- The IT team will perform a certified secure wipe (**NIST 800-88**) before reassigning equipment.

## 6. Clean Desk and Clean Screen Policy

### 6.1 Clean Desk

- When stepping away from the workstation (including home office), all printed documents containing information classified as **Confidential** or **Restricted** must be stored under lock and key.
- Sticky notes with passwords, PIN codes, or sensitive information visible on monitors, desks, or surrounding areas are **not permitted**.
- Printed documents no longer needed must be destroyed using a **cross-cut shredder** (minimum P-4 level per DIN 66399).
- Whiteboards and writing surfaces must be cleaned after work sessions involving sensitive information.

### 6.2 Clean Screen

- The computer session must lock automatically after **5 minutes** of inactivity.
  - **Windows**: GPO configuration with mandatory session lock.
  - **macOS**: Screen Lock configuration in Security Preferences.
  - **Linux**: Screensaver configuration with lock enabled.
- Before stepping away, the employee must manually lock the session:
  - **Windows**: `Win + L`
  - **macOS**: `Ctrl + Cmd + Q`
  - **Linux**: `Super + L` (distribution-dependent)
- **Disabling automatic screen lock** or setting timeouts greater than 5 minutes is **prohibited**.
- No windows with sensitive information should remain open at the end of the workday; all applications and sessions must be closed.

## 7. Personal Devices (BYOD)

### 7.1 Conditions for BYOD Use

Use of personal devices to access **[EMPRESA]** resources is **permitted only** if all of the following conditions are met:

1. The device has an **up-to-date operating system** (latest major version or the immediately preceding one with current security patches).
2. It has **antivirus/antimalware software** approved by IT installed.
3. **Full disk encryption** is enabled.
4. Access is made **exclusively through the corporate VPN**.
5. The employee has signed the **Personal Device Use Agreement**.
6. The IT team has verified and registered the device.

### 7.2 BYOD Restrictions

- **[EMPRESA]** is not responsible for the loss of personal data during the application of corporate security measures.
- In the event of employment termination or a security incident, **[EMPRESA]** reserves the right to request a **remote wipe** of corporate data from the device.
- **Storing information classified as Confidential** on personal devices outside of authorized corporate applications (sandboxing) is **prohibited**.

## 8. Incident Reporting

### 8.1 Lost or Stolen Devices

- Any corporate or authorized BYOD device that is **lost or stolen** must be reported to the IT team and the Security Officer within a maximum of **1 hour** from the moment the loss is detected.
- The report must include:
  - Device description (model, serial number)
  - Approximate time and place of loss
  - Whether the device had active disk encryption
  - Whether there were active sessions at the time of loss
  - Whether a police report has been filed (mandatory in case of theft)
- Reporting channels:
  - **Email**: security@[EMPRESA].com
  - **Phone**: IT direct line (available 24/7)
  - **Ticket**: incident management system

### 8.2 Loss/Theft Response

- IT will immediately execute a **remote lock** of the device.
- VPN and corporate access credentials associated with the device will be revoked.
- If the device contained unencrypted data, the **breach notification protocol** will be activated per applicable legislation.
- The incident will be documented in the information security incident register.

## 9. Responsibilities

| Role | Responsibility |
|------|---------------|
| **Executive Management** | Approve this policy and allocate resources for its implementation. |
| **Information Security Officer** | Oversee compliance, update the policy, and manage incidents. |
| **IT Manager** | Implement technical controls: VPN, encryption, inventory, remote wipe, GPOs. |
| **Human Resources** | Include this policy in onboarding and manage asset returns. |
| **All personnel** | Comply with this policy, report incidents, and protect assigned assets. |

## 10. Non-Compliance Sanctions

Non-compliance with this policy will be addressed according to severity:

| Level | Example | Consequence |
|-------|---------|------------|
| **Minor** | Failing to lock the screen when stepping away. | Verbal warning and mandatory training. |
| **Moderate** | Connecting without VPN or disabling encryption. | Written warning and temporary restriction of remote access. |
| **Serious** | Failing to report a device loss within the established timeframe. | Suspension of remote work privileges and disciplinary proceedings. |
| **Very Serious** | Sharing VPN credentials with third parties or leaking information. | Disciplinary proceedings that may result in contract termination and legal action. |

## 11. Reviews and Updates

This policy will be reviewed **annually** or upon significant changes in infrastructure, legislation, or the threat landscape.

## Revision History

| Version | Date | Change Description | Author |
|---------|------|-------------------|--------|
| 1.0 | 2026-11-11 | Initial document creation. | [SECURITY_OFFICER] |

---

> **Classification**: Internal  
> **ISO 27001:2022 Controls**: A.6.7 (Remote working), A.7.7 (Clear desk and clear screen), A.8.1 (User endpoint devices)  
> **Next review**: 2027-11-11
