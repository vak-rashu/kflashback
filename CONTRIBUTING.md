# Contributing to kflashback

Thank you for your interest in contributing to kflashback! This document provides guidelines and information for contributors.

## Code of Conduct

This project follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). By participating, you are expected to uphold this code.

## How to Contribute

### Reporting Issues

- Use GitHub Issues to report bugs or request features.
- Search existing issues before creating a new one.
- Include steps to reproduce, expected behavior, and actual behavior.
- Include Kubernetes version, kflashback version, and relevant logs.

### Pull Requests

1. **Fork** the repository and create a branch from `main`.
2. **Sign off** your commits using `git commit -s` (DCO requirement).
3. **Write tests** for new functionality.
4. **Run checks** locally before submitting:
   ```bash
   make fmt
   make vet
   make lint
   make test
   ```
5. **Keep PRs focused** - one feature or fix per PR.
6. **Update documentation** if your change affects user-facing behavior.

### Developer Certificate of Origin (DCO)

All commits must be signed off to certify you have the right to submit the code:

```
Signed-off-by: Your Name <your.email@example.com>
```

Use `git commit -s` to automatically add this.

## Development Setup

### Prerequisites

- Go 1.22+
- Node.js 20+
- Docker (for image builds)
- A Kubernetes cluster (kind, minikube, etc.)
- `kubectl` configured

### Building

```bash
# Clone the repository
git clone https://github.com/kflashback/kflashback.git
cd kflashback

# Build the binary
make build

# Install UI dependencies and build
make ui-install
make ui-build

# Run locally against your cluster
make run
```

### Running Tests

```bash
make test
```

### Code Generation

After modifying CRD types in `api/v1alpha1/types.go`:

```bash
make generate
```

## Project Structure

| Directory | Description |
|-----------|-------------|
| `api/v1alpha1/` | CRD type definitions |
| `cmd/kflashback/` | Main entrypoint |
| `internal/controller/` | Kubernetes controllers |
| `internal/diff/` | JSON diff/patch engine |
| `internal/server/` | REST API server |
| `internal/storage/` | Storage backends |
| `ui/` | React dashboard |
| `config/` | Kubernetes manifests |

## Style Guide

- **Go**: Follow standard Go conventions. Run `make fmt` and `make vet`.
- **TypeScript/React**: Follow existing patterns in the UI codebase.
- **Commits**: Use conventional commit messages (`feat:`, `fix:`, `docs:`, etc.).

## Release Process

Releases are automated via GitHub Actions on tag push. Version tags follow semantic versioning (`v0.1.0`).

## Getting Help

- Open a GitHub Issue for bugs or feature requests.
- Start a GitHub Discussion for questions or ideas.
