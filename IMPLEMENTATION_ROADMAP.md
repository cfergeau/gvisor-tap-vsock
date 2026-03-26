# Implementation Roadmap: Unified Network Filtering

This document provides a concrete, step-by-step implementation plan for building the unified network filtering system.

## Phase 0: Preparation (Week 0)

### Tasks

- [ ] Create feature branch `feature/unified-network-filtering`
- [ ] Set up project structure
- [ ] Create tracking issue with checklist
- [ ] Identify code owners for review

### Directory Structure

```
pkg/
├── types/
│   ├── configuration.go         (extend existing)
│   └── network_policy.go        (NEW)
├── services/
│   ├── policy/                  (NEW package)
│   │   ├── engine.go           (decision engine)
│   │   ├── sni.go              (from PR #609)
│   │   ├── sni_test.go         (from PR #609)
│   │   ├── dns_filter.go       (integrate PR #631)
│   │   ├── network_filter.go   (integrate PR #631)
│   │   ├── interactive.go      (pending queue + approval)
│   │   ├── events.go           (SSE streaming)
│   │   └── export.go           (policy export)
│   ├── dns/
│   │   └── dns.go              (modify to use policy engine)
│   └── forwarder/
│       ├── tcp.go              (modify to use policy engine)
│       └── udp.go              (modify to use policy engine)
├── virtualnetwork/
│   ├── services.go             (modify to initialize policy engine)
│   └── virtualnetwork.go       (add policy HTTP endpoints)
cmd/gvproxy/
└── config.go                   (add --network-policy, --secure-mode flags)
examples/
├── network-policy-discovery.yaml
├── network-policy-ci-build.yaml
├── network-policy-production.yaml
└── network-policy-sandbox.yaml
```

## Phase 1: Core Policy Engine (Weeks 1-2)

### Goal

Build the foundation: configuration parsing, decision engine, and SNI parser integration.

### 1.1: Define Configuration Types

**File:** `pkg/types/network_policy.go`

```go
package types

import "regexp"

// NetworkPolicy defines the complete network filtering configuration.
type NetworkPolicy struct {
    Version      int                   `yaml:"version"`
    Mode         PolicyMode            `yaml:"mode"`
    DNS          DNSPolicy             `yaml:"dns"`
    Network      NetworkACLPolicy      `yaml:"network"`
    TLS          TLSPolicy             `yaml:"tls"`
    Interactive  InteractivePolicy     `yaml:"interactive"`
    Exemptions   PolicyExemptions      `yaml:"exemptions"`
}

type PolicyMode string

const (
    PolicyModeStatic      PolicyMode = "static"
    PolicyModeInteractive PolicyMode = "interactive"
    PolicyModeHybrid      PolicyMode = "hybrid"
)

type PolicyAction string

const (
    PolicyActionAllow PolicyAction = "allow"
    PolicyActionDeny  PolicyAction = "deny"
    PolicyActionAsk   PolicyAction = "ask"
)

type DNSPolicy struct {
    DefaultAction  PolicyAction `yaml:"defaultAction"`
    Allow          []string     `yaml:"allow"`           // regex patterns
    Deny           []string     `yaml:"deny"`            // regex patterns
    AllowWildcard  []string     `yaml:"allowWildcard"`   // *.example.com

    // Compiled patterns (populated at runtime)
    allowPatterns  []*regexp.Regexp
    denyPatterns   []*regexp.Regexp
}

type NetworkACLPolicy struct {
    DefaultAction PolicyAction  `yaml:"defaultAction"`
    Rules         []NetworkRule `yaml:"rules"`
}

type NetworkRule struct {
    Action      PolicyAction `yaml:"action"`
    Protocol    string       `yaml:"protocol"`  // tcp, udp, *
    CIDR        string       `yaml:"cidr"`
    Ports       []int        `yaml:"ports"`     // [80, 443] or nil for all
    Description string       `yaml:"description"`

    // Compiled CIDR (populated at runtime)
    ipNet *net.IPNet
}

type TLSPolicy struct {
    Enabled          bool `yaml:"enabled"`
    RejectIPLiterals bool `yaml:"rejectIPLiterals"`
    DNSValidation    bool `yaml:"dnsValidation"`
    LogALPN          bool `yaml:"logALPN"`
    AllowECH         bool `yaml:"allowECH"`
}

type InteractivePolicy struct {
    Enabled        bool   `yaml:"enabled"`
    Timeout        int    `yaml:"timeout"`        // seconds
    AutoLearn      bool   `yaml:"autoLearn"`
    PersistLearned bool   `yaml:"persistLearned"`
    PersistPath    string `yaml:"persistPath"`
    Events         EventsConfig `yaml:"events"`
}

type EventsConfig struct {
    Enabled    bool `yaml:"enabled"`
    BufferSize int  `yaml:"bufferSize"`
}

type PolicyExemptions struct {
    AllowGateway bool `yaml:"allowGateway"`
    AllowDHCP    bool `yaml:"allowDHCP"`
}
```

### 1.2: Configuration Parser

**File:** `pkg/types/network_policy.go` (continued)

```go
// LoadNetworkPolicy loads and validates a network policy from YAML.
func LoadNetworkPolicy(path string) (*NetworkPolicy, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read policy file: %w", err)
    }

    var policy NetworkPolicy
    if err := yaml.Unmarshal(data, &policy); err != nil {
        return nil, fmt.Errorf("parse policy YAML: %w", err)
    }

    // Validate version
    if policy.Version != 1 {
        return nil, fmt.Errorf("unsupported policy version: %d", policy.Version)
    }

    // Compile DNS regex patterns
    if err := policy.DNS.compile(); err != nil {
        return nil, fmt.Errorf("compile DNS patterns: %w", err)
    }

    // Compile network CIDR rules
    if err := policy.Network.compile(); err != nil {
        return nil, fmt.Errorf("compile network rules: %w", err)
    }

    return &policy, nil
}

func (d *DNSPolicy) compile() error {
    // Compile allow patterns
    for _, pattern := range d.Allow {
        re, err := regexp.Compile(pattern)
        if err != nil {
            return fmt.Errorf("invalid allow pattern %q: %w", pattern, err)
        }
        d.allowPatterns = append(d.allowPatterns, re)
    }

    // Compile deny patterns
    for _, pattern := range d.Deny {
        re, err := regexp.Compile(pattern)
        if err != nil {
            return fmt.Errorf("invalid deny pattern %q: %w", pattern, err)
        }
        d.denyPatterns = append(d.denyPatterns, re)
    }

    // Convert wildcards to regex
    for _, wildcard := range d.AllowWildcard {
        pattern := wildcardToRegex(wildcard)
        re := regexp.MustCompile(pattern)
        d.allowPatterns = append(d.allowPatterns, re)
    }

    return nil
}

func wildcardToRegex(wildcard string) string {
    // *.example.com -> ^.*\.example\.com$
    escaped := regexp.QuoteMeta(wildcard)
    pattern := strings.ReplaceAll(escaped, `\*`, `.*`)
    return "^" + pattern + "$"
}
```

### 1.3: SNI Parser (Copy from PR #609)

**File:** `pkg/services/policy/sni.go`

Copy the entire SNI parser from PR #609:
- `PeekSNI()` function
- `parseClientHello()` helper
- All security validations (IP literals, ECH, case normalization)
- Complete test suite in `sni_test.go`

**No modifications needed** - the parser is self-contained.

### 1.4: Decision Engine Skeleton

**File:** `pkg/services/policy/engine.go`

```go
package policy

import (
    "context"
    "net"
    "sync"

    "github.com/containers/gvisor-tap-vsock/pkg/types"
)

// Engine is the unified policy decision engine.
type Engine struct {
    policy      *types.NetworkPolicy
    interactive *InteractiveQueue  // nil if not in interactive mode

    // Runtime learned rules
    learnedDNS     map[string]bool  // domain -> allowed
    learnedNetwork map[string]bool  // "proto:ip:port" -> allowed
    mu             sync.RWMutex
}

// NewEngine creates a new policy engine.
func NewEngine(policy *types.NetworkPolicy) (*Engine, error) {
    e := &Engine{
        policy:         policy,
        learnedDNS:     make(map[string]bool),
        learnedNetwork: make(map[string]bool),
    }

    if policy.Interactive.Enabled {
        e.interactive = NewInteractiveQueue(policy.Interactive.Timeout)
    }

    return e, nil
}

// EvaluateDNS decides whether a DNS query should be allowed.
func (e *Engine) EvaluateDNS(ctx context.Context, domain string) (PolicyDecision, error) {
    // TODO: Implement in Phase 1
    return PolicyDecision{}, nil
}

// EvaluateTCP decides whether a TCP connection should be allowed.
func (e *Engine) EvaluateTCP(ctx context.Context, destIP net.IP, destPort int, sni string) (PolicyDecision, error) {
    // TODO: Implement in Phase 1
    return PolicyDecision{}, nil
}

// EvaluateUDP decides whether a UDP connection should be allowed.
func (e *Engine) EvaluateUDP(ctx context.Context, destIP net.IP, destPort int) (PolicyDecision, error) {
    // TODO: Implement in Phase 1
    return PolicyDecision{}, nil
}

type PolicyDecision struct {
    Allow  bool
    Reason string
    ID     string  // For pending requests
}
```

### Deliverables (Week 2)

- [ ] Configuration types defined
- [ ] YAML parser working with validation
- [ ] SNI parser integrated from PR #609
- [ ] Decision engine skeleton with stubs
- [ ] Unit tests for configuration parsing
- [ ] Unit tests for SNI parser (from PR #609)

## Phase 2: Decision Logic (Weeks 3-4)

### Goal

Implement the actual policy decision algorithms.

### 2.1: DNS Decision Logic

**File:** `pkg/services/policy/engine.go` (continued)

```go
func (e *Engine) EvaluateDNS(ctx context.Context, domain string) (PolicyDecision, error) {
    // Normalize domain
    domain = strings.ToLower(strings.TrimSuffix(domain, "."))

    // Check learned allowlist first
    e.mu.RLock()
    if e.learnedDNS[domain] {
        e.mu.RUnlock()
        return PolicyDecision{Allow: true, Reason: "learned"}, nil
    }
    e.mu.RUnlock()

    // Check static deny patterns
    for _, re := range e.policy.DNS.denyPatterns {
        if re.MatchString(domain) {
            return PolicyDecision{Allow: false, Reason: "denied by pattern"}, nil
        }
    }

    // Check static allow patterns
    for _, re := range e.policy.DNS.allowPatterns {
        if re.MatchString(domain) {
            return PolicyDecision{Allow: true, Reason: "allowed by pattern"}, nil
        }
    }

    // No match - apply default action
    switch e.policy.DNS.DefaultAction {
    case types.PolicyActionAllow:
        return PolicyDecision{Allow: true, Reason: "default allow"}, nil
    case types.PolicyActionDeny:
        return PolicyDecision{Allow: false, Reason: "default deny"}, nil
    case types.PolicyActionAsk:
        if e.interactive != nil {
            id := e.interactive.EnqueueDNS(domain)
            return PolicyDecision{Allow: false, Reason: "pending approval", ID: id}, nil
        }
        // Fallback to deny if interactive is disabled
        return PolicyDecision{Allow: false, Reason: "no interactive mode"}, nil
    }

    return PolicyDecision{Allow: false, Reason: "invalid policy"}, nil
}
```

### 2.2: TCP Decision Logic with SNI Validation

**File:** `pkg/services/policy/engine.go` (continued)

```go
func (e *Engine) EvaluateTCP(ctx context.Context, destIP net.IP, destPort int, rawConn net.Conn) (PolicyDecision, error) {
    // For port 443, try to extract SNI
    var sni string
    var alpn []string

    if destPort == 443 && e.policy.TLS.Enabled {
        result, err := PeekSNI(rawConn)
        if err != nil && err != ErrNotTLS {
            return PolicyDecision{Allow: false, Reason: "TLS parse error"}, err
        }

        if err == nil {
            sni = result.SNI
            alpn = result.ALPN

            // Security validations from PR #609
            if e.policy.TLS.RejectIPLiterals {
                if net.ParseIP(sni) != nil {
                    log.Warnf("Rejected IP literal in SNI: %q (CVE-2025-25255)", sni)
                    return PolicyDecision{Allow: false, Reason: "IP literal in SNI"}, nil
                }
            }

            if e.policy.TLS.LogALPN && len(alpn) > 0 {
                log.Debugf("ALPN protocols for %s: %v", sni, alpn)
            }

            // DNS cross-validation
            if e.policy.TLS.DNSValidation && sni != "" {
                if err := e.validateSNI(sni, destIP); err != nil {
                    log.Warnf("SNI validation failed: %v", err)
                    return PolicyDecision{Allow: false, Reason: "SNI mismatch"}, nil
                }
            }
        }
    }

    // Check DNS policy if SNI is available
    if sni != "" {
        dnsDecision, err := e.EvaluateDNS(ctx, sni)
        if err != nil || !dnsDecision.Allow {
            return dnsDecision, err
        }
    }

    // Check network ACL rules
    return e.evaluateNetworkACL(ctx, "tcp", destIP, destPort, sni)
}

func (e *Engine) validateSNI(sni string, destIP net.IP) error {
    ips, err := net.LookupIP(sni)
    if err != nil {
        return fmt.Errorf("DNS lookup failed for %q: %w", sni, err)
    }

    for _, ip := range ips {
        if ip.Equal(destIP) {
            return nil
        }
    }

    return fmt.Errorf("SNI %q resolved to %v, but connecting to %v", sni, ips, destIP)
}
```

### Deliverables (Week 4)

- [ ] DNS decision logic implemented
- [ ] TCP decision logic with SNI validation
- [ ] UDP decision logic
- [ ] Network ACL matching
- [ ] Learned rule management
- [ ] Unit tests for all decision paths
- [ ] Integration tests

## Phase 3: Interactive Mode (Week 5)

### Goal

Build the pending queue, approval API, and SSE events.

### 3.1: Interactive Queue

**File:** `pkg/services/policy/interactive.go`

```go
package policy

type InteractiveQueue struct {
    timeout  time.Duration
    pending  map[string]*PendingRequest
    events   chan PolicyEvent
    mu       sync.RWMutex
}

type PendingRequest struct {
    ID        string
    Type      string  // "dns" or "network"
    Domain    string  // for DNS
    Protocol  string  // for network
    IP        string
    Port      int
    SNI       string
    ALPN      []string
    Timestamp time.Time
    ExpiresAt time.Time
    Result    chan bool  // approval channel
}

func (q *InteractiveQueue) EnqueueDNS(domain string) string {
    id := generateID()
    req := &PendingRequest{
        ID:        id,
        Type:      "dns",
        Domain:    domain,
        Timestamp: time.Now(),
        ExpiresAt: time.Now().Add(q.timeout),
        Result:    make(chan bool, 1),
    }

    q.mu.Lock()
    q.pending[id] = req
    q.mu.Unlock()

    q.events <- PolicyEvent{Type: "dns.pending", Data: req}

    go q.waitForTimeout(id)

    return id
}

func (q *InteractiveQueue) Approve(id string, autoLearn bool) error {
    q.mu.Lock()
    req, exists := q.pending[id]
    delete(q.pending, id)
    q.mu.Unlock()

    if !exists {
        return fmt.Errorf("request not found: %s", id)
    }

    req.Result <- true
    q.events <- PolicyEvent{Type: req.Type + ".approved", Data: req}

    return nil
}
```

### 3.2: HTTP API Endpoints

**File:** `pkg/virtualnetwork/policy_api.go` (NEW)

```go
package virtualnetwork

func (n *VirtualNetwork) handlePolicyPending(w http.ResponseWriter, r *http.Request) {
    if n.policyEngine == nil || n.policyEngine.interactive == nil {
        http.Error(w, "interactive mode not enabled", http.StatusNotFound)
        return
    }

    pending := n.policyEngine.interactive.ListPending()
    json.NewEncoder(w).Encode(pending)
}

func (n *VirtualNetwork) handlePolicyApprove(w http.ResponseWriter, r *http.Request) {
    var req struct {
        ID       string `json:"id"`
        Remember bool   `json:"remember"`
        Persist  bool   `json:"persist"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if err := n.policyEngine.Approve(req.ID, req.Remember); err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }

    json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

func (n *VirtualNetwork) handlePolicyEvents(w http.ResponseWriter, r *http.Request) {
    // SSE implementation
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "SSE not supported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    events := n.policyEngine.SubscribeEvents()
    defer n.policyEngine.UnsubscribeEvents(events)

    for {
        select {
        case event := <-events:
            fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.JSON())
            flusher.Flush()
        case <-r.Context().Done():
            return
        }
    }
}
```

### Deliverables (Week 5)

- [ ] Pending request queue with timeout
- [ ] HTTP API for approval/denial
- [ ] SSE event streaming
- [ ] Policy export endpoint
- [ ] Auto-learn and persistence logic
- [ ] Integration tests for interactive workflows

## Phase 4: Integration (Week 6)

### Goal

Wire everything together and add comprehensive tests.

### 4.1: Modify DNS Server

**File:** `pkg/services/dns/dns.go`

```go
// Modify addAnswers() to use policy engine
func (h *dnsHandler) addAnswers(m *dns.Msg) {
    for _, q := range m.Question {
        if done := h.addLocalAnswers(m, q); done {
            return
        }

        // NEW: Check policy engine
        if h.policyEngine != nil {
            domain := strings.ToLower(strings.TrimSuffix(q.Name, "."))
            decision, err := h.policyEngine.EvaluateDNS(context.Background(), domain)
            if err != nil || !decision.Allow {
                log.Debugf("DNS policy denied %q: %s", domain, decision.Reason)
                m.Rcode = dns.RcodeNameError
                return
            }
        }

        // Proceed with upstream resolution...
    }
}
```

### 4.2: Modify TCP Forwarder

**File:** `pkg/services/forwarder/tcp.go`

```go
// Modify handle() to use policy engine
func (f *Forwarder) handle(conn net.Conn, ep *gvisorNet.Endpoint) {
    destIP := ep.DestIP
    destPort := ep.DestPort

    // NEW: Check policy engine
    if f.policyEngine != nil {
        decision, err := f.policyEngine.EvaluateTCP(context.Background(), destIP, destPort, conn)
        if err != nil || !decision.Allow {
            log.Debugf("Network policy denied %v:%d: %s", destIP, destPort, decision.Reason)
            conn.Close()
            return
        }
    }

    // Proceed with connection...
}
```

### Deliverables (Week 6)

- [ ] DNS server integrated
- [ ] TCP forwarder integrated
- [ ] UDP forwarder integrated
- [ ] CLI flags implemented
- [ ] End-to-end integration tests
- [ ] Documentation updates
- [ ] Example configurations tested

## Testing Strategy

### Unit Tests

- Configuration parsing and validation
- SNI parser (from PR #609 - ~2000 test cases)
- Policy decision logic for all combinations
- Interactive queue operations
- HTTP API endpoints

### Integration Tests

```go
func TestDiscoveryWorkflow(t *testing.T) {
    // 1. Start gvproxy with --secure-mode
    // 2. Attempt connections (should be pending)
    // 3. Approve via API
    // 4. Verify connections succeed
    // 5. Export policy
    // 6. Verify exported YAML matches approvals
}

func TestStaticPolicy(t *testing.T) {
    // 1. Load static policy
    // 2. Attempt allowed connection → succeeds
    // 3. Attempt denied connection → fails
    // 4. Verify SNI validation blocks spoofing
}

func TestHybridMode(t *testing.T) {
    // 1. Load policy with base rules + interactive
    // 2. Known destination → auto-allowed
    // 3. Unknown destination → pending
    // 4. Approve unknown → succeeds
}
```

### Security Tests (From PR #609)

- IP literal in SNI (CVE-2025-25255)
- SNI spoofing via DNS mismatch
- Case-bypass attempts
- ECH/ESNI handling
- TLS fragmentation attacks

## Success Criteria

- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Security tests from PR #609 pass
- [ ] Performance: < 5ms p99 overhead for SNI parsing
- [ ] Documentation complete
- [ ] Example configs work
- [ ] Migration guide written
- [ ] Both PR authors review and approve

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance regression | High | Add benchmarks, implement SNI cache |
| API incompatibility | Medium | Version API endpoints, provide migration guide |
| Complex configuration | Medium | Provide CLI shortcuts, example files, validation |
| Security bypass | High | Port all security tests from PR #609, add fuzzing |
| Merge conflicts with PRs | Low | Coordinate with PR authors, rebase frequently |

## Post-Implementation

- [ ] Create follow-up issues for Phase 2 features (auth, persistence, UI)
- [ ] Update CLAUDE.md with policy configuration patterns
- [ ] Blog post / announcement
- [ ] Solicit feedback from early adopters
- [ ] Consider presenting at community meeting
