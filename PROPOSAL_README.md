# Network Filtering Proposal - Quick Start

This directory contains a comprehensive proposal to unify PR #609 and PR #631 into a single network filtering system for gvisor-tap-vsock.

## Created on 2026-03-26

## Documents Overview

### Core Proposal Documents
- **[PROPOSAL_UNIFIED_FILTERING.md](PROPOSAL_UNIFIED_FILTERING.md)** - Full technical specification (21KB)
  - Architecture and design
  - Complete API specification
  - Security considerations
  - 6-week implementation plan

- **[PROPOSAL_SUMMARY.md](PROPOSAL_SUMMARY.md)** - Executive summary (11KB)
  - Quick reference
  - Three operating modes
  - Comparison matrix
  - Migration paths

- **[IMPLEMENTATION_ROADMAP.md](IMPLEMENTATION_ROADMAP.md)** - Detailed build plan (21KB)
  - Week-by-week breakdown
  - Code examples
  - Testing strategy
  - Success criteria

### Example Configurations
- **[examples/network-policy-discovery.yaml](examples/network-policy-discovery.yaml)** - Development/learning mode
- **[examples/network-policy-ci-build.yaml](examples/network-policy-ci-build.yaml)** - CI/CD isolation
- **[examples/network-policy-production.yaml](examples/network-policy-production.yaml)** - Production with override
- **[examples/network-policy-sandbox.yaml](examples/network-policy-sandbox.yaml)** - Malware analysis

## Context

### Problem
Issue #630 requests network filtering with deny/allow lists. Two PRs were submitted with different approaches:

- **PR #609** - Security-hardened static allowlist
  - SNI extraction from TLS ClientHello
  - DNS cross-validation to prevent spoofing
  - CVE-2025-25255 mitigation
  - 4600+ lines of tests

- **PR #631** - Interactive approval mode
  - Real-time approval via HTTP API
  - ACL-based rules (CIDR, port, protocol)
  - Auto-learning approved destinations
  - Svelte web UI for management

### Solution
This proposal **unifies both approaches** into a single system with three operating modes:

1. **Static** - PR #609's security hardening for production
2. **Interactive** - PR #631's discovery/approval for development
3. **Hybrid** - NEW: Static rules + interactive override

## Quick Reference

### Discovery Workflow (Interactive Mode)
```bash
# Start with deny-all + approval UI
gvproxy --secure-mode --services unix:///tmp/gvproxy.sock

# Run your app, approve connections
curl --unix-socket /tmp/gvproxy.sock http:/unix/policy/pending
curl --unix-socket /tmp/gvproxy.sock -X POST http:/unix/policy/approve \
  -d '{"id": "pending-net-1", "remember": true}'

# Export learned policy
curl --unix-socket /tmp/gvproxy.sock http:/unix/policy/export > policy.yaml
```

### Production Deployment (Static Mode)
```bash
# Deploy with exported policy
gvproxy --network-policy policy.yaml

# All enforcement is automatic, no user interaction needed
# SNI validation prevents bypasses
```

## Next Steps

1. **Review** - Share with maintainers and PR authors
2. **Discuss** - Post to issue #630 or create GitHub discussion
3. **Refine** - Incorporate feedback on config schema and API
4. **Implement** - Follow 6-week roadmap
5. **Test** - Comprehensive test suite from both PRs

## Key Files to Read First

1. **Start here:** [PROPOSAL_SUMMARY.md](PROPOSAL_SUMMARY.md) - 5-minute read
2. **Then:** [PROPOSAL_UNIFIED_FILTERING.md](PROPOSAL_UNIFIED_FILTERING.md) - 20-minute read
3. **For implementation:** [IMPLEMENTATION_ROADMAP.md](IMPLEMENTATION_ROADMAP.md)
4. **For examples:** Browse `examples/` directory

## Related Links

- Issue: https://github.com/containers/gvisor-tap-vsock/issues/630
- PR #609: https://github.com/containers/gvisor-tap-vsock/pull/609
- PR #631: https://github.com/containers/gvisor-tap-vsock/pull/631

## Contact

Created via Claude Code analysis of PRs #609 and #631.
For questions, reference this proposal in issue #630.
