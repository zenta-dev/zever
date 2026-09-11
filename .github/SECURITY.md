# Security Policy

## Reporting a Vulnerability

**Do not report security vulnerabilities through public GitHub issues.** Public issues are visible to everyone, including the people who might exploit the vulnerability.

Please report vulnerabilities privately using the ["Report a vulnerability"](https://github.com/zenta-dev/zever/security/advisories/new) page in GitHub Security Advisories instead.

### What to include in a report

To help triage the issue quickly, include:

- The affected version(s) of zever and the Go version used
- A description of the vulnerability and its impact
- A minimal reproduction, including configuration and environment details where relevant
- Steps to trigger the issue
- Where applicable, a proposed fix or mitigation

You may include a patch or proof of concept. Do not include secrets, credentials, or private keys.

### What to expect

- An initial acknowledgement within 3 business days of submission.
- A triage assessment and plan within 2 weeks.
- Coordination on a fix and a release date before public disclosure.
- Credit for responsibly reported vulnerabilities, if you want it, in the security advisory.

## Supported Versions

As a pre-1.0 project, security fixes are backported to the latest release. If you rely on a specific release, upgrade to its latest patch version.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| older   | :x:                |

## Responsible Disclosure

We ask that you:

- Give maintainers reasonable time to fix and release before disclosing publicly.
- Not exploit, or help others exploit, the vulnerability beyond what is needed to demonstrate it.

We will coordinate disclosure and credit options with you. If a security issue is disclosed without coordination, we will still fix it, but coordinated reporting is strongly preferred and appreciated.