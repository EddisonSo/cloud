# Service Agent System Design

**Date:** 2026-04-05
**Status:** Approved

## Overview

A multi-agent system for developing the edd-cloud monorepo. An orchestrator agent analyzes user intent and routes tasks to specialized service agents, each with deep knowledge of their respective service. Agents communicate exclusively through the orchestrator (no direct cross-communication).

## Goals

- **Development speed**: each agent knows its service's patterns, dependencies, and common tasks — no re-explaining context
- **Code quality**: agents enforce service-specific conventions and flag cross-service impacts
- **Scoped autonomy**: agents freely edit within their own service directory; cross-service changes require user visibility through the orchestrator

## Scope Enforcement

Scope restrictions (write scope, read-only access) are **prompt-based behavioral guidelines**, not technical sandboxes. Each agent's `.md` file instructs it to only write within its declared scope. Claude Code does not enforce filesystem restrictions on agents — the agent's prompt is the enforcement mechanism. If an agent writes outside its scope, the orchestrator should flag it in its synthesis step.

## Architecture

### Approach: Smart Orchestrator with Agent Composition

The orchestrator dispatches service agents in parallel for independent work and sequentially when there are dependencies. Each agent returns results to the orchestrator, which synthesizes and presents to the user.

```
User -> Orchestrator --+-- auth-dev (parallel)
                       +-- gateway-dev (parallel)
                       +-- then: infra-dev (after both complete)
```

No separate coordinator agent — the orchestrator handles sequencing directly.

### Dispatch Mechanism

The orchestrator uses Claude Code's **Agent tool** to spawn service agents. Each service agent's `.md` file in `.claude/agents/` is automatically registered as an agent type. The orchestrator invokes them by name (e.g., `auth-dev`) via the Agent tool's `subagent_type` parameter, with a task-specific prompt. For parallel dispatch, the orchestrator issues multiple `Agent` tool calls in a single response. For sequential dispatch, it waits for the previous agent's result before dispatching the next.

## Agent Inventory

### Orchestrator

| Agent | Scope | Model |
|-------|-------|-------|
| `cloud-orchestrator` | Dispatch only (no file writes) | sonnet |

### Service Agents — Opus (complex systems)

| Agent | Write Scope | Read-Only Access | Service |
|-------|-------------|------------------|---------|
| `auth-dev` | `edd-cloud-auth/` | `proto/auth/`, `manifests/` | Authentication (WebAuthn, sessions, JWT) |
| `services-dev` | `edd-cloud-interface/services/`, `edd-cloud-interface/pkg/` | `proto/registry/`, `proto/sfs/`, `proto/compute/`, `manifests/` | Go backend services (compute, registry, sfs, health) |
| `gateway-dev` | `edd-gateway/` | `proto/gateway/`, `manifests/` | Custom ingress gateway (routing, TLS, SSH) |
| `gfs-dev` | `go-gfs/` | `proto/`, `manifests/` | Distributed file system |
| `infra-dev` | `manifests/`, `proto/` | All service dirs | Kubernetes manifests, protobuf definitions |

### Service Agents — Sonnet (straightforward services)

| Agent | Write Scope | Read-Only Access | Service |
|-------|-------------|------------------|---------|
| `frontend-dev` | `edd-cloud-interface/frontend/` | `edd-cloud-interface/services/` | React/TypeScript dashboard |
| `log-dev` | `log-service/` | `proto/log/`, `manifests/` | Log ingestion and streaming |
| `monitor-dev` | `cluster-monitor/` | `proto/cluster/`, `manifests/` | Health checks, K8s watchers |
| `manager-dev` | `cluster-manager/` | `manifests/` | PTY management, cluster utilities |
| `alerting-dev` | `alerting-service/` | `proto/`, `manifests/` | Event-driven alerts (NATS) |
| `notification-dev` | `notification-service/` | `proto/notification/`, `manifests/` | Notification delivery |
| `docs-dev` | `edd-cloud-docs/` | All service dirs | Documentation site (Docusaurus) |

## Orchestrator Design

The orchestrator performs three functions:

### 1. Intent Analysis
Parses the user's request to determine: task type (fix, feature, refactor, debug, explain), affected service(s), and scope.

### 2. Routing (priority order)
1. User explicitly names a service -> route there
2. Changed/mentioned files map to a service directory -> route there
3. Request is ambiguous -> ask the user which service
4. Task spans multiple services -> dispatch in dependency order

### 3. Sequencing (for multi-service tasks)
Dependency order for cross-service changes:
1. `proto/` changes first (via `infra-dev`)
2. Backend services in parallel
3. `frontend-dev` after backends it depends on
4. `manifests/` after services (via `infra-dev`)
5. `docs-dev` last if API behavior changed

### What the orchestrator does NOT do
- Read or write code
- Make architectural decisions
- Run tests or builds

## Service Agent Structure

### Common Capabilities (all agents)
- Read files within their scope
- Write/edit files within their own service directory
- Run tests and builds for their service
- Access git history for their service
- Understand shared project patterns (gRPC, NATS, JWT, go-gfs client)

### Service-Specific Knowledge (baked into each agent)
- Entry points, directory structure, key files
- Dependencies (what this service calls, what calls it)
- Tech stack details (Go version, frameworks, libraries)
- Common tasks and patterns
- Testing patterns specific to the service
- Environment variables and config

### Cross-Service Awareness (read-only)
- Proto definitions the service consumes/produces
- NATS subjects the service publishes/subscribes to
- Other services it communicates with

### Boundary Rules
- Agent can freely edit files in its own directory
- Agent must flag when a change requires updates in another service
- The orchestrator handles dispatching follow-up work to other agents

### Agent Output Contract

Every agent must return a structured report to the orchestrator containing:

1. **Status**: `success` | `partial` | `failed`
2. **Files changed**: list of files created, modified, or deleted
3. **Tests run**: test commands executed and their pass/fail status
4. **Cross-service flags**: list of other services that may need changes, with reason
5. **Summary**: brief description of what was done

The orchestrator uses this report to decide whether to dispatch follow-up agents, present results, or escalate failures.

### Test Verification

Agents must run relevant tests before reporting success:
- Go services: `go build ./...` and `go test ./...` within the service directory
- Frontend: `npm run build` and `npm run lint` within `edd-cloud-interface/frontend/`
- If tests fail, the agent must attempt to fix the issue. If it cannot, report `status: partial` with the failure details.

### Error Handling

- **Agent fails (status: failed)**: orchestrator presents the failure to the user and asks how to proceed. Does not dispatch downstream agents.
- **Agent partially succeeds (status: partial)**: orchestrator presents what worked and what didn't. User decides whether to continue the sequence or stop.
- **Mis-routing**: if an agent determines the task is outside its scope, it reports `status: failed` with a routing suggestion. The orchestrator re-routes.
- **Multi-step sequence failure**: if step N fails in a multi-agent sequence, the orchestrator stops, presents all completed work from steps 1 through N-1, and asks the user whether to retry step N or abort.

## Interaction Flow

### Single-Service Task
```
User: "Add rate limiting to the gateway"
Orchestrator:
  1. Intent: feature, gateway
  2. Route: gateway-dev
  3. Dispatch gateway-dev
     -> Implements rate limiting
     -> Reports: "Added rate limiting middleware. No cross-service impact."
  4. Present results to user
```

### Multi-Service Task
```
User: "Add SSH key deletion"
Orchestrator:
  1. Intent: bug fix, SSH keys
  2. Route: auth-dev (API) + services-dev (UI)
  3. Dispatch auth-dev first
     -> Fixes DELETE handler
     -> Reports: "Fixed handler. Frontend may need API call check."
  4. Review report, dispatch services-dev
     -> Fixes frontend component
     -> Reports: "Updated API call"
  5. Synthesize and present both changes to user
  6. Flag if docs/manifests need updating
```

### Integration with Existing Agents

The following agents already exist in `.claude/agents/` and coexist with the new service agents:

| Existing Agent | Role | Integration |
|----------------|------|-------------|
| `commit-organizer` | Git commit workflow | Orchestrator invokes via Agent tool after development work completes |
| `security-vulnerability-scanner` | Security scanning | Invoked by `commit-organizer` during commit phase (unchanged) |
| `actions-monitor` | GitHub Actions monitoring | Orchestrator invokes after `commit-organizer` pushes |
| `docs-writer` | Documentation writing | Superseded by `docs-dev` for this project |
| `issue-diagnostician` | Bug diagnosis | Coexists — orchestrator can invoke for production debugging |
| `k8s-log-scanner` | Kubernetes log scanning | Coexists — orchestrator can invoke for cluster monitoring |

**Post-development flow:**
1. Development agents complete work
2. Orchestrator invokes `commit-organizer` (handles security scan, docs check, commit, push)
3. After push, orchestrator invokes `actions-monitor` to track the deployment

## File Layout

```
.claude/agents/
  # New agents
  cloud-orchestrator.md
  auth-dev.md
  services-dev.md
  frontend-dev.md
  gateway-dev.md
  gfs-dev.md
  log-dev.md
  monitor-dev.md
  manager-dev.md
  alerting-dev.md
  notification-dev.md
  infra-dev.md
  docs-dev.md
  # Existing agents (unchanged)
  commit-organizer.md
  security-vulnerability-scanner.md
  actions-monitor.md
  docs-writer.md
  issue-diagnostician.md
  k8s-log-scanner.md
```

## Usage

- **Default workflow**: invoke `cloud-orchestrator`, describe the task, it handles routing
- **Direct invocation**: bypass the orchestrator and invoke a service agent directly when you know exactly which service you're working in
- **Orchestrator triggers commit-organizer** at the end of development work (existing workflow preserved)

## Communication Model

Orchestrator-mediated only. Agents do not communicate directly with each other.

```
auth-dev: "I added a new endpoint, gateway needs a route"
  -> orchestrator
  -> gateway-dev: "Add route for new auth endpoint"
  -> orchestrator
  -> present both results to user
```

This gives:
- Single point of visibility for all cross-service decisions
- No cascading changes without user awareness
- Simpler debugging when something goes wrong
