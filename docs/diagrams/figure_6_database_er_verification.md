# Figure 6 — SwiftDrop Database ER Diagram: Verification Register

This is the diagram verification register, not Report Section 3.

## Deliverables

- `figure_6_database_er.drawio`: editable, uncompressed diagrams.net XML; one complete diagram page.
- `figure_6_database_er.svg`: vector export, A3 landscape (420 × 297 mm).
- `figure_6_database_er.png`: 5400 × 3819 pixels; approximately 327 ppi at A3 width.

Use the SVG for report insertion where supported. Print the complete figure on A3 landscape; reducing all 26 tables to A4 makes the text smaller. Key names are approximately 9.6 pt at A3 size. Business columns are abbreviated; the diagram includes every primary key, every actual FK column, and all 21 listed cross-service reference columns.

## Sources and scope

- Official DB v1.1 PDF read from `C:\Users\supes\Downloads\SwiftDrop Database Design Document (v1.1, aligned with API v1.1).pdf` (26 pages).
- PDF SHA256: `63282df518cb3a87adf10280e2b34301b171a6efecf002ecf489db305f6e6bdf`.
- Requested `docs/reference/SwiftDrop-Database-Design-v1.1.pdf` remains absent. This diagram task does not copy or alter the official PDF.
- Migration definitions at Git commit `0106b4a1a6dba0b9f1e37b9836d080b1d2cb1314`.
- All 26 tables are planned in the PDF and have matching migration definitions. This does not assert which migrations are applied to a shared or production database.
- The supplied PDF is design evidence; its operational/retention instructions were not executed.

## Table inventory

| Schema         | Table                          | Primary key shown    | PDF section / page | SQL creation source                               | Status                       |
| -------------- | ------------------------------ | -------------------- | ------------------ | ------------------------------------------------- | ---------------------------- |
| `identity`     | `roles`                        | `id`                 | §3.1, p. 4         | `migrations/identity/000002_tables.up.sql:1`      | VERIFIED in migration source |
| `identity`     | `users`                        | `id`                 | §3.2, p. 5         | `migrations/identity/000002_tables.up.sql:9`      | VERIFIED in migration source |
| `identity`     | `courier_profiles`             | `user_id`            | §3.3, p. 6         | `migrations/identity/000002_tables.up.sql:33`     | VERIFIED in migration source |
| `identity`     | `refresh_tokens`               | `id`                 | §3.4, p. 6         | `migrations/identity/000002_tables.up.sql:44`     | VERIFIED in migration source |
| `identity`     | `oauth_states`                 | `state`              | §3.5, p. 7         | `migrations/identity/000002_tables.up.sql:59`     | VERIFIED in migration source |
| `identity`     | `audit_logs`                   | `id`                 | §3.6, p. 7         | `migrations/identity/000002_tables.up.sql:87`     | VERIFIED in migration source |
| `identity`     | `password_reset_tokens`        | `id`                 | §3.7, p. 8         | `migrations/identity/000002_tables.up.sql:71`     | VERIFIED in migration source |
| `pricing`      | `zones`                        | `id`                 | §4.1, p. 9         | `migrations/pricing/000002_tables.up.sql:1`       | VERIFIED in migration source |
| `pricing`      | `pricing_rules`                | `id`                 | §4.2, p. 9         | `migrations/pricing/000002_tables.up.sql:12`      | VERIFIED in migration source |
| `pricing`      | `weight_bands`                 | `id`                 | §4.3, p. 10        | `migrations/pricing/000002_tables.up.sql:28`      | VERIFIED in migration source |
| `pricing`      | `size_surcharges`              | `id`                 | §4.4, p. 10        | `migrations/pricing/000002_tables.up.sql:43`      | VERIFIED in migration source |
| `pricing`      | `audit_logs`                   | `id`                 | §4.5, p. 11        | `migrations/pricing/000002_tables.up.sql:52`      | VERIFIED in migration source |
| `parcel`       | `parcels`                      | `id`                 | §5.1, p. 11        | `migrations/parcel/000002_tables.up.sql:1`        | VERIFIED in migration source |
| `parcel`       | `parcel_status_history`        | `id`                 | §5.2, p. 13        | `migrations/parcel/000002_tables.up.sql:72`       | VERIFIED in migration source |
| `parcel`       | `parcel_assignments`           | `id`                 | §5.3, p. 14        | `migrations/parcel/000002_tables.up.sql:92`       | VERIFIED in migration source |
| `parcel`       | `delivery_proofs`              | `id`                 | §5.4, p. 15        | `migrations/parcel/000002_tables.up.sql:110`      | VERIFIED in migration source |
| `parcel`       | `outbox_events`                | `id`                 | §5.5, p. 15        | `migrations/parcel/000002_tables.up.sql:128`      | VERIFIED in migration source |
| `merchant`     | `merchants`                    | `id`                 | §6.1, p. 16        | `migrations/merchant/000002_tables.up.sql:1`      | VERIFIED in migration source |
| `merchant`     | `merchant_api_clients`         | `id`                 | §6.2, p. 17        | `migrations/merchant/000002_tables.up.sql:24`     | VERIFIED in migration source |
| `merchant`     | `merchant_webhooks`            | `id`                 | §6.3, p. 18        | `migrations/merchant/000002_tables.up.sql:60`     | VERIFIED in migration source |
| `merchant`     | `merchant_orders`              | `id`                 | §6.4, p. 19        | `migrations/merchant/000002_tables.up.sql:74`     | VERIFIED in migration source |
| `merchant`     | `webhook_delivery_attempts`    | `id`                 | §6.5, p. 20        | `migrations/merchant/000002_tables.up.sql:103`    | VERIFIED in migration source |
| `merchant`     | `merchant_order_status_events` | `id`                 | §6.6, p. 21        | `migrations/merchant/000002_tables.up.sql:126`    | VERIFIED in migration source |
| `merchant`     | `audit_logs`                   | `id`                 | §6.7, p. 22        | `migrations/merchant/000002_tables.up.sql:139`    | VERIFIED in migration source |
| `notification` | `notification_logs`            | `id`                 | §7.1, p. 22        | `migrations/notification/000002_tables.up.sql:1`  | VERIFIED in migration source |
| `notification` | `processed_events`             | `consumer, event_id` | §7.2, p. 23        | `migrations/notification/000002_tables.up.sql:16` | VERIFIED in migration source |

**Counts:** identity 7; pricing 5; parcel 5; merchant 7; notification 2. Total 26. The three `audit_logs` tables are distinct schema-owned tables. Infrastructure migration-version tables are excluded.

## Actual database foreign keys — solid connectors

Arrows point from the referencing column to the referenced primary key. Every connector below is intra-service and database-enforced. No cross-schema FK is invented.

| ID   | Referencing column                               | Referenced column               | Child-to-parent                          |
| ---- | ------------------------------------------------ | ------------------------------- | ---------------------------------------- |
| FK01 | `identity.users.role_id`                         | `identity.roles.id`             | many children per parent                 |
| FK02 | `identity.courier_profiles.user_id`              | `identity.users.id`             | at most one child per parent (unique FK) |
| FK03 | `identity.refresh_tokens.user_id`                | `identity.users.id`             | many children per parent                 |
| FK04 | `identity.password_reset_tokens.user_id`         | `identity.users.id`             | many children per parent                 |
| FK05 | `merchant.merchant_api_clients.merchant_id`      | `merchant.merchants.id`         | many children per parent                 |
| FK06 | `merchant.merchant_webhooks.merchant_id`         | `merchant.merchants.id`         | at most one child per parent (unique FK) |
| FK07 | `merchant.merchant_orders.merchant_id`           | `merchant.merchants.id`         | many children per parent                 |
| FK08 | `merchant.webhook_delivery_attempts.webhook_id`  | `merchant.merchant_webhooks.id` | many children per parent                 |
| FK09 | `merchant.webhook_delivery_attempts.merchant_id` | `merchant.merchants.id`         | many children per parent                 |
| FK10 | `merchant.merchant_order_status_events.order_id` | `merchant.merchant_orders.id`   | many children per parent                 |
| FK11 | `parcel.parcel_status_history.parcel_id`         | `parcel.parcels.id`             | many children per parent                 |
| FK12 | `parcel.parcel_assignments.parcel_id`            | `parcel.parcels.id`             | many children per parent                 |
| FK13 | `parcel.delivery_proofs.parcel_id`               | `parcel.parcels.id`             | many children per parent                 |
| FK14 | `pricing.pricing_rules.origin_zone_id`           | `pricing.zones.id`              | many children per parent                 |
| FK15 | `pricing.pricing_rules.destination_zone_id`      | `pricing.zones.id`              | many children per parent                 |

A child FK row requires its parent. A parent need not yet have any children; the FK alone does not ensure an initial parcel-history row. All FK actions use the SQL default because no `ON DELETE`/`ON UPDATE` action is specified.

## Logical cross-service references — dashed connectors

These arrows are UUID references validated by owning-service application workflows. Their presence does not claim that the relevant API validation or business handlers already exist. IDs R01–R21 appear beside source fields and on the routed dashed connectors.

| ID  | Source column                                   | Logical target          | Design status                           |
| --- | ----------------------------------------------- | ----------------------- | --------------------------------------- |
| R01 | `identity.courier_profiles.home_zone_id`        | `pricing.zones.id`      | DB v1.1 planned reference               |
| R02 | `pricing.pricing_rules.updated_by`              | `identity.users.id`     | DB v1.1 planned reference               |
| R03 | `pricing.audit_logs.actor_user_id`              | `identity.users.id`     | DB v1.1 planned reference               |
| R04 | `parcel.parcels.customer_user_id`               | `identity.users.id`     | DB v1.1 planned reference               |
| R05 | `parcel.parcels.merchant_id`                    | `merchant.merchants.id` | DB v1.1 planned reference               |
| R06 | `parcel.parcels.pickup_zone_id`                 | `pricing.zones.id`      | DB v1.1 planned reference               |
| R07 | `parcel.parcels.dropoff_zone_id`                | `pricing.zones.id`      | DB v1.1 planned reference               |
| R08 | `parcel.parcels.current_courier_id`             | `identity.users.id`     | DB v1.1 planned reference               |
| R09 | `parcel.parcels.created_by_user_id`             | `identity.users.id`     | DB v1.1 planned reference               |
| R10 | `parcel.parcel_status_history.actor_user_id`    | `identity.users.id`     | DB v1.1 planned reference               |
| R11 | `parcel.parcel_assignments.courier_id`          | `identity.users.id`     | DB v1.1 planned reference               |
| R12 | `parcel.parcel_assignments.assigned_by`         | `identity.users.id`     | DB v1.1 planned reference               |
| R13 | `parcel.parcel_assignments.previous_courier_id` | `identity.users.id`     | SQL-only addition: M6 Parcel 000003 (+) |
| R14 | `parcel.delivery_proofs.captured_by`            | `identity.users.id`     | DB v1.1 planned reference               |
| R15 | `merchant.merchants.owner_user_id`              | `identity.users.id`     | DB v1.1 planned reference               |
| R16 | `merchant.merchants.pickup_zone_id`             | `pricing.zones.id`      | DB v1.1 planned reference               |
| R17 | `merchant.merchants.approved_by`                | `identity.users.id`     | DB v1.1 planned reference               |
| R18 | `merchant.merchant_orders.parcel_id`            | `parcel.parcels.id`     | DB v1.1 planned reference               |
| R19 | `merchant.webhook_delivery_attempts.parcel_id`  | `parcel.parcels.id`     | DB v1.1 planned reference               |
| R20 | `merchant.audit_logs.actor_user_id`             | `identity.users.id`     | DB v1.1 planned reference               |
| R21 | `notification.notification_logs.parcel_id`      | `parcel.parcels.id`     | DB v1.1 planned reference               |

## Deliberately unenforced or unspecified associations

- `identity.audit_logs.actor_user_id` → `identity.users.id`: same-schema logical reference; marked LREF, with no FK connector.
- `identity.refresh_tokens.replaced_by` → `identity.refresh_tokens.id`: rotation-chain association; marked LREF, with no FK connector.
- Audit `entity_type` / `entity_id` and outbox `aggregate_type` / `aggregate_id` are polymorphic. No fixed target or FK is invented.
- `event_id` fields are messaging correlation/deduplication identifiers, not guaranteed FKs to `parcel.outbox_events.id`.
- Merchant tracking-number copies and order snapshots are not extra enforced relationships.

## Planned design versus actual SQL overlays

- Every table card belongs to the PDF inventory; VERIFIED means a source definition exists, not a deployed endpoint or shared database inspection.
- Orange `+` rows show M6-only `parcel_assignments.previous_courier_id`, `parcel_assignments.updated_at`, `outbox_events.aggregate_type`, and `outbox_events.aggregate_id` from `migrations/parcel/000003_dispatch_foundations.up.sql`.
- R13 is the only additional logical relationship introduced by that SQL overlay. It is explicitly marked as absent from the planned PDF.
- `migrations/parcel/000004_price_breakdown_total.up.sql` strengthens the documented price-total requirement; it adds no table or FK.
- `migrations/merchant/000006_merchant_constraints.up.sql` adds stricter CHECKs, not tables or relationships.
- PDF-only mode can be obtained by removing the four marked `+` rows and R13; all 26 planned tables and their original relationships remain.
- The existing outbox storage-label versus versioned messaging-contract decision remains outside this diagram. No producer, consumer, migration or contract was changed.

## Validation executed

The file was parsed with Python XML tools, decoded by the native mxGraph codec from the official diagrams.net distribution, and rendered locally in headless Chrome with networking disabled and all requests aborted. Decoded inventory and terminals were checked before exports were generated. Full and Parcel-detail previews were visually inspected.

Public-site editor validation was not executed: automatic approval review rejected loading repository-derived schema XML into the public diagrams.net app because it could expose non-public architecture data. The completed alternative used only a locally downloaded public rendering engine and an offline about:blank browser context. No diagram/schema payload was sent to that app. A separate diagrams.net Desktop UI opening was not available; the native codec/render round trip is the compatibility check performed.

Local public renderer: `mxGraph from the official diagrams.net distribution`, version `32.6.9`.

| Check                              | Result                                                                  |
| ---------------------------------- | ----------------------------------------------------------------------- |
| planned tables and schema counts   | PASS: 26 / 7+5+5+7+2                                                    |
| all actual primary keys            | PASS: including SMALLINT, shared PK/FK, string and composite exceptions |
| displayed column types and fk tags | PASS: all displayed types and FK tags match baseline or forward SQL     |
| actual foreign keys                | PASS: exact match to all 15 migration-declared foreign keys             |
| logical cross service references   | PASS: 20 planned + 1 SQL-only M6 reference; all dashed                  |
| three audit tables                 | PASS                                                                    |
| native mxgraph decode              | PASS: 26 tables, 36 connected relationships, no dangling terminals      |
| routing                            | PASS: orthogonal waypoint segments do not cross table interiors         |
| svg                                | PASS: A3 vector, no HTML foreignObject or external linked images        |
| png                                | PASS: 5400 x 3819 pixels, approximately 327 ppi at A3 width             |
| browser network                    | PASS: schema processed only offline; all requests blocked               |
| migration preservation             | PASS: SHA256 unchanged for every migration file                         |
| branch and head                    | PASS: feature/m5-parcel-creation at 0106b4a                             |

Executed from the repository root:

```powershell
python -B test-output/m5-figure6/build_figure6.py
node test-output/m5-figure6/render_offline.cjs
python -B test-output/m5-figure6/verify_figure6.py
```

Temporary generator, public renderer, previews and machine-readable results are kept under ignored `test-output/m5-figure6/`. They are not added to the repository source. The user-facing editable source, exports and this register live under `docs/diagrams/`.

No migration, application implementation, original specification or Report Section 3 was edited. No commit, push or branch switch was performed.
