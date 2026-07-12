# Database Design

## Overview

`ctx` stores its internal state in a SQLite database.

The database is considered an implementation detail of `ctx`. External tools should use the JSON API whenever possible instead of directly depending on the database schema.

## Goals

The database design aims to satisfy the following requirements:

- Preserve user data across updates.
- Support automatic schema migration.
- Keep the schema simple and maintainable.
- Allow future schema evolution without breaking existing installations.

## Migration Policy

Every released version of `ctx` must be able to migrate databases created by previous released versions.

Schema migrations are considered part of the public compatibility guarantee.

Development-only schemas that have never been released do not require migration support.

## Schema Version

The database schema version is managed independently from the application version.

The application checks the current schema version when opening the database and automatically applies any required migrations before normal operation.

## Implementation Policy

The `ctx` database layer follows these implementation rules.

- Use `golang-migrate` for schema migrations.
- Embed the latest schema snapshot and migration files into the binary using `go:embed`.
- Store schema version information using the `schema_migrations` table managed by `golang-migrate`.
- Automatically apply pending migrations when opening the database.
- Execute each migration inside a transaction. On failure, roll back the transaction and abort normal startup.
- Create new databases directly from `internal/ctx/schema.sql` without replaying historical migrations.
- Upgrade existing databases by applying every pending migration in order.
- Treat a database without `schema_migrations` as a legacy released database only after validating that it matches a known released schema.
- Downgrade is not part of normal operation. Keep `.down.sql` files for development and verification purposes.
- Keep a database fixture for every released version of ctx.
- Fixtures must be created by the actual released ctx binary rather than reconstructed from the current source tree.
- CI must verify that every released fixture can be migrated to the latest schema.

## Test Fixtures

A database fixture is kept for every released version of ctx.

Fixtures are immutable test assets and must not be modified after they are added.

Tests must migrate a temporary copy of a fixture rather than modifying the fixture itself.

Each fixture should record the release tag or commit that was used to generate it.

## Latest Schema Snapshot

`internal/ctx/schema.sql` is the latest complete schema used only for creating new databases.

When a new database is created, `ctx` applies `schema.sql` directly and then records the latest migration version in `schema_migrations`.

`schema.sql` is not a migration history. It is a snapshot of the final schema for the current source tree.

## Migration Files

Migration files are applied in ascending order.

Standard naming:

```text
<sequence>_<ctx-version>.up.sql
<sequence>_<ctx-version>.down.sql
```

Example:

```text
000001_1.0.0.up.sql
000001_1.0.0.down.sql

000002_1.1.0.up.sql
000002_1.1.0.down.sql
```

When additional clarity is helpful, a descriptive suffix may be added.

```text
<sequence>_<ctx-version>_<description>.up.sql
<sequence>_<ctx-version>_<description>.down.sql
```

Example:

```text
000003_1.2.0_rework_workspaces.up.sql
000003_1.2.0_rework_workspaces.down.sql
```

## Migration Rules

- One `ctx` release should normally contain a single schema migration.
- If multiple schema changes are required for a release, combine them into one migration whenever practical.
- Released migration files are immutable. Do not modify or delete them after release.
- Fixes to released migrations must be implemented as a new migration.

## New Database Creation

A newly created database should be initialized directly from the latest schema.

It must not execute every historical migration from the beginning.

After applying the latest schema snapshot, the database must be marked as being at the latest migration version.

## Existing Database Upgrade

Existing databases must be upgraded by applying every migration in order until the latest schema version is reached.

Skipping intermediate migrations is not supported.

If an existing database does not have `schema_migrations`, `ctx` must not blindly assume a version. It must first validate that the database matches a known released schema.

For a `ctx v1.0.0` baseline, validation must check at least:

- all required tables exist
- required columns exist
- column types and major constraints match
- required indexes exist
- `PRAGMA integrity_check` returns `ok`

Only after successful validation may `ctx` baseline the database by writing version `1` to `schema_migrations`.

If validation fails, startup must fail with an explicit error and normal operation must not continue.

## Compatibility Guarantee

`ctx` guarantees that databases created by any officially released version can be upgraded to newer released versions.

Maintaining this compatibility is a core design requirement.
