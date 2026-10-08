# Code Signing Policy for PopTrans

This document outlines the code signing policy and practices for **PopTrans** (an open-source project hosted at [https://github.com/ifocus9/PopTrans](https://github.com/ifocus9/PopTrans)).

Code signing certificates provided by **SignPath Foundation** are used exclusively to sign release binaries and installation artifacts for the PopTrans project.

---

## 1. Project Information & Purpose

- **Project Name**: PopTrans
- **Repository**: [https://github.com/ifocus9/PopTrans](https://github.com/ifocus9/PopTrans)
- **License**: MIT License ([LICENSE](LICENSE))
- **Project Maintainer**: ifocus9 ([GitHub Profile](https://github.com/ifocus9))
- **Target Platform**: Windows (x64)

The sole purpose of code signing in this project is to verify authenticity, maintain file integrity, and prevent Windows SmartScreen warnings for legitimate users downloading PopTrans releases.

---

## 2. Scope of Signed Artifacts

Only official release artifacts generated from trusted CI builds are eligible for code signing:

- Windows executables:
  - `PopTrans.exe` / `translate-ui.exe` (Wails-based GUI frontend)
  - `translate-go.exe` (Core background service, OCR, and tray controller)
  - `ai_engine.exe` (Bundled offline local inference engine)
- Compressed distribution packages and installers:
  - `PopTrans-v*.zip` / installer executables attached to official GitHub Releases.

Intermediate build outputs, development snapshots, unreviewed pull request artifacts, or arbitrary third-party binaries **will never be signed**.

---

## 3. Key Storage & Security

- **No Local Private Keys**: Private signing keys are never held, exported, or stored on local developer machines or self-hosted servers.
- **Hardware Security Module (HSM)**: Signing keys are generated, secured, and stored exclusively inside SignPath's certified Hardware Security Modules (HSM).
- **Access Delegation**: CI runners and project maintainers interact with signing keys strictly through authenticated, scoped API tokens provided by the SignPath integration.

---

## 4. Build & Signing Pipeline Integrity

All official builds and signing operations are executed through an automated, reproducible GitHub Actions CI/CD pipeline:

1. **Source Code Integrity**:
   - Signing workflows are only triggered by tagged releases (e.g., `v*.*.*`) or direct pushes to the protected primary branch (`main`) by authenticated maintainers.
   - Pull Requests (PRs), whether internal or from external forks, **do not have access** to signing credentials or signing workflows.
2. **Transparent Build Scripts**:
   - The entire build environment, dependency fetching, compilation flags, and packaging steps are defined openly in the repository workflows and build scripts (`scripts/`).
   - Builds are executed in clean, ephemeral GitHub-hosted runners.
3. **Auditability & Traceability**:
   - Every signed binary is traceable to a specific Git commit hash, release tag, and GitHub Actions workflow run.
   - The commit history, release notes, and SHA256 checksums are publicly documented in the respective GitHub Release.

---

## 5. Roles and Responsibilities

- **Maintainers**:
  - Responsible for reviewing code changes before merging into the main branch.
  - Responsible for creating official release tags.
  - Ensuring the SignPath policy configuration remains strictly aligned with the repository's security requirements.
- **Contributors**:
  - May propose code modifications via Pull Requests. Contributions undergo automated linting/testing and maintainer review before integration.

---

## 6. Vulnerability & Malware Incident Response

In the event that a signed binary is discovered to contain a critical security vulnerability, inadvertent malware, or compromised dependency:

1. **Immediate Notification**:
   - Security issues can be reported confidentially via **GitHub Security Advisories** or directly to the project maintainer via GitHub issues/email: `contact@ifocus9` (or repository issue tracker).
2. **Revocation & Containment**:
   - The maintainer will immediately notify the SignPath Foundation team to request certificate revocation if keys or build systems are suspected to be compromised.
   - The compromised release will be immediately removed or marked unsafe on GitHub Releases.
3. **Patch & Reissue**:
   - A patched version will be built, signed, and published with a detailed advisory detailing the remediation steps.

---

## 7. Compliance with SignPath Foundation Requirements

PopTrans affirms that:
- It is a genuine, active, open-source project licensed under an OSI-approved license (MIT).
- Signing certificates will never be used for commercial reselling, deceptive software, telemetry trackers, or malware distribution.
- The repository and signing policies will remain publicly accessible to all users.

---

*Last Updated: 2026-10-08*
