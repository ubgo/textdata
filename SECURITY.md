# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, report them privately:

- Use **[GitHub Security Advisories](https://github.com/ubgo/textdata/security/advisories/new)** (preferred), or
- Email **khanakia@gmail.com** with the details.

Please include:

- A description of the vulnerability and its impact.
- The module and version affected (for example `github.com/ubgo/textdata/plugins/file v0.1.4`).
- Steps to reproduce, ideally the smallest input file or program that shows it.

We will acknowledge your report within a few days and keep you updated on the fix. We ask that you give us a reasonable window to release a fix before any public disclosure, and we're happy to credit you in the advisory.

Areas where a report is especially welcome: a value plugin reading outside its confinement (`plugins/file`, `plugins/extends`), a secret value appearing in an error message or log, a `$(command)` running without a command runner installed, or a save that leaves a partly written file.

## Supported versions

Security fixes go into the latest release of the affected module. The pre-release tags `v0.1.1`–`v0.1.3` are retracted; upgrade to `v0.1.4` or newer.

| Version | Supported |
|---|---|
| latest `v0.1.x` of each module | ✅ |
| retracted `v0.1.1`–`v0.1.3` | ❌ |
