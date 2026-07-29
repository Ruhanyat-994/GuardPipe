## What
<!-- One or two sentences. -->

## Why
<!-- Link the issue. What problem does this solve? -->
Closes #

## How
<!-- Notable design decisions. Anything a reviewer would otherwise have to reverse-engineer. -->

## Type
- [ ] feat  - [ ] fix  - [ ] docs  - [ ] test  - [ ] refactor  - [ ] chore

## Checklist
- [ ] Follows the module boundary rule (no cross-module internal imports)
- [ ] Tests added or updated
- [ ] `make lint` and `make test` pass locally
- [ ] Documentation updated if behaviour or a contract changed
- [ ] No secrets, tokens, or keys added anywhere
- [ ] New dependencies justified below

## Security checklist (from documentation/12 §10)
- [ ] All SQL parameterised
- [ ] All user input validated at the boundary
- [ ] Authorisation checked in the service layer, not only the router
- [ ] Errors leak no internal detail
- [ ] Nothing sensitive added to logs
- [ ] New external calls have timeouts

## Schema changes
- [ ] None
- [ ] Migration included, numbered sequentially, `documentation/06` updated,
      two approvals requested

## New dependencies
<!-- name — why it is needed — why not stdlib. Delete if none. -->

## Screenshots
<!-- Required for any UI change. -->
