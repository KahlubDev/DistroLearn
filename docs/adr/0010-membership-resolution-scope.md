# ADR 0010: A user scope for membership resolution

- Status: proposed
- Date: 2026-10-14
- Amends: `0003-tenancy-model.md`, `0008-tenant-scope-discipline.md`
- Related: `0006-db-pooler.md`, `0009-tenant-directory-visibility.md`, `docs/plans/022-database.md`

## Context

ADR 0003 layer 2 resolves a request's tenant from membership: load the caller's
memberships, then apply the role check. That lookup runs before any tenant is known,
which is the one point in a request where `app.tenant_id` cannot be set.

ADR 0009 made the same observation about `tenants` and decided the directory stays
readable unscoped, because a policy keyed on `app.tenant_id` filters every row out of
exactly that lookup. `memberships` has no such escape. Its rows belong to people, so
reading it unscoped returns the whole directory of who belongs to what, which is the
thing layer 2 exists to keep private.

So the membership table needs a second scope, known at the moment the tenant is unknown.

ADR 0008 decided the tenant GUC is set in one place, with `SetTenant` unexported and a
source-scanning test behind it. Its revisit trigger already names this case: when
`memberships` lands and a second GUC needs the same discipline.

The discipline is not ceremony. `docs/reviews/017-compose.md` measures it: flipping the
third argument of `set_config` from `true` to `false` leaks one tenant's rows to a
transaction that set no tenant, and the poisoned server connection outlives the code
revert until PgBouncer restarts. A user value on the same pooled connection fails the
same way, and leaks a person's memberships rather than an institution's rows.

## Decision

**A second transaction-local setting, `app.user_id`, set through the same helper as the
tenant and nowhere else.**

**`InTenant` takes the user and an optional tenant.** The signature gains a `userID`
parameter and the tenant becomes nullable. `InTenant` always sets `app.user_id` with
`set_config(..., true)`, and sets `app.tenant_id` only when a tenant is passed. No new
exported setter. `SetUser` does not exist.

The two phases of a request are mutually exclusive:

| Phase | `app.user_id` | `app.tenant_id` | `memberships` read returns |
|---|---|---|---|
| Membership resolution | set | unset | this caller's own rows |
| Tenant-scoped work | set | set | the tenant's rows |

A transaction with neither set reads nothing from either. That is what the ADR 0006
variants already assert for the tenant GUC, and it still holds.

### The policy on memberships

Read and write do not have the same shape here. The asymmetry is deliberate.

`USING` carries two branches, guarded so exactly one can be true:

```sql
USING (
    tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
    OR (
        NULLIF(current_setting('app.tenant_id', true), '') IS NULL
        AND user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
    )
)
```

With a tenant set, only the first branch can match, so an in-tenant read is the tenant
roster and nothing else. With no tenant set, only the second can match, so the sign-in
read is the caller's own rows and nobody else's.

The unguarded form, `tenant_id = ... OR user_id = ...`, is wrong, and it is worth naming
because it is the shorter thing to type. With both settings present it lets a caller
scoped to tenant A also read their own membership row in tenant B, so a roster query
returns rows from a tenant the query was not scoped to. The row is the caller's own, so
nothing confidential crosses a user boundary, but a result set mixing a roster with
out-of-scope rows is the kind of thing a handler misreads as a membership.

`WITH CHECK` carries only the tenant branch:

```sql
WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
```

A membership row always has a tenant, and ADR 0003 gives membership writes to an admin
inside one institution. No legitimate write happens in the pre-tenant phase, so admitting
one would be a hole: a caller holding only `app.user_id` could add themselves to any
tenant.

### Sign-in, one user with two memberships

The path this exists for:

1. Verify the Cognito token and take the subject as the user.
2. Open a transaction through `InTenant(userID, nil)`. `app.user_id` is set, `app.tenant_id` is not.
3. `SELECT tenant_id, role FROM memberships`. Row security returns exactly this caller's rows, across every tenant, and no other user's.
4. Present the tenants, or pick one and carry it through the rest of the request.
5. Every later transaction is `InTenant(userID, tenantID)` with both settings set, so reads key on the tenant.

A caller in two institutions gets two rows at step 3 and one tenant at step 5, and
authorization uses the membership for the institution owning the resource, as ADR 0003
already requires. The two-membership case is what proves the user branch is a filter
rather than a tenant lookup in disguise: if it silently fell back to the tenant clause,
step 3 would return nothing and sign-in would appear to work for single-tenant callers
only.

### Reset behaviour is the same rule

`app.user_id` is transaction-local for the reason ADR 0008 measured. A session-level
user value would survive commit, and the next request to land on that server connection
would read that person's memberships. `server_reset_query_always` stays 0, so nothing
clears it, and ADR 0008's measured table applies to this setting unchanged. The
source-scanning test is extended rather than duplicated.

## Consequences

- `InTenant` gains a user parameter and an optional tenant. Every call site changes,
  including the three ADR 0006 variants in `tenant_isolation_test.go`. The call sites
  change mechanically; the variants' assertions do not.
- A nil tenant is now expressible, so the helper can be called without scoping to a
  tenant. The source-scanning test is what stops that becoming the default.
- `docs/plans/022-database.md` line 55 specifies the `memberships` policy as the same
  shape as `labs_tenant_isolation`. That row is superseded by the policy above. The
  `labs` policy is unchanged and stays single-branch, since `labs` has no pre-tenant
  phase.
- Two settings are now set for a tenant-scoped request. A call that passes a user but no
  tenant, or neither, gets a transaction that reads nothing rather than an error.
  Whether the helper should reject that outright is not decided here.
- The user identifier is the Cognito subject, stored as `uuid` per
  `docs/plans/022-database.md` line 49. This ADR does not change that.

## Tests

Three, all written by the builder.

**`TestMembership_SignInSeesOnlyOwnRows`.** Two users, each holding a membership in the
same tenant, so a policy that filtered on the tenant alone would return both rows and the
test would prove nothing. Seed through the owner role. Open a transaction with
`app.user_id` for the first caller and no tenant, read `memberships`, assert exactly that
caller's row. Repeat for the second. Put both on one pooled connection, so the second read
runs on the connection the first returned, which is what catches a user value that did not
reset.

**`TestMembership_SignInReturnsBothOfOneUsersMemberships`.** One caller with two
memberships in distinct tenants, so the primary key `(user_id, tenant_id)` holds and the
second row is a second insert rather than a role change. In one pre-tenant transaction,
read and assert both rows come back and no third. Then set a tenant, read again, assert
only that tenant's row. That second read is the branch switch in the policy.

**`TestUserScopeSetOnlyInHelper`.** Extends the source-scanning test that
`docs/plans/022-database.md` line 122 names, rather than adding a second reader of every
`.go` file under `services/`. It fails if `app.user_id` appears outside
`services/internal/db/db.go`, if `set_config` appears with a third argument other than
`true`, or if a literal `SET app.user_id` appears anywhere. Paste the run with `true` and
the run with the bare `SET`, the two runs the plan already asks for on the tenant GUC.

The three ADR 0006 variants are unchanged and must still pass. They set only
`app.tenant_id`, and a second setting does not move their assertions.

## Alternatives considered

| Option | Rejected | Reason |
|---|---|---|
| A second exported entry point, `InUserScope` or an exported `SetUser` | Two entry points, which is the shape ADR 0008 removed when it unexported `SetTenant` | The measured regression in `docs/reviews/017-compose.md` came from a helper that was easy to reach. A second reachable helper doubles the surface the scan test polices |
| Plain `SET app.user_id` at session scope | A session-level value survives on a pooled server connection and leaks to the next request | The failure ADR 0008 measured for the tenant GUC, applied to a value identifying a person rather than an institution |
| A `SECURITY DEFINER` function taking the user id as an argument | Correct results, and it bypasses row security entirely | Row security is the backstop ADR 0003 relies on. A definer function reads `memberships` with the owner's privileges, so a mistake in its body is a total isolation failure rather than a filtered one |
| Filter the policy by a membership list passed in a GUC | No second setting, so nothing new to reset | The same pooling hazard, and worse: the value grows with the caller's membership count, so one connection expects a different value per request |
| Resolve membership in Go from a table the role can read unscoped | No GUC, so no reset problem | That table is `memberships`. Reading it unscoped is the leak this ADR closes, and it is why the application role gets no second unscoped directory table |

## Revisit triggers

Revisit when SSO or SCIM enters scope and the user identifier is no longer the Cognito
subject, which is on ADR 0003's own trigger list. Revisit when memberships gain a level
above the institution, a consortium or a department tree, since the pre-tenant phase would
then need a scope that is neither the user nor the tenant. Revisit when the second EU
region lands and identity provider selection starts routing. Revisit when the
source-scanning test reports a false positive on a legitimate user-scope path. Revisit
when a PgBouncer version makes `DISCARD ALL` cheap enough on the transaction path to
re-examine ADR 0008's table, since this setting carries the same exposure.

Not decided here: whether `InTenant` should reject a call passing neither a user nor a
tenant, and whether the setting needs another name if identity ever becomes composite.