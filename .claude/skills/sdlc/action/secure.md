# /secure

/secure performs comprehensive security scanning and checks to identify vulnerabilities and security issues in the codebase.

**Purpose**: Security scanning and vulnerability detection

## Usage

```
/sdlc secure [check] [check...]
```

**Checks:**
- `vuln` - Vulnerability scanning in dependencies and code
- `deps` - Dependency security analysis
- `secrets` - Secret detection (API keys, tokens, passwords)
- `config` - Security configuration review
- `all` - Run all security checks (default)

**Examples:**
- `/sdlc secure` - Run all security checks
- `/sdlc secure vuln secrets` - Run only vulnerability and secret scans
- `/sdlc secure deps` - Run only dependency check

## Execution Order

Checks run in this order:

1. **secrets** - Scan for leaked credentials (critical)
2. **deps** - Check dependency vulnerabilities
3. **config** - Review security configurations
4. **vuln** - Full vulnerability scan

## Security Categories

### 1. Secrets Detection
**Purpose**: Find leaked credentials and sensitive data

**Scans for:**
- API keys (AWS, Google, GitHub, etc.)
- Database connection strings
- JWT tokens and session keys
- Passwords in code
- Private keys and certificates
- OAuth tokens
- API secrets

**Tools**: git-secrets, gitleaks, truffleHog

**Output example**:
```
━━━ Secrets Detection ━━━
✓ No secrets detected in recent commits

Scanned:
- 45 files modified in last 7 days
- 156 commits in current branch
- 3 binary files excluded

Excluded patterns:
- node_modules/, .git/, dist/, build/
```

**Failure example**:
```
━━━ Secrets Detection ━━━
✗ POTENTIAL SECRETS FOUND

Critical:
  ✗ AWS Access Key ID
    Location: src/config/aws.ts:23
    Match: AKIAIOSFODNN7EXAMPLE
    Action: Rotate immediately, remove from code

  ✗ Database URL
    Location: .env.example:15
    Match: mongodb+srv://user:pass@cluster.mongodb.net
    Action: Use environment variables, do not commit

Warnings:
  ⚠ GitHub Token (possible false positive)
    Location: tests/fixtures/mock-data.json:156
    Match: ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    Action: Verify if this is a test token
```

### 2. Dependency Security
**Purpose**: Check for vulnerabilities in dependencies

**Checks:**
- Known vulnerabilities (CVEs)
- Outdated packages with security fixes
- Malicious packages
- License compliance
- Transitive dependencies

**Tools**: npm audit, yarn audit, Snyk, Dependabot

**Output example**:
```
━━━ Dependency Security ━━━
✓ No vulnerabilities found

Dependencies checked:
- 152 production dependencies
- 34 development dependencies
- 1,245 transitive dependencies

Last audit: 2024-03-08
```

**Failure example**:
```
━━━ Dependency Security ━━━
✗ 5 vulnerabilities found

Critical (1):
  ✗ lodash < 4.17.21
    Vulnerability: Prototype Pollution
    Severity: CRITICAL
    Package: lodash
    Fix: Upgrade to 4.17.21 or later
    Command: npm install lodash@^4.17.21

High (2):
  ✗ axios < 0.21.1
    Vulnerability: Server-Side Request Forgery
    Severity: HIGH
    Fix: Upgrade to 0.21.2 or later

  ✗ minimist < 0.2.1
    Vulnerability: Prototype Pollution
    Severity: HIGH
    Fix: Upgrade to 0.2.1 or later

Moderate (2):
  ⚠ follow-redirects < 1.14.7
  ⚠ node-forge < 1.3.0

Recommended action:
npm audit fix
```

### 3. Configuration Review
**Purpose**: Review security configurations

**Checks:**
- CORS configuration
- Security headers
- Authentication setup
- Environment variable handling
- File permissions
- Database security settings

**Output example**:
```
━━━ Configuration Review ━━━
✓ Security configurations reviewed

Passed checks:
✓ CORS properly restricted to origins
✓ Security headers configured (HSTS, CSP, X-Frame-Options)
✓ Environment variables used for secrets
✓ Database connection uses SSL
✓ File permissions are restrictive (600/644)
✓ Rate limiting configured on API endpoints

Warnings:
⚠ Consider adding Content-Security-Policy-Report-Only
⚠ Session timeout could be shorter (currently 7 days)
```

**Failure example**:
```
━━━ Configuration Review ━━━
✗ Security issues found in configuration

Critical:
  ✗ CORS allows all origins
    Location: src/server.ts:45
    Current: origin: "*"
    Fix: Restrict to specific origins

  ✗ Missing security headers
    Missing: HSTS, X-Content-Type-Options, X-Frame-Options
    Fix: Add helmet middleware

High:
  ✗ Hardcoded credentials in config
    Location: src/config/database.ts:12
    Issue: Database URL in source code
    Fix: Use environment variables

  ✗ Debug mode enabled in production
    Location: .env:8
    Current: DEBUG=true
    Fix: Set DEBUG=false in production
```

### 4. Vulnerability Scan
**Purpose**: Comprehensive code vulnerability scanning

**Checks:**
- SQL injection
- XSS (Cross-site scripting)
- CSRF (Cross-site request forgery)
- Command injection
- Path traversal
- Insecure deserialization
- Authentication bypasses
- Authorization issues

**Tools**: Semgrep, SonarQube, CodeQL

**Output example**:
```
━━━ Vulnerability Scan ━━━
✓ No critical vulnerabilities found

Scanned:
- 245 source files
- 12,450 lines of code
- 156 functions

Results:
- Critical: 0
- High: 0
- Medium: 2
- Low: 5

Medium findings:
  ⚠ Unvalidated user input in redirect
    Location: src/controllers/auth.ts:89
    Issue: Redirect URL not validated against whitelist
    Fix: Validate redirect URLs against allowed list

  ⚠ Missing rate limiting on login
    Location: src/routes/auth.ts:15
    Issue: No rate limiting on authentication endpoint
    Fix: Add rate limiting middleware

Low findings:
  ⚠ Non-constant time comparison
  ⚠ Debug logging in production code
  ⚠ Deprecated API usage
  ⚠ Missing input length validation
  ⚠ Generic error messages
```

## Full Security Report Example

```
Running security scan...

━━━ Secrets Detection ━━━
✗ POTENTIAL SECRETS FOUND

Critical:
  ✗ Database URL in .env.example
    Location: .env.example:15
    Action: Remove, use placeholder only

Warnings:
  ⚠ Possible GitHub token in test file
    Location: tests/auth.test.ts:23
    Action: Verify this is a mock token

━━━ Dependency Security ━━━
✗ 3 vulnerabilities found

High:
  ✗ lodash < 4.17.21
    Fix: npm install lodash@^4.17.21

  ✗ minimist < 0.2.1
    Fix: npm update minimist

Moderate:
  ⚠ axios < 0.21.1
    Fix: npm install axios@^0.21.2

━━━ Configuration Review ━━━
⚠ 3 warnings found

Warnings:
  ⚠ CORS allows all origins in development
  ⚠ Session timeout is 7 days (recommend shorter)
  ⚠ Missing Content-Security-Policy header

━━━ Vulnerability Scan ━━━
✓ No critical vulnerabilities

Medium findings:
  ⚠ Unvalidated redirect in auth controller
  ⚠ Missing rate limiting on login endpoint

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
SECURITY SCAN FAILED ✗
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Summary:
- Secrets: 1 critical, 1 warning
- Dependencies: 2 high, 1 moderate
- Configuration: 3 warnings
- Vulnerabilities: 0 critical, 2 medium

━━━ Action Items ━━━
Priority - CRITICAL:
1. Remove database URL from .env.example
2. Rotate leaked credentials if any

Priority - HIGH:
3. Update lodash to 4.17.21
4. Update minimist to 0.2.1
5. Restrict CORS configuration

Priority - MEDIUM:
6. Add rate limiting to login endpoint
7. Validate redirect URLs against whitelist
8. Add Content-Security-Policy header
```

## Success Report Example

```
Running security scan...

━━━ Secrets Detection ━━━
✓ No secrets detected
  Scanned 45 files, 156 commits

━━━ Dependency Security ━━━
✓ No vulnerabilities found
  Checked 152 production dependencies

━━━ Configuration Review ━━━
✓ All security configurations valid
  CORS, headers, environment variables: OK

━━━ Vulnerability Scan ━━━
✓ No critical or high vulnerabilities
  2 low-severity findings (info only)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
ALL SECURITY CHECKS PASSED ✓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Summary:
- Secrets: 0 issues
- Dependencies: 0 vulnerabilities
- Configuration: 0 issues
- Vulnerabilities: 0 critical/high

Duration: 45.2s
Recommendations:
- Schedule regular security scans (weekly)
- Set up automated dependency updates
- Implement security monitoring in production
```

## Security Output

**Always save security reports** to `.sdlc/docs/category-feature-date.secure.md` where:
- `category` - Module/category
- `feature` - Feature or component name
- `date` - Date in YYYYMMDD format
- `secure` - Document type for security reports

## Best Practices

### Prevention
- **Never commit secrets**: Use environment variables
- **Scan regularly**: Run security checks before every release
- **Update dependencies**: Keep packages up to date
- **Use security tools**: Linting with security rules
- **Review code**: Security-focused code reviews

### Tools Setup
```bash
# npm audit (built-in)
npm audit

# Snyk (requires account)
npm install -g snyk
snyk auth
snyk test

# git-secrets
git secrets --install
git secrets --register-aws

# gitleaks
gitleaks detect --source .
```

### CI/CD Integration
Add security checks to your CI pipeline:
```yaml
# Example GitHub Actions
- name: Security Scan
  run: |
    npm audit
    npm run lint:security
    # Add your security scanning tools here
```

### Handling Findings
- **Critical**: Block deployment, fix immediately
- **High**: Fix before next release
- **Medium**: Schedule fix, assess risk
- **Low**: Track and fix when possible

## Common Security Issues

### 1. Hardcoded Secrets
**Problem**: Credentials in source code
**Fix**: Use environment variables
```bash
# Bad
const apiKey = "sk_live_1234567890"

# Good
const apiKey = process.env.STRIPE_API_KEY
```

### 2. SQL Injection
**Problem**: Unsanitized input in queries
**Fix**: Use parameterized queries
```javascript
// Bad
db.query(`SELECT * FROM users WHERE id = ${userId}`)

// Good
db.query("SELECT * FROM users WHERE id = ?", [userId])
```

### 3. XSS
**Problem**: Unescaped user input in output
**Fix**: Sanitize and escape output
```javascript
// Bad
div.innerHTML = userInput

// Good
div.textContent = userInput
// or use DOMPurify for HTML
```

### 4. Insecure Dependencies
**Problem**: Outdated packages with vulnerabilities
**Fix**: Regular updates and auditing
```bash
npm audit fix
npm update
```

## Completion Conditions

- [ ] All critical vulnerabilities resolved
- [ ] All high vulnerabilities resolved or documented
- [ ] No secrets detected in code
- [ ] Dependencies up to date or vulnerabilities acknowledged
- [ ] Security configurations reviewed
- [ ] Security report saved to `.sdlc/.sdlc/docs/secure/`
- [ ] Action items documented for any remaining issues

## State Integration

- **Updates**: `sdlc.phase` = `secure`
- **Creates**: Security report in `.sdlc/docs/category-feature-date.secure.md`
- **Requires**: `validate` phase completed (implementation validated)
- **Next**: Proceed to deployment or back to coding for fixes

## Related Skills

- `/sdlc verify` - Prerequisite: implementation must be verified first
- `/sdlc coding` - Return here if security fixes are needed
- `/sdlc test` - May need to re-test after security fixes

## Security Checklist

### Code Security
- [ ] No hardcoded credentials
- [ ] Input validation on all user inputs
- [ ] Output encoding to prevent XSS
- [ ] Parameterized queries to prevent SQL injection
- [ ] Proper error handling (don't leak sensitive info)

### Authentication & Authorization
- [ ] Passwords hashed (bcrypt, argon2)
- [ ] JWT tokens properly validated
- [ ] Session management secure
- [ ] Rate limiting on auth endpoints
- [ ] Proper access controls

### API Security
- [ ] HTTPS only in production
- [ ] CORS properly configured
- [ ] Security headers set
- [ ] API rate limiting
- [ ] Request validation

### Data Security
- [ ] Sensitive data encrypted at rest
- [ ] Sensitive data encrypted in transit
- [ ] Proper access controls
- [ ] Audit logging for sensitive operations
- [ ] Data retention policies

### Infrastructure Security
- [ ] Environment variables for secrets
- [ ] No secrets in code or config files
- [ ] Dependencies regularly updated
- [ ] Security scanning in CI/CD
- [ ] Incident response plan
