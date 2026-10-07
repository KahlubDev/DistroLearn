# DistroLearn: Agent Handbook

Product: browser-based hands-on distributed-systems learning platform.
Multi-tenant, EU data residency option, ephemeral sandbox labs.

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

## Writing rules for docs

Write like an engineer explaining something to a teammate.

- No em dashes. Use a period, comma, or parentheses.
- Cut filler words: genuinely, deliberately, honestly, plainly, crucial, robust,
  seamless, leverage, comprehensive, "worth noting", "it's important to".
- No "not X, but Y" framing, no lists of exactly three adjectives, no summary
  sentence that repeats the paragraph.
- No narrating the document ("Stated plainly...", "This section covers...").
- Don't write in first person as the agent. State the fact or the open question.
- Not every bullet needs a bold lead-in. Use bold sparingly.
- Prefer concrete names, numbers and file paths over general claims.
- Say what you don't know once, where it matters, not as hedges throughout.
- Vary sentence length. Short is fine.
- Length: under one page per doc unless the ticket says otherwise. Cut anything
  nobody would miss.
- Before finishing, reread the doc and rewrite every sentence that sounds like a
  press release.

## Source documents

Decisions live in `docs/adr/`. Each ADR is accepted, proposed, or superseded.
Read the ADRs before planning work.

## Ticket workflow

Branch from `main`, open a PR, and do not merge it.

```
git checkout main
git pull
git checkout -b feature/NNN-short-name
```

Branch names: `feature/NNN-short-name`, or `fix/NNN-short-name` for a defect.
Commit as you go. Push the branch and open a PR against `main`. Merging is a human
decision.

## Definition of Done

A ticket is done when all of these hold:

- [ ] Code compiles (`make build`).
- [ ] Tests pass (`make test`).
- [ ] New tests exist for the change. A bug fix has a test that fails without it.
- [ ] Lint and typecheck pass (`make lint`).
- [ ] No unrelated changes in the diff.
- [ ] Docs updated where behaviour or a decision changed.

`make check` runs lint, typecheck, test, and build for web and Go.
