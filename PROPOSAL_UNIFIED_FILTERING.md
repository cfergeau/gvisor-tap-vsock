# Proposal: Unified Network Filtering for gvisor-tap-vsock

**Status:** Draft
**Related Issues:** #630
**Related PRs:** #609, #631
**Author:** Proposal based on analysis of existing PRs

## Executive Summary

This proposal unifies the approaches from PR #609 (security-hardened static allowlisting) and PR #631 (interactive approval mode) into a single, cohesive network filtering system. The unified design supports both discovery/development workflows and production security enforcement through a common configuration model and enforcement engine.

## Goals

1. **Unified Configuration Model:** Single YAML schema that supports static rules, interactive mode, and hybrid scenarios
2. **Layered Security:** Combine PR #609's hardened SNI validation with flexible policy enforcement
3. **Progressive Workflow:** Enable discovery → policy refinement → production deployment in one system
4. **API Consistency:** Common HTTP API for both static and interactive modes
5. **Backward Compatibility:** Zero impact on existing gvproxy deployments

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    gvproxy Configuration                         │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ Network Policy (networkPolicy.yaml)                       │  │
│  │  • mode: static | interactive | hybrid                    │  │
│  │  • DNS rules (allow/deny patterns + wildcard support)     │  │
│  │  • Network rules (CIDR/port/protocol ACLs)                │  │
│  │  • Interactive settings (timeout, auto-learn, etc.)       │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Policy Enforcement Engine                      │
│                                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ DNS Filter   │  │ SNI Parser   │  │ Network ACL Engine   │  │
│  │ (from #631)  │  │ (from #609)  │  │ (from #631)          │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
│                              │                                   │
│                              ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │          Unified Decision Engine                         │  │
│  │  • Evaluate DNS queries against allow/deny patterns      │  │
│  │  • Extract SNI from TLS ClientHello                      │  │
│  │  • Cross-validate SNI → DNS → destination IP (from #609) │  │
│  │  • Apply ACL rules (CIDR, port, protocol)                │  │
│  │  • Interactive approval for pending requests             │  │
│  │  • Auto-learn approved destinations                      │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     HTTP API + Events                            │
│  • /policy/dns/allow, /policy/dns/deny (static rules)           │
│  • /policy/network/allow, /policy/network/deny (static rules)   │
│  • /policy/pending (list pending requests)                      │
│  • /policy/approve, /policy/deny (interactive decisions)        │
│  • /policy/events (SSE stream for real-time notifications)      │
│  • /policy/export (generate YAML from learned rules)            │
└─────────────────────────────────────────────────────────────────┘
```

## Configuration Model

### Unified YAML Schema

```yaml
# Network policy configuration for gvisor-tap-vsock
version: 1

# Operating mode: static, interactive, or hybrid
mode: hybrid

# DNS-level filtering
dns:
  # Default action when no rule matches
  defaultAction: deny  # allow | deny | ask

  # Static allow patterns (regex)
  allow:
    - "^.*\\.github\\.com$"
    - "^.*\\.googleapis\\.com$"
    - "^registry\\.fedoraproject\\.org$"

  # Static deny patterns (regex)
  deny:
    - "^.*\\.malware\\.example$"

  # Wildcard support (converted to regex internally)
  allowWildcard:
    - "*.docker.io"
    - "*.cloudflare.com"

# Network-level filtering (IP/port/protocol)
network:
  # Default action when no rule matches
  defaultAction: deny  # allow | deny | ask

  # ACL rules (first-match-wins)
  rules:
    - action: allow
      protocol: tcp
      cidr: "0.0.0.0/0"
      ports: [80, 443]
      description: "Allow HTTPS/HTTP to anywhere"

    - action: deny
      protocol: tcp
      cidr: "10.0.0.0/8"
      ports: "*"
      description: "Block RFC1918 private networks"

    - action: allow
      protocol: udp
      cidr: "8.8.8.8/32"
      ports: [53]
      description: "Allow Google DNS"

# TLS/SNI security (from PR #609)
tls:
  # Enable SNI extraction and validation
  enabled: true

  # Reject connections with IP literals in SNI (CVE-2025-25255)
  rejectIPLiterals: true

  # Perform DNS cross-validation (resolve SNI, compare with dest IP)
  dnsValidation: true

  # Log ALPN protocols for tunneling detection
  logALPN: true

  # Handle ECH/GREASE per RFC 9849 (allow outer SNI)
  allowECH: true

# Interactive mode settings
interactive:
  # Enable interactive approval
  enabled: true

  # Timeout for pending requests (seconds)
  timeout: 30

  # Auto-learn: add approved destinations to runtime allowlist
  autoLearn: true

  # Persist learned rules to file on shutdown
  persistLearned: true
  persistPath: "/var/lib/gvisor-tap-vsock/learned-policy.yaml"

  # SSE event stream configuration
  events:
    enabled: true
    bufferSize: 100

# Gateway exemptions (always allowed, regardless of rules)
exemptions:
  # Gateway IP is always reachable (DNS, internal services)
  allowGateway: true

  # DHCP is always allowed
  allowDHCP: true
```

### CLI Flags (Shorthand Modes)

```bash
# Static mode (production)
gvproxy --network-policy /path/to/policy.yaml

# Interactive mode (discovery)
gvproxy --secure-mode --services unix:///tmp/gvproxy.sock

# Hybrid mode (interactive with base rules)
gvproxy --network-policy /path/to/policy.yaml --interactive

# Quick deny-all with exceptions
gvproxy --deny-all --allow-domains "*.github.com,*.docker.io"
```

## Unified HTTP API

### Policy Management (Static Rules)

```
POST /policy/dns/allow
  Body: {"pattern": "^.*\\.example\\.com$"}
  Response: {"status": "ok", "id": "dns-allow-1"}

POST /policy/dns/deny
  Body: {"pattern": "^.*\\.malware\\.example$"}
  Response: {"status": "ok", "id": "dns-deny-1"}

DELETE /policy/dns/allow/{id}
  Response: {"status": "ok"}

GET /policy/dns/rules
  Response: {
    "allow": [{"id": "dns-allow-1", "pattern": "^.*\\.example\\.com$"}],
    "deny": [{"id": "dns-deny-1", "pattern": "^.*\\.malware\\.example$"}]
  }

POST /policy/network/allow
  Body: {"protocol": "tcp", "cidr": "0.0.0.0/0", "ports": [443]}
  Response: {"status": "ok", "id": "net-allow-1"}

POST /policy/network/deny
  Body: {"protocol": "tcp", "cidr": "10.0.0.0/8"}
  Response: {"status": "ok", "id": "net-deny-1"}
```

### Interactive Approval

```
GET /policy/pending
  Response: {
    "dns": [
      {
        "id": "pending-dns-1",
        "domain": "api.stripe.com",
        "timestamp": "2026-03-26T10:30:00Z",
        "expiresAt": "2026-03-26T10:30:30Z"
      }
    ],
    "network": [
      {
        "id": "pending-net-1",
        "protocol": "tcp",
        "ip": "142.250.80.46",
        "port": 443,
        "sni": "www.google.com",
        "alpn": ["h2", "http/1.1"],
        "timestamp": "2026-03-26T10:30:05Z",
        "expiresAt": "2026-03-26T10:30:35Z"
      }
    ]
  }

POST /policy/approve
  Body: {
    "id": "pending-net-1",
    "remember": true,  // Add to runtime allowlist
    "persist": false   // Don't write to config file
  }
  Response: {"status": "approved", "autoLearned": true}

POST /policy/deny
  Body: {
    "id": "pending-dns-1",
    "remember": true,  // Add to runtime denylist
    "persist": false
  }
  Response: {"status": "denied"}
```

### Events (SSE Stream)

```
GET /policy/events
  Content-Type: text/event-stream

Events:
  event: dns.pending
  data: {"id": "pending-dns-1", "domain": "api.stripe.com", ...}

  event: dns.approved
  data: {"id": "pending-dns-1", "domain": "api.stripe.com", "autoLearned": true}

  event: dns.denied
  data: {"id": "pending-dns-1", "domain": "api.stripe.com", "reason": "timeout"}

  event: network.pending
  data: {"id": "pending-net-1", "protocol": "tcp", "ip": "...", ...}

  event: network.approved
  data: {"id": "pending-net-1", "autoLearned": true, "learnedRule": {...}}

  event: network.denied
  data: {"id": "pending-net-1", "reason": "user_denied"}

  event: security.sni_mismatch
  data: {"sni": "allowed.com", "ip": "1.2.3.4", "resolved": ["5.6.7.8"]}

  event: security.ip_literal_rejected
  data: {"sni": "192.0.2.1", "reason": "CVE-2025-25255"}
```

### Policy Export (Generate Config from Learned Rules)

```
GET /policy/export?format=yaml
  Response: (YAML policy file with all learned rules)

GET /policy/export?format=json
  Response: (JSON policy with all learned rules)

GET /policy/stats
  Response: {
    "dnsQueries": {"allowed": 1234, "denied": 56, "pending": 2},
    "tcpConnections": {"allowed": 890, "denied": 12, "pending": 1},
    "udpConnections": {"allowed": 456, "denied": 3, "pending": 0},
    "learnedRules": {"dns": 23, "network": 15},
    "sniValidations": {"passed": 800, "failed": 5}
  }
```

## Decision Engine Logic

### DNS Query Flow

```
1. DNS query for "api.stripe.com"
2. Check local zones (gateway, DHCP) → if match, allow (bypass policy)
3. Normalize domain to lowercase: "api.stripe.com"
4. Check static deny patterns → if match, return NXDOMAIN
5. Check static allow patterns → if match, forward to upstream
6. Check runtime learned allowlist → if match, forward
7. If mode == interactive && defaultAction == ask:
   - Hold query pending (up to timeout)
   - Send event: dns.pending
   - Wait for approval or timeout
   - If approved && autoLearn: add to runtime allowlist
   - If denied: return NXDOMAIN
8. Else: apply defaultAction (allow → forward, deny → NXDOMAIN)
```

### TCP Connection Flow (Port 443 - TLS)

```
1. TCP connection to 142.250.80.46:443
2. Check if destination is gateway → if yes, allow
3. Peek TLS ClientHello (first 5-16KB)
4. Extract SNI: "www.google.com"
5. Security validations (from PR #609):
   - Reject if SNI contains IP literal (net.ParseIP fails)
   - Normalize SNI to lowercase: "www.google.com"
   - Detect ECH/ESNI extensions → if allowECH=false, reject
   - Extract ALPN protocols → log for observability
6. If dnsValidation enabled:
   - Resolve "www.google.com" → [142.250.80.46, 142.250.80.78, ...]
   - Verify destination IP in resolved set → if not, reject (spoofing)
7. Check static DNS allow patterns for "www.google.com"
   - If denied: reject connection
   - If allowed: proceed to network ACL check
8. Check network ACL rules (first-match-wins):
   - Rule: allow tcp 0.0.0.0/0:443 → match, allow
9. If no rule matched && mode == interactive && defaultAction == ask:
   - Hold connection pending
   - Send event: network.pending (include SNI, ALPN, resolved IPs)
   - Wait for approval
   - If approved && autoLearn: add rule to runtime allowlist
10. Else: apply defaultAction
```

### TCP Connection Flow (Non-443)

```
1. TCP connection to 1.2.3.4:8080
2. No SNI available (not TLS port 443)
3. Check network ACL rules only
4. If no match && interactive && defaultAction == ask:
   - Hold pending (but note: no SNI context available)
   - User approves based on IP:port only
5. Apply decision
```

### UDP Connection Flow

```
1. UDP packet to 8.8.8.8:53
2. Check if destination is gateway → DNS to gateway always allowed
3. Check network ACL rules
4. If no match && interactive: hold pending
5. Apply decision

Note: Most UDP is DNS to gateway, so will bypass ACL via exemption.
```

## Security Hardening (From PR #609)

### SNI Parser Implementation

Reuse PR #609's `pkg/services/forwarder/sni.go` with all security features:

- **TLS record reassembly:** Handle fragmented ClientHello across multiple TLS records
- **Bounds checking:** Prevent buffer overruns in malformed handshakes
- **IP literal rejection:** `net.ParseIP()` check per RFC 6066 §3
- **Case normalization:** `strings.ToLower()` before any regex matching
- **ECH/ESNI handling:** Configurable via `tls.allowECH` flag
- **ALPN extraction:** Log protocols for tunneling detection

### DNS Cross-Validation

From PR #609's `handleTLSWithAllowlist`:

```go
// After SNI extraction and allowlist check
if cfg.TLS.DNSValidation {
    resolved, err := net.LookupIP(sni)
    if err != nil {
        log.Warnf("DNS lookup failed for SNI %q: %v (fail-closed)", sni, err)
        return deny
    }

    // Verify destination IP is in resolved set
    if !containsIP(resolved, destinationIP) {
        log.Warnf("SNI spoofing detected: SNI=%q resolved=%v dest=%v",
            sni, resolved, destinationIP)
        publishEvent("security.sni_mismatch", ...)
        return deny
    }
}
```

### Test Coverage

- Inherit 2000+ test cases from PR #609 for SNI parser
- Add interactive approval test cases from PR #631
- Add integration tests for hybrid mode workflows

## User Workflows

### Workflow 1: Discovery Mode (Development)

```bash
# Step 1: Start in secure mode (deny-all + interactive)
gvproxy --secure-mode --services unix:///tmp/gvproxy.sock

# Step 2: Run your application in the VM
# (All connections are held pending)

# Step 3: Approve requests via CLI or UI
curl --unix-socket /tmp/gvproxy.sock http:/unix/policy/pending
curl --unix-socket /tmp/gvproxy.sock -X POST http:/unix/policy/approve \
  -d '{"id": "pending-net-1", "remember": true}'

# Step 4: Export learned policy
curl --unix-socket /tmp/gvproxy.sock http:/unix/policy/export > learned-policy.yaml

# Step 5: Review and refine the policy
vi learned-policy.yaml
```

### Workflow 2: Static Policy (Production)

```bash
# Step 1: Create policy from discovery phase
cat > policy.yaml <<EOF
mode: static
dns:
  defaultAction: deny
  allow:
    - "^.*\\.github\\.com$"
    - "^.*\\.docker\\.io$"
network:
  defaultAction: deny
  rules:
    - action: allow
      protocol: tcp
      cidr: "0.0.0.0/0"
      ports: [80, 443]
tls:
  enabled: true
  dnsValidation: true
  rejectIPLiterals: true
EOF

# Step 2: Deploy with static policy
gvproxy --network-policy policy.yaml --services unix:///tmp/gvproxy.sock

# No interactive approval needed - all decisions are static
```

### Workflow 3: Hybrid Mode (Production + Exceptions)

```bash
# Base policy is static, but allow interactive approval for unknowns
gvproxy --network-policy policy.yaml --interactive

# Most traffic follows static rules
# Unknown destinations are held for approval (e.g., emergency access)
# Approved destinations can be persisted to config
```

## Implementation Plan

### Phase 1: Core Unification (Weeks 1-2)

- [ ] Define unified `pkg/types/NetworkPolicy` struct
- [ ] Implement unified configuration parser (YAML + CLI flags)
- [ ] Create `pkg/services/policy/` package with decision engine
- [ ] Integrate SNI parser from PR #609 into policy engine
- [ ] Implement DNS cross-validation logic
- [ ] Basic HTTP API endpoints (`/policy/dns/allow`, `/policy/network/allow`)

### Phase 2: Interactive Features (Weeks 3-4)

- [ ] Implement pending request queue with timeout
- [ ] Add approval/denial logic with auto-learn
- [ ] Implement SSE event streaming (`/policy/events`)
- [ ] Build HTTP API for interactive operations
- [ ] Add policy export endpoint
- [ ] Runtime allowlist/denylist management

### Phase 3: Integration & Testing (Week 5)

- [ ] Integrate policy engine into TCP forwarder
- [ ] Integrate policy engine into UDP forwarder
- [ ] Integrate policy engine into DNS server
- [ ] Port test suite from PR #609 (SNI parser, security validations)
- [ ] Port test suite from PR #631 (ACL engine, interactive mode)
- [ ] Add integration tests for hybrid workflows

### Phase 4: UI & Documentation (Week 6)

- [ ] Optional: Adapt Svelte UI from PR #631 (separate repo or subdirectory)
- [ ] CLI tool for policy management (`gvproxy-policy-cli`)
- [ ] Documentation: configuration reference, security guide, workflow examples
- [ ] Migration guide for users of PR #609 or PR #631

## Backward Compatibility

- **No flags by default:** Existing gvproxy deployments are unaffected
- **Opt-in only:** Filtering is enabled only with `--network-policy` or `--secure-mode`
- **Configuration versioning:** `version: 1` in YAML for future schema evolution
- **API stability:** `/policy/*` namespace isolates new endpoints

## Migration from Existing PRs

### From PR #609 (Static Allowlist)

```yaml
# PR #609 config (via code)
outboundAllow:
  - "^.*\\.example\\.com$"

# Unified config
mode: static
dns:
  defaultAction: deny
  allow:
    - "^.*\\.example\\.com$"
tls:
  enabled: true
  dnsValidation: true
  rejectIPLiterals: true
```

### From PR #631 (Interactive Mode)

```bash
# PR #631 command
gvproxy --secure-mode --services unix:///tmp/sock

# Unified command (identical)
gvproxy --secure-mode --services unix:///tmp/sock

# Or with explicit config
mode: interactive
dns:
  defaultAction: ask
network:
  defaultAction: ask
interactive:
  enabled: true
  timeout: 30
  autoLearn: true
```

## Open Questions

1. **UI Repository:** Should the Svelte approval UI live in this repo, or be a separate project?
   - **Recommendation:** Separate repo (`gvisor-tap-vsock-ui`) to avoid frontend build deps in core

2. **Persistence Format:** Should learned rules be stored as YAML, SQLite, or both?
   - **Recommendation:** YAML for human readability, with optional SQLite for high-volume scenarios

3. **Policy Merging:** If `--network-policy` and `--interactive` are both specified, how do conflicting rules interact?
   - **Recommendation:** Static rules take precedence; interactive mode only applies when no static rule matches

4. **Authentication:** Should the HTTP API require authentication?
   - **Recommendation:** Phase 2 feature - add optional token-based auth for `/policy/*` endpoints

5. **Performance:** What's the overhead of SNI parsing + DNS validation on every HTTPS connection?
   - **Recommendation:** Add benchmarks, implement SNI cache (hash of first 16KB → parsed result)

## Success Metrics

- **Functionality:** All test cases from PR #609 and PR #631 pass
- **Usability:** Discovery workflow (secure-mode → export → deploy) takes < 10 minutes
- **Security:** No bypasses for hardened SNI validation (existing CVE test cases)
- **Performance:** < 5ms p99 latency overhead for SNI parsing + validation
- **Adoption:** Clear migration path for both PR authors and users

## References

- Issue #630: https://github.com/containers/gvisor-tap-vsock/issues/630
- PR #609: https://github.com/containers/gvisor-tap-vsock/pull/609
- PR #631: https://github.com/containers/gvisor-tap-vsock/pull/631
- RFC 6066 (TLS Extensions): https://www.rfc-editor.org/rfc/rfc6066
- RFC 9849 (ECH): https://www.rfc-editor.org/rfc/rfc9849
- CVE-2025-25255: IP literal bypass in SNI filtering
