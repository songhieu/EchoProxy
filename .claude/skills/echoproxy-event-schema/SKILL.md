---
name: echoproxy-event-schema
description: Rules for changing event.proto (the schema shared by proxy-gateway, ingest-api, log-consumer, ClickHouse, and every language SDK). Backwards-compatible only. Apply whenever adding a new field to HttpEvent; changing how data maps to ClickHouse; bumping an SDK version; adding an enum value; deprecating an old field; or reviewing PRs that touch api/event.proto or migrations/clickhouse/.
---

# EchoProxy Event Schema Contract

`api/event.proto` is the **public contract** between N producers (proxy-gateway, ingest-api called from Go/PHP/Python/Node SDKs...) and the backend pipeline (Kafka → log-consumer → ClickHouse → stats-api). One bad change can crash an entire fleet of SDKs already deployed at customers.

**Top rule:** the schema only changes in **backwards-compatible** ways relative to old SDKs.

## 1. Change lifecycle

```
You want to change the schema
        ↓
Read this skill
        ↓
Classify: add / deprecate / breaking
        ↓
If BREAKING → STOP, use a new schema_version (see §6)
        ↓
Edit api/event.proto
        ↓
make proto-gen   (regen Go + JSON + bindings for every SDK)
        ↓
Update the ClickHouse migration
        ↓
Update log-consumer mapping
        ↓
Update docs/sdk-spec.md if the public contract changes
        ↓
Bump the reference SDK version (sdk-reference-go) and tag
        ↓
PR must pass: proto lint + buf breaking check
```

## 2. Adding fields — always safe

✅ **ALLOWED:**
- Adding a new field with an unused tag.
- Appending a new enum value at the end.
- Adding a new RPC method.
- Adding a new key to `Map<string,string> attributes` (no proto change needed).

```proto
message HttpEvent {
  // ... existing fields with tags 1-33

  // NEW (tag 34) — backwards compatible
  string region = 34;
}
```

Old SDKs don't know about this field → they skip it during parse → they keep working. Backend treats it as nullable.

## 3. Removing / changing — NOT allowed

❌ **FORBIDDEN:**
- Removing a field that's already shipped.
- Changing the tag number of a field (even if unused — the tag is part of the contract).
- Changing the type of a field (`string` → `int32`, `int32` → `int64`).
- Renaming a field if any SDK serializes it via JSON (JSON uses names).
- Changing the semantic of a field (e.g. `latency_ms` → `latency_us`).
- Reordering enum values.
- Switching from `optional` → `required` (proto3 has no `required`, but avoid changing cardinality).

If you really must remove: **deprecate** first (see §4), keep the field for at least 2 minor versions before fully removing it.

## 4. Deprecating an old field

```proto
message HttpEvent {
  string old_field = 7 [deprecated = true];  // DEPRECATED 2026-05-01, remove after 2026-08-01
  string new_field = 35;                     // replacement
}
```

- Add the `[deprecated = true]` annotation.
- Comment the deprecation date and the planned removal date.
- log-consumer keeps mapping both → ClickHouse during the transition (preferring `new_field`).
- Update `docs/sdk-spec.md` so SDKs migrate.
- After the removal date: confirm `dropped_events_total{reason="unknown_field"}` = 0 → only then remove.

## 5. Changing the ClickHouse table

The proto → ClickHouse mapping lives in `log-consumer/internal/adapter/clickhouse/insert.go`. When adding a proto field:

1. Add the matching column in a new migration: `migrations/clickhouse/00X_add_<field>.sql`.

```sql
ALTER TABLE http_events ADD COLUMN IF NOT EXISTS region LowCardinality(String) DEFAULT '';
```

2. Update the `INSERT` statement in `log-consumer`.
3. Old rows get the default value (`''`, `0`, ...).
4. Materialized view: if the MV needs the new field, create a new MV `mv_minute_metrics_v2` running in parallel; deprecate `_v1` after 30 days.

**DO NOT** run `ALTER TABLE ... DROP COLUMN` in production until you're 100% sure no query still uses it (check `system.query_log`).

## 6. Breaking change — must bump the version

If you genuinely need a breaking change (very rare, justify it in an RFC):

1. Create a new package `echoproxy.v2` in `api/event.proto`:

```proto
package echoproxy.v2;
message HttpEvent { /* new schema */ }
service EventIngest { /* new RPC */ }
```

2. New endpoints: `POST /v2/events:batch` and `echoproxy.v2.EventIngest`.
3. `ingest-api` serves both `v1` and `v2` simultaneously for at least 90 days.
4. log-consumer supports both topics / both paths; map to the same ClickHouse table (or to a new table).
5. All SDKs migrate to v2 → only then retire v1.
6. Kafka topic: create a separate `http_events_v2` so schemas don't mix in the same topic.

## 7. New fields = sane defaults

Every new field added to `HttpEvent` must:
- Have a clear default value (proto3 zero values: `""`, `0`, `false`, empty map).
- Be handled at the backend when the value is zero (don't filter rows because of zero).
- Document the semantic in a proto comment.

```proto
// region: AWS region or cloud provider region of the captured request.
// Optional. Empty string means unknown / not captured by the SDK.
// Added 2026-05.
string region = 34;
```

## 8. SDK versioning

Every SDK declares `sdk_version` (semver):
- **Patch** (`1.0.0` → `1.0.1`): bug fix, no change in events sent.
- **Minor** (`1.0.0` → `1.1.0`): new field added (compatible).
- **Major** (`1.0.0` → `2.0.0`): only when the backend introduces `echoproxy.v2`. Bumped in lockstep.

The `source` field stores `"sdk-laravel"`, `"sdk-python"`, `"proxy"`. The `sdk_version` field stores the SDK version. Both are LowCardinality in ClickHouse → queryable.

## 9. Lint & CI gate

PRs that touch `api/event.proto` must:

```bash
make proto-lint        # buf lint
make proto-breaking    # buf breaking against the main branch
make proto-gen         # regen Go + check no diff
```

`buf breaking` config:
```yaml
# buf.yaml
breaking:
  use:
    - FILE  # forbid removing fields, changing tags, changing types
```

CI failure → no merge. If you really need a breaking change: create a `v2` package (see §6), don't bypass the lint.

## 10. PR review checklist for schema changes

- [ ] Does the new field use an unused tag?
- [ ] Does the tag avoid overlap with a deprecated field that hasn't expired yet?
- [ ] Sane default value at the backend?
- [ ] Proto comment explaining the semantic + the "added on" date?
- [ ] ClickHouse migration adds the matching column?
- [ ] log-consumer mapping updated?
- [ ] `docs/sdk-spec.md` documents the new field?
- [ ] `make proto-breaking` passes?
- [ ] Reference Go SDK has a test that sends the new field?
- [ ] Reference SDK minor version bumped?

## 11. Anti-patterns

- ❌ "Temporarily remove the old field, we'll add it back" — NO. The tag is permanently burned.
- ❌ Editing the proto without regenerating → drift between Go bindings and the proto file.
- ❌ Adding a field but forgetting to update the ClickHouse migration → log-consumer hits `Unknown column`.
- ❌ Removing a field without deprecating first → old SDKs at customers send the field and get rejected.
- ❌ Changing `int32` → `int64` to "support larger numbers" — protobuf wire format differs, parsing breaks.
- ❌ Putting secrets / PII (passwords, full credit cards) in a fixed field. Use the `attributes` map for optional rich context, with SDK-side masking.
- ❌ Adding one field but touching 50 unrelated files in the PR. Split it.

## 12. File pointers

- Proto: `api/event.proto`
- Generated Go: `pkg/event/event.pb.go` (auto-generated, DO NOT hand-edit)
- Producer wrapper: `pkg/event/producer.go`
- ClickHouse migrations: `migrations/clickhouse/`
- log-consumer mapping: `log-consumer/internal/adapter/clickhouse/insert.go`
- SDK contract doc: `docs/sdk-spec.md`
- Reference SDK: `sdk-reference-go/`
- Buf config: `buf.yaml`, `buf.gen.yaml`
