Perform a security audit of this repository. Check for:
- Command injection and shell injection vulnerabilities
- SQL injection, XSS, CSRF risks
- Path traversal and directory traversal
- Hardcoded secrets, API keys, credentials in code
- Unsafe deserialization
- Missing input validation at system boundaries
- Authentication and authorization flaws
- Insecure cryptographic practices

For each finding, report: file, line, severity (critical/high/medium/low), description, and suggested fix.
Write findings to SECURITY-AUDIT.md at the repo root, then commit and push to a review/security-audit branch.
