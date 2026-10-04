# Runbook: retire Glossa v0.3

RFC 0006 §7.4. v0.3 is `apps/api`, `apps/admin`, `packages/*` and the `glossa` namespace serving `glossa.felixgeelhaar.de`. This runbook carries each v0.3 project to the platform, proves it renders the same, switches its product to the edge, watches it, and only then retires v0.3. Run **steps 1–9 once per former v0.3 project** and **step 10 once, after the last one**.

Every command below exists as written (`glossa` is `platform/cmd/glossa`; flags are those in `platform/internal/cli`). Until step 7, v0.3 still serves the product, so every earlier rollback is "stop and leave the product on v0.3".

## Who may do what

| Action | Who | Why |
|---|---|---|
| Steps 1, 2 (freeze, backup, restore) | Operator with `kubectl` on the `glossa` namespace and a scratch Postgres 16 | Touches the v0.3 deployment. |
| Steps 3, 4 without `--history`, 5, 6 `--verify` | Anyone holding a token with the `write` scope (`--invite` needs `admin`; `release keys` and `publish` need `publish`) | |
| Step 4 `--history` | **Owner** of the platform organisation, signed in with `glossa login --device` | Needs `audit.import`; no API token scope reaches it. |
| `promote` / `publish` into an environment that requires approvals | The people the environment's policy names, with `glossa approve` | The command exits 5 (held) until they decide (RFC 0006 §5.1, §15 q6). |
| Step 7 | The product's maintainer | Product repository work. |
| Steps 9 and 10 | **Owner only**: scaling v0.3 to zero, namespace and DNS removal, npm deprecation, merging the deletion PR | Destructive or publishing (RFC 0006 §7.4). An agent prepares, never performs them. |

## Evidence to keep

One folder per project (`evidence/<project>/`, outside the repository; never commit it): the v0.3 dump and its SHA-256; the output of every command below saved with `--json`; the `--verify` report; the release IDs; the observation notes; the date and name of whoever ran each step. The dump is the record of last resort (RFC 0006 §7.2) and is archived immutably until one year after the last project.

## Before you start

- The platform is deployed (RFC 0006 §1.3) and the project exists on it. The importer is idempotent: a re-run never undoes a review made in between.
- `glossa` is installed. `node` 22 or later is on the machine that runs `--verify`, and `@felixgeelhaar/glossa-format` and `@glossa/runtime` are built (`make system-m5-deps` in a Glossa checkout, or installed under `node_modules/`).
- Shell variables used below: `V0_TENANT` (v0.3 tenant slug), `V0_PROJECT` (v0.3 project slug), `RESTORE_DB` (a new database name, e.g. `glossa_v0_restore_<project>`).

## 1. Freeze writes on v0.3

v0.3 has no read-only mode. Take away every way to write and keep reads on, because the product reads from v0.3 until step 7.

1. Stop the editing UI: `kubectl -n glossa scale deploy/admin --replicas=0`.
2. In v0.3, list the project's API keys and revoke every `write` key; `read` keys stay. Record the labels, not the keys.
3. Tell the project's translators the freeze has started. Anything written to v0.3 afterwards is not carried over.

Rollback: `kubectl -n glossa scale deploy/admin --replicas=1` and re-issue the write keys. Nothing has left v0.3.

## 2. Back up and restore, with the restore marker

```sh
# A fresh dump, taken after the freeze (the daily CronJob also writes one).
kubectl -n glossa exec postgres-0 -- pg_dump -U glossa glossa --no-owner --no-privileges --clean --if-exists | gzip > glossa-v0-YYYYMMDD.sql.gz
shasum -a 256 glossa-v0-YYYYMMDD.sql.gz                # keep this

# Restore into a NEW database on a scratch Postgres 16, never v0.3's own server.
PGHOST=localhost PGUSER=postgres platform/scripts/v0-restore.sh glossa-v0-YYYYMMDD.sql.gz "$RESTORE_DB"
```

`v0-restore.sh` fails if the database exists, restores in one transaction, writes the restore marker and makes the database read-only by default. `glossa import --from v0 --v0-db` refuses a database without it (`v0_not_a_restore`, exit 2), so production v0.3 can never be the source. Archive the dump to immutable object storage now.

Rollback: `dropdb "$RESTORE_DB"` and run the script again. v0.3 is untouched.

## 3. Dry run

```sh
PGPASSWORD=… glossa import --from v0 --v0-db "postgres://postgres@localhost/$RESTORE_DB" \
  --v0-tenant "$V0_TENANT" --v0-project "$V0_PROJECT" --dry-run
```

Read the report: `restore` (database, dump name, SHA-256: compare with step 2), locales added, per-status counts, `invitations` (`planned`; `held` for a translator with no locales), `audit_entries`, `not_carried`. `--v0-tenant` is needed only when two tenants share the slug. Stop if the counts disagree with what v0.3 shows.

Rollback: none; a dry run writes nothing.

## 4. Import the data, then the history

```sh
glossa login                                           # a token that may write
glossa import --from v0 --v0-db "postgres://postgres@localhost/$RESTORE_DB" \
  --v0-tenant "$V0_TENANT" --v0-project "$V0_PROJECT" [--invite] [--locales de,en]
```

`--invite` sends the planned invitations (needs the `admin` scope; they can only be accepted once the platform sends mail, RFC 0006 §1.3). Exit 4 means some items failed: read them, fix, re-run (idempotent).

Then, **as an owner**:

```sh
glossa login --device --client-name "v0 retirement"    # open the link in Studio, check the code and name, approve
glossa whoami                                          # must name the owner and say it is a device session
glossa import --from v0 --v0-db "postgres://postgres@localhost/$RESTORE_DB" \
  --v0-tenant "$V0_TENANT" --v0-project "$V0_PROJECT" --history
glossa logout                                          # ends the device session on the server too
```

`--history` appends v0.3's `audit_log` to the organisation's audit trail as imported entries carrying SHA-256 digests of the text, never the text. A token is refused (`forbidden`, exit 3). `--history` and `--dry-run` exclude each other. The report's `history` gives `sent`, `recorded`, `existing`; a re-run records nothing twice. Importing a second project of the same organisation re-sends the rows whose translation is gone and records them once.

Rollback: translations arrive as revisions with provenance `import` and reach no product (the product reads v0.3), so a wrong import is fixed in the restore or the data and re-run. Audit entries are append-only and cannot be removed; that is why `--history` runs last, after the dry run was read and the data import checked.

## 5. Publish to staging

```sh
glossa release publish --environment staging --dry-run           # lists every problem and exits 1 if not releasable
glossa release publish --environment staging --note "v0.3 import"
glossa release keys create verify --environments staging         # a delivery key for the comparison
```

`staging` ships approved text only. Text that arrived `needs_review` (the importer downgrades an approved value where the project requires review and a token cannot approve) is not shipped, falls back, and shows as a mismatch in step 6: review and approve it first.

Rollback: `glossa release rollback --environment staging`. No product reads staging.

## 6. Prove it renders the same: `--verify`

```sh
export GLOSSA_DELIVERY_KEY=dk_…                        # the key from step 5
glossa import --from v0 --v0-db "postgres://postgres@localhost/$RESTORE_DB" \
  --v0-tenant "$V0_TENANT" --v0-project "$V0_PROJECT" \
  --verify --edge https://cdn.glossa.klarlabs.de --environment staging --json > evidence/verify.json
```

`--verify` imports and writes nothing. It renders every key in every locale twice, with v0.3's own formatter and with `@glossa/runtime` over the release the edge serves, using the same generated arguments (each plural at 0, 1, 2, 5, 21 and its exact keys; each select key plus a catch-all value). `--node`, `--format-module` and `--runtime-module` point it at other builds.

Reading `glossa.cli.import-verify/v1`:

- `summary` counts `renderings`, `match`, `known_defect`, `mismatch`. Exit 0 means no mismatch; exit 1 (`renderings_differ`) means at least one.
- **`mismatch` must be 0.** Each row lists key, locale, arguments and both outputs. Usual causes: a key the environment did not ship, a number-format difference, an error on either side (`v0_error`, `runtime_error`).
- **`known_defect` is accepted, and only as `defect: v0_bare_apostrophe`.** v0.3 drops a bare apostrophe (`Geht's gut` renders `Gehts gut`); the platform and ICU are right. A row is classed so only when v0.3's own formatter, given the text requoted, renders exactly the runtime's output (`v0_requoted` is that evidence). Skim the list: each row must differ by apostrophes and nothing else. A changed word beside an apostrophe is a mismatch.
- `runtime_errors`: the runtime could not load the release. Fix the delivery key, environment or edge URL and re-run; ignore the rest of that report.
- `module_not_found`, `module_not_built`, `node_not_found` (exit 2): the comparison did not run. That is not a pass.

The gate is **zero mismatches, every known defect listed and accepted, the report saved.** Then promote and give the product its key:

```sh
glossa release promote v<N> --to production            # <N> as printed by publish in step 5
glossa release keys create <product>                   # the key the product's runtime ships (reads production)
glossa release environments
```

If production requires approvals, `promote` exits 5 and waits for `glossa approve <request>`.

Rollback: fix the data or release and re-run; `glossa release rollback --environment production`. No product has changed yet.

## 7. Switch the product's runtime to the edge

In the product repository (deferred until the Glossa rewrite is finished; owner decision): replace the v0.3 SDK with `createRuntime({ edge: "https://cdn.glossa.klarlabs.de", deliveryKey })` from `@glossa/runtime` (or the Go or Dart runtime), keep the product's fallbacks, deploy to staging, then production. For a gradual start use `glossa release rollout start …` (CLI README, *Staged rollouts*).

Rollback: redeploy the previous product build, which reads v0.3. Keep v0.3's `api` deployment and read keys until step 10 so this stays one deploy away.

## 8. Observe for 14 days, v0.3 still running

Daily: `glossa release environments` (what production serves), the product's and the edge's error rates against the baseline, `glossa status` for missing translations. Re-run step 6's `--verify` (against `--environment production`) after any publish that changes imported text. On a regression roll the product back (step 7) or the release (`glossa release rollback --environment production`), note it in the evidence, and restart the 14 days.

## 9. Retire the project on v0.3

After 14 clean days, and only when **no product reads this project from v0.3** (check the `api` deployment's access log for the slug): leave the step 1 freeze in place (admin at 0, no write keys). Delete nothing yet. Revoke the verify key: `glossa release keys revoke verify`.

Rollback: point the product back at v0.3 (step 7's rollback). Nothing is lost.

## 10. After the last project (owner only)

1. Archive a final v0.3 dump (step 2's command) for one year, immutable; keep its SHA-256 with the evidence.
2. `kubectl -n glossa scale deploy/api deploy/admin --replicas=0` and leave it for 30 days.
3. Remove the IngressRoute and the DNS record for `glossa.felixgeelhaar.de`; delete the `glossa` namespace.
4. Deprecate the npm packages with a pointer to the new ones (`npm deprecate`).
5. Merge the deletion PR (branch `m5/retire-v0`; it must not merge before steps 1–9 have run for every former v0.3 project). The PR also removes v0.3's `.rollops/*.yaml` (namespace `glossa`), so merge it only after step 3. Afterwards the exit test's v0.3 half (`apps/api` build, `packages/format`) is gone: see the PR description for what replaces it.
6. Export the organisation's audit trail as an owner and verify it offline: `glossa audit verify <export> --public-key glossa-audit-keys.json --json` (the key document is served at `/.well-known/glossa-audit-keys.json`).

Rollback: before 3, `kubectl -n glossa scale deploy/api deploy/admin --replicas=1`. After 3, rebuild from the archived dump with `platform/scripts/v0-restore.sh` and the git history of the deletion PR; that is why the dump is kept for a year.
