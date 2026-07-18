# Security policy

ParityLab v1 is for Stripe Sandbox and test-mode data only.

## Reporting

Do not open public issues containing credentials, webhook payloads, personal data, or exploit details. Report security concerns privately to the repository owner with reproduction steps and affected versions.

## Supported guarantees

- Live-mode Stripe events and keys are rejected.
- Webhook signatures are verified from the raw request body.
- Sensitive values are redacted from logs and reports.
- Target URLs must pass SSRF validation before production use.

See `docs/THREAT_MODEL.md` for the detailed model.
