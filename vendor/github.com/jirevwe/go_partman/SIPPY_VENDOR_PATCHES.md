# Vendor Patches for go_partman

This file documents patches applied to the vendored copy of
`github.com/jirevwe/go_partman` (v0.4.1). These patches work around
upstream bugs in go_partman's handling of non-multi-tenant tables and
should be removed if the upstream library addresses them.

## 1. Default tenant for non-multi-tenant tables

**File:** `manager.go`, function `initialize`

**Problem:** go_partman was designed around multi-tenant workloads.
Two internal functions assume at least one row exists in
`partman.tenants` for each parent table:

- `CreateFuturePartitions` queries `partman.tenants` and iterates over
  the results to create partitions. For non-multi-tenant tables no
  tenant rows exist, so the loop body never executes and no partitions
  are created.

- `importExistingPartitions` discovers existing child tables and
  inserts management entries into `partman.partitions` with
  `tenant_id = ''` (empty string, not SQL NULL). The
  `validate_tenant_id` trigger on that table checks
  `NEW.tenant_id IS NOT NULL` — empty string passes that check — then
  verifies the tenant exists in `partman.tenants`. Because no tenant
  row with `id = ''` exists, the trigger raises an exception and the
  import fails.

**Fix:** After `CreateParentTable` returns the parent table ID, insert
a default tenant row with an empty-string ID when `TenantIdColumn` is
empty (i.e. the table is not multi-tenant):

```go
if table.TenantIdColumn == "" {
    if _, err = m.db.ExecContext(ctx, insertTenantSQL, "", id); err != nil {
        return fmt.Errorf("failed to register default tenant for table %s: %w", table.Name, err)
    }
}
```

**Why the empty string is safe:** Every branch in go_partman that
changes behavior based on tenant ID uses `len(x) > 0` guards:

| Code path | Guard | Result with `""` |
|---|---|---|
| `generatePartitionName` | `len(tc.TenantId) > 0` | Uses `table_YYYYMMDD` (no tenant prefix) |
| `generateRangePartitionSQL` | `len(tc.TenantId) > 0` | Uses non-tenant DDL template |
| `DropOldPartitions` | `len(table.TenantId) > 0` | Pattern is `table_%` (no tenant infix) |
| `checkTableColumnsExist` | `len(TenantIdColumn) > 0 && len(tenantId) > 0` | Skips tenant column check |
| `buildTableName` | `tenantId != "" && len(tenantId) > 0` | Returns `schema.table` |
| `importExistingPartitions` | `len(p.TenantFrom.String) > 0` | Skips duplicate tenant registration |

The empty-string tenant satisfies the `partman.tenants` foreign key
and `validate_tenant_id` trigger without affecting partition naming,
DDL generation, or drop patterns. `insertTenantSQL` uses
`ON CONFLICT DO NOTHING`, so repeated calls (e.g. server restarts)
are idempotent. The tenant row cascades on parent table deletion.

## 2. Embedded web assets placeholder

**Directory:** `web/dist/index.html`

go_partman's `ui.go` contains `//go:embed web/dist` but the published
module does not include built web assets. Go's vendoring copies only
`.go` files, so `go mod vendor` fails with
`pattern web/dist: no matching files found`. A minimal placeholder
`index.html` is provided so the embed directive resolves. The UI is
not used by Sippy.
