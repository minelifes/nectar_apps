# Contributing to Nectar Apps

Thanks for helping! A few ground rules:

1. **CLA.** Every contributor signs the [Contributor License Agreement](CLA.md) once — a bot asks on your first pull request.
2. **Discuss big changes first.** Open an issue before large features so we can agree on the design.
3. **Keep the contract.** API changes start in `api/openapi.yaml`. Breaking API changes bump `version.APIVersion`.
4. **Tests and lint.** `make lint test shellcheck` must pass. New behavior needs tests.
5. **Commits.** [Conventional commits](https://www.conventionalcommits.org) (`feat:`, `fix:`, `docs:`…) — they drive the changelog and version bumps.
6. **Security issues** go to support@techbrain.llc, not public issues.

## Code style

- Go: standard library first, small packages, errors wrapped with context, no panics in request paths.
- Never log or audit secrets (passwords, tokens, env values).
- Every write endpoint checks a permission (`internal/authz`) and records an audit entry.
- Shell: POSIX `sh`, passes `shellcheck -s sh`.
