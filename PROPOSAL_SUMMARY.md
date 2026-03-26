# Unified Network Filtering - Quick Reference

## The Big Idea

Merge PR #609's security hardening with PR #631's interactive approval into a single system that supports the full security lifecycle: **discover → refine → enforce**.

## Three Operating Modes

```
┌──────────────────────────────────────────────────────────────┐
│ MODE: STATIC (Production Security)                           │
│ • Pre-defined allow/deny rules in YAML                       │
│ • SNI validation + DNS cross-checking (from PR #609)         │
│ • No user interaction needed                                 │
│ • Use case: CI/CD, production deployments                    │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ MODE: INTERACTIVE (Discovery & Development)                  │
│ • Deny-all by default, ask for each new destination          │
│ • Real-time approval via HTTP API or web UI (from PR #631)   │
│ • Auto-learn approved destinations to runtime allowlist      │
│ • Export learned rules to YAML policy file                   │
│ • Use case: Development, debugging, policy creation          │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ MODE: HYBRID (Production + Escape Hatch)                     │
│ • Base rules from static policy                              │
│ • Interactive approval for unknown destinations              │
│ • Approved exceptions can be persisted to config             │
│ • Use case: Prod with emergency override capability          │
└──────────────────────────────────────────────────────────────┘
```

## Key Innovations

### 1. Unified Configuration Schema

Single YAML file that works for all three modes:

```yaml
mode: hybrid  # static | interactive | hybrid

dns:
  defaultAction: deny  # allow | deny | ask
  allow: ["^.*\\.github\\.com$"]
  allowWildcard: ["*.docker.io"]

network:
  defaultAction: deny
  rules:
    - action: allow
      protocol: tcp
      cidr: "0.0.0.0/0"
      ports: [80, 443]

tls:
  enabled: true
  dnsValidation: true      # PR #609's SNI cross-check
  rejectIPLiterals: true   # CVE-2025-25255 mitigation

interactive:
  enabled: true
  timeout: 30
  autoLearn: true          # PR #631's learning capability
  persistLearned: true
```

### 2. Layered Security Decision Engine

Every TLS connection goes through:

```
1. Extract SNI from ClientHello (PR #609's hardened parser)
   ├─ Reject IP literals (CVE-2025-25255)
   ├─ Normalize to lowercase (case-bypass mitigation)
   └─ Handle ECH/GREASE per RFC 9849

2. DNS Cross-Validation (PR #609's anti-spoofing)
   ├─ Resolve SNI → [list of IPs]
   ├─ Verify destination IP is in resolved set
   └─ Block if mismatch (SNI spoofing detected)

3. Apply Policy Rules
   ├─ Check static DNS allow/deny patterns
   ├─ Check static network ACL rules (CIDR/port/protocol)
   └─ Check runtime learned allowlist

4. Interactive Fallback (PR #631's approval flow)
   ├─ If no rule matched && defaultAction == "ask"
   ├─ Hold connection pending (configurable timeout)
   ├─ Send SSE event: network.pending
   ├─ Wait for user approval via HTTP API
   └─ If approved && autoLearn: add to runtime allowlist

5. Apply Final Decision
   ├─ Allow → proxy connection
   └─ Deny → reject connection
```

### 3. Complete HTTP API

```
Policy Management:
  POST /policy/dns/allow        - Add static DNS allow rule
  POST /policy/network/allow    - Add static network ACL rule
  GET  /policy/dns/rules        - List all DNS rules
  GET  /policy/network/rules    - List all network rules

Interactive Approval:
  GET  /policy/pending          - List pending requests
  POST /policy/approve          - Approve pending request
  POST /policy/deny             - Deny pending request

Real-time Events:
  GET  /policy/events           - SSE stream (dns.pending, network.approved, etc.)

Policy Export:
  GET  /policy/export           - Generate YAML from learned rules
  GET  /policy/stats            - Statistics (allowed/denied/pending counts)
```

### 4. Discovery-to-Production Workflow

```bash
# PHASE 1: Discovery (Interactive Mode)
$ gvproxy --secure-mode --services unix:///tmp/gvproxy.sock
# Start web UI or use curl to approve/deny connections
# Auto-learn captures all approved destinations

# PHASE 2: Export Learned Policy
$ curl --unix-socket /tmp/gvproxy.sock http:/unix/policy/export > policy.yaml

# PHASE 3: Review and Refine
$ vi policy.yaml
# Edit patterns, tighten rules, add comments

# PHASE 4: Deploy to Production (Static Mode)
$ gvproxy --network-policy policy.yaml
# No interactive approval needed - all enforced via static rules
# SNI validation and DNS cross-checking prevent bypasses
```

## Comparison Matrix

| Feature | PR #609 Only | PR #631 Only | Unified Proposal |
|---------|--------------|--------------|------------------|
| **Static regex allowlist** | ✅ | ❌ | ✅ |
| **SNI extraction** | ✅ | ❌ | ✅ |
| **DNS cross-validation** | ✅ | ❌ | ✅ |
| **IP literal rejection** | ✅ | ❌ | ✅ |
| **Interactive approval** | ❌ | ✅ | ✅ |
| **Real-time SSE events** | ❌ | ✅ | ✅ |
| **Auto-learn approved rules** | ❌ | ✅ | ✅ |
| **CIDR/port ACL rules** | ❌ | ✅ | ✅ |
| **Wildcard domain support** | ❌ | ✅ | ✅ |
| **Policy export to YAML** | ❌ | ✅ | ✅ |
| **Hybrid mode** | ❌ | ❌ | ✅ |
| **CVE-2025-25255 mitigation** | ✅ | ❌ | ✅ |
| **SNI spoofing detection** | ✅ | ❌ | ✅ |
| **Case-bypass prevention** | ✅ | ❌ | ✅ |
| **ECH/GREASE handling** | ✅ | ❌ | ✅ |
| **ALPN logging** | ✅ | ❌ | ✅ |

## Example Configurations

### Development Environment

```yaml
# Allow developers to discover what their app needs
mode: interactive
dns:
  defaultAction: ask
network:
  defaultAction: ask
interactive:
  enabled: true
  timeout: 30
  autoLearn: true
  persistPath: "/tmp/learned-policy.yaml"
```

### CI/CD Pipeline

```yaml
# Strict allowlist for build isolation
mode: static
dns:
  defaultAction: deny
  allow:
    - "^.*\\.github\\.com$"
    - "^.*\\.npmjs\\.org$"
    - "^registry\\.fedoraproject\\.org$"
network:
  defaultAction: deny
  rules:
    - action: allow
      protocol: tcp
      cidr: "0.0.0.0/0"
      ports: [443]
tls:
  enabled: true
  dnsValidation: true
  rejectIPLiterals: true
```

### Production with Emergency Override

```yaml
# Base policy enforced, but SRE can approve exceptions
mode: hybrid
dns:
  defaultAction: deny
  allow: ["^.*\\.example\\.com$", "^.*\\.cloudflare\\.com$"]
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
interactive:
  enabled: true
  timeout: 60
  autoLearn: false  # Don't auto-learn in prod
  persistLearned: false
```

## Security Guarantees

### From PR #609 (Hardening)

- ✅ **SNI Spoofing:** Blocked via DNS cross-validation
- ✅ **IP Literal Bypass (CVE-2025-25255):** Rejected in SNI parser
- ✅ **Case Mismatch Exploit:** Normalized before matching
- ✅ **Hardcoded IP Bypass:** Caught by DNS validation requirement
- ✅ **TLS Fragmentation Attack:** Parser reassembles before extraction
- ✅ **ECH/ESNI:** Configurable handling per RFC 9849

### From PR #631 (Visibility)

- ✅ **Real-time Visibility:** SSE events for all policy decisions
- ✅ **Auditability:** All approved/denied requests logged
- ✅ **Policy Testing:** Interactive mode validates rules before production
- ✅ **Zero Trust Discovery:** Deny-all default with explicit approvals

## Migration Path

### For Users of PR #609

```bash
# Old approach (PR #609)
gvproxy --outbound-allow "^.*\\.example\\.com$" ...

# New unified approach
cat > policy.yaml <<EOF
mode: static
dns:
  defaultAction: deny
  allow: ["^.*\\.example\\.com$"]
tls:
  enabled: true
  dnsValidation: true
EOF

gvproxy --network-policy policy.yaml
```

### For Users of PR #631

```bash
# Old approach (PR #631)
gvproxy --secure-mode --services unix:///tmp/sock

# New unified approach (same command!)
gvproxy --secure-mode --services unix:///tmp/sock

# Or with explicit config
cat > policy.yaml <<EOF
mode: interactive
dns:
  defaultAction: ask
network:
  defaultAction: ask
interactive:
  enabled: true
  timeout: 30
  autoLearn: true
EOF

gvproxy --network-policy policy.yaml --services unix:///tmp/sock
```

## Implementation Effort

### Code Reuse

- **~90% of PR #609's SNI parser:** Reuse as-is in `pkg/services/policy/sni.go`
- **~80% of PR #631's filter engine:** Adapt to unified decision engine
- **~100% of PR #631's HTTP API:** Extend with new endpoints
- **~100% of both test suites:** Combine into unified test coverage

### New Code Required

- Unified configuration parser (~300 lines)
- Decision engine coordinator (~500 lines)
- Policy export logic (~200 lines)
- Hybrid mode orchestration (~300 lines)
- Integration glue (~400 lines)

**Total new code:** ~1700 lines + ~4000 lines reused from PRs = **~5700 lines total**

### Timeline Estimate

- Week 1-2: Core unification, decision engine
- Week 3-4: Interactive features, HTTP API
- Week 5: Integration, testing
- Week 6: Documentation, UI adaptation

**Total: 6 weeks for complete implementation**

## Next Steps

1. **Review this proposal** with maintainers and PR authors
2. **Get consensus** on configuration schema and API design
3. **Assign implementation** (can be split across multiple contributors)
4. **Create tracking issue** with implementation checklist
5. **Start with Phase 1** (core unification) as standalone PR
6. **Iterate** based on feedback and testing

## Questions for Discussion

1. Should the web UI be in-tree or separate repo?
2. Is YAML the right format for policy persistence, or also support JSON?
3. Should we support policy includes for large deployments? (`includes: [base.yaml, overrides.yaml]`)
4. Should runtime learned rules be separate from static rules in the export?
5. Do we need role-based access for the HTTP API (read-only vs admin)?

---

**Full proposal:** See `PROPOSAL_UNIFIED_FILTERING.md`
