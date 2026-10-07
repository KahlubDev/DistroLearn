# DistroLearn: Agent Handbook

Product: browser-based hands-on distributed-systems learning platform
(multi-tenant, EU data residency option, ephemeral sandbox labs).

## Rules
1. Do only the task in your ticket. No refactors, no extra features.
2. Never make architecture decisions. If the plan is unclear, stop and ask.
3. Read the files listed in the ticket before writing code.
4. Work only on the branch named in the ticket.
5. Every change needs tests. Run lint, typecheck, and tests before finishing.
6. End every session with a summary: what changed, what's left, any blockers.

## Roles
researcher → writes docs/research/NNN-*.md (no code)
planner → writes docs/plans/NNN-*.md (no code)
scaffolder → creates skeleton, compiles, no feature logic
builder → implements one planned ticket
qa → tests and reports in docs/reviews/, doesn't fix
security → audits auth/tenancy/labs, doesn't fix
