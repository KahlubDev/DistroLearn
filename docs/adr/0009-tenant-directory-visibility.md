# ADR 0009: Tenant directory rows stay readable to the application role

- Status: accepted
- Date: 2026-10-14, accepted 2026-10-14
- Amends: `0003-tenancy-model.md`
- Related: `docs/reviews/017-compose.md` finding 6, `docs/plans/022-database.md`

## Context

`relrowsecurity` is `f` on `tenants`, and `services/migrations/00001_tenancy.sql` grants the
application role `SELECT` on it. A transaction scoped to tenant A reads every tenant row, so
one query returns `Institution A|eu-central-1` and `Institution B|eu-central-1`.

The review calls this a scope note rather than a leak of tenant-owned data, for a two-table
scaffold. It also says turning row security on now is cheap and doing it once membership
tables exist is not. That advice is right about cost and incomplete about mechanism, and the
mechanism is the whole decision.

`tenants` is the table ADR 0003's layer 2 resolves against: a user is mapped to an
institution with a role, checked on every request. That lookup happens before the request
knows which tenant it is for. A transaction that has set no tenant is not a mistake at that
point, it is the only correct state.

## Decision

**`tenants` rows stay readable to the application role, unscoped, for now. The isolation
boundary is the membership table, which does not exist yet.**

The alternative, a policy on `tenants` keyed on `app.tenant_id`, cannot work. Membership
resolution runs with no tenant set, the policy compares `id` against an empty setting, and
the comparison yields NULL, so every row is filtered out. The lookup that decides which
tenant a request belongs to would return nothing, and no request would resolve a tenant at
all. A policy on `tenants` is only correct once membership rows exist to filter against,
and then the policy keys on membership rather than on the tenant GUC.

Two further points support leaving the directory readable.

ADR 0003 already treats existence as non-confidential. A caller with no membership in the
requested tenant gets 404 rather than 403, specifically so the response does not confirm the
resource exists. A product that does not hide whether an institution exists gains little
from hiding the list of them.

`region` discloses nothing operationally yet. ADR 0002 fixes the MVP to one launch region
and ADR 0003 says `region` routes nothing, so the column carries no routing or placement
signal today.

## Consequences

- **Finding 6 from `docs/reviews/017-compose.md` is closed as an accepted interim risk, not
  fixed.** The application role can still read every tenant row. Accepting this ADR is the
  decision that the read is correct for now, not evidence that the table is protected. A
  later reader who finds the grant on `tenants` should find this ADR and the test named
  below, and should not file it as a new finding.
- The application role can read tenant names for every institution. If institution names
  turn out to be something a user without membership should not see, that is a product
  decision and this ADR is where it gets revisited. It is not a security defect today,
  because nothing in the scaffold treats the directory as confidential.
- ADR 0003 layer 2 names membership as a check on every request. Until that table exists,
  layers 3 and 4 are the only isolation the system has. The window is bounded by one
  migration, `00002_memberships.sql` in `docs/plans/022-database.md`, and it closes when
  that lands.
- When `memberships` lands it carries the boundary, and it gets row security on
  `tenant_id` in the same migration that creates it. `tenants` stays readable, because the
  membership table is now the thing that must not leak.
- `docs/plans/022-database.md` adds a test that asserts an unscoped read of `tenants`
  returns both rows. That test is what makes this decision executable rather than a comment:
  if someone turns on row security on `tenants` later, the test fails and names this ADR.

## Alternatives considered

| Option | Rejected | Reason |
|---|---|---|
| Row security on `tenants` keyed on `app.tenant_id` | Filters every row out of the unscoped membership lookup | Breaks tenant resolution rather than protecting it |
| Row security on `tenants` keyed on a membership `EXISTS` | Correct, and the destination | Needs a membership table that does not exist yet, and a `app.user_id` setting that does not exist yet |
| Revoke `SELECT` on `tenants` from the application role | Closes the read | Breaks membership resolution the same way the GUC policy does, with less to read |
| A view exposing only the current tenant's row | A per-request view | Same unscoped-lookup problem, plus a second object to keep in step with the schema |
| Denormalise the tenant name onto every tenant-owned table | Removes the lookup | Duplicates a directory row across every table and leaves it to be updated |

## Revisit triggers

Revisit when `memberships` lands, when institution names become something a user without
membership should not see, when a second EU region lands and `region` starts routing, or
when a public-facing surface ever renders a list of tenants.