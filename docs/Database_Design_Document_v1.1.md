# SwiftDrop Database Design Document — Version 1.1 (aligned with API v1.1)

> Converted from the supplied PDF. Text and table alignment have been preserved where possible; review complex tables and diagrams against the original PDF.


<!-- PDF page 1 -->

Oct 21, 2026        ·



SwiftDrop Parcel Delivery and Courier Management System. Version 1.1, October 2026.



Version 1.0 did not match the API v1.1 in 16 places. This version fixes each one so every

endpoint, error code and field in the API has a column or constraint behind it.





## What changed from version 1.0



  #           Change                                               Why (API reference)         Table



  1           Added password_reset_tokens                          Forgot and reset            Identity

                                                                   password flow (2.16,

                                                                   2.17)



  2            key_prefix widened from 12 to 20                    Key prefix                  merchant_api_clients

              characters and made non-unique; new                   sdk_live_ab12 is 13

              unique lookup_id                                     characters, and only 4

                                                                   random characters

                                                                   would collide often (8.9,

                                                                   8.10)



  3           Added request_hash                                   Same                        merchant_orders

                                                                   externalOrderId with

                                                                   a different payload

                                                                   must return 409

                                                                   DUPLICATE_ORDER

                                                                   (8.16)



  4           Added tracking_number                                Delivery log is filtered    webhook_delivery_attempts

                                                                   by trackingNumber

                                                                   (8.15)



  5           Added unique (merchant_id)                           API exposes one             merchant_webhooks

                                                                   webhook per merchant

                                                                   (8.12, 8.13)



  6           Added idempotency_key                                Idempotency-Key             parcels

                                                                   header on POST

                                                                   /parcels (4.1)



  7           Added failed_at and                                  Courier list shows          parcels

               delivered_to_name                                   parcels finished on a

                                                                   date (6.1); DELIVERED

                                                                   requires

                                                                    recipientName (6.3)


<!-- PDF page 2 -->

#           Change                                               Why (API reference)         Table



  8           Added suspension_reason , made                       Suspend takes a reason      merchants

              pickup_zone_id NOT NULL                              (8.8); registration

                                                                   requires a pickup zone

                                                                   (8.1)



  9           Added last_status_at                                 Merchant sync               merchant_orders

                                                                   consumer must ignore

                                                                   out-of-order events

                                                                   (11.1)



  10          Added last_error ; tracking number                   Outbox diagnostics;         outbox_events , parcels

              format documented                                    examples

                                                                   SDK7M2Q9XP4 and

                                                                   SDN4T8W2LC6 (4.1,

                                                                   8.16)



  11           audit_logs now also owned by                        Zone, pricing and           Ownership

              Pricing and Merchant                                 merchant actions are

                                                                   audited (3.3 to 3.12, 8.6

                                                                   to 8.8)



  12          Removed unused                                       No API or workflow          Pricing, Merchant

               size_surcharges.max_length_cm and                   uses them

               merchant_api_clients.expires_at



  13          Extra indexes for dispatch, reports and              5.1, 5.3, 7.1, 7.2          parcels , history

              workload queries



  14          Unique constraints on webhook                        Retry safety (10.3); one    Merchant, parcels

              attempts and merchant owner; CHECK                   merchant per owner

              that exactly one parcel owner is set



  15          Added snapshot columns (recipient,                   Order list and detail       merchant_orders

              drop-off city, weight, size, description,            return them without

              fee)                                                 calling Parcel

                                                                   Operations per row

                                                                   (8.17, 8.18)



  16          Added                                                Order timeline (8.18)       Merchant

               merchant_order_status_events                        and idempotent status

                                                                   sync (11.1)









## 1. Overview

SwiftDrop uses one PostgreSQL 16 server. Each microservice owns its own tables and is

the only service that writes to them. Other services read that data only through REST

APIs or RabbitMQ events. There are no cross-service foreign keys. Where a table stores

an ID that belongs to another service, it is a plain UUID column (marked "ref" below)

validated through the owning service's API.


<!-- PDF page 3 -->

Suggested schema names: identity , parcel , pricing , merchant , notification .





1.1 Conventions



  Convention                   Rule



  Primary keys                 UUID (v4), generated by the application, column

                               name id



  Timestamps                    TIMESTAMPTZ , stored in UTC. Most tables have

                                created_at , and mutable tables also have

                                updated_at



  Money                         NUMERIC(12,2) in LKR unless a currency column

                               says otherwise



  Enumerations                 Stored as VARCHAR with a CHECK constraint (easy

                               to migrate with golang-migrate)



  Naming                        snake_case , plural table names



  Soft delete                  Not used. Use is_active or a status column



  Migrations                   Versioned SQL files in migrations/<service>/ ,

                               each with an up and a down file



  API mapping                  Database columns are snake_case ; the API

                               returns the same values in camelCase







1.2 Enumerations



  Enum                                   Values



  role name                              CUSTOMER, MERCHANT, COURIER, DISPATCHER,

                                         ADMIN



  auth_provider                          LOCAL, GOOGLE



  parcel status                          CREATED, PICKED_UP, IN_TRANSIT,

                                         OUT_FOR_DELIVERY, DELIVERED, FAILED



  size_category                          SMALL, MEDIUM, LARGE, OVERSIZED



  merchant status                        PENDING, APPROVED, REJECTED, SUSPENDED



  api client status                      ACTIVE, REVOKED



  notification channel                   EMAIL, SMS


<!-- PDF page 4 -->

Enum                                   Values



  notification status                    PENDING, SENT, FAILED



  outbox status                          PENDING, PUBLISHED, FAILED









## 2. Service Ownership Summary



  Service                                  Tables



  Identity and User                         roles , users , courier_profiles ,

                                            refresh_tokens , oauth_states , audit_logs ,

                                            password_reset_tokens (new)



  Pricing and Zones                         zones , pricing_rules , weight_bands ,

                                            size_surcharges , audit_logs (new)



  Parcel Operations                         parcels , parcel_status_history ,

                                            parcel_assignments , delivery_proofs ,

                                            outbox_events



  Merchant Integration                      merchants , merchant_api_clients ,

                                            merchant_webhooks , merchant_orders ,

                                            webhook_delivery_attempts ,

                                           merchant_order_status_events (new),

                                           audit_logs (new)



  Notification Worker                       notification_logs , processed_events



  Webhook Worker                           No tables. It records attempts through the

                                           Merchant Integration internal API (API v1.1, section

                                           13)



  Realtime Service                         No tables. It is stateless and the API is the source

                                           of truth









## 3. Identity and User Service Tables



3.1 roles



  Column                  Type                            Constraints     Description



  id                      SMALLINT                        PK              Role identifier


<!-- PDF page 5 -->

Column                  Type                            Constraints            Description



  name                    VARCHAR(30)                     NOT NULL,              CUSTOMER, MERCHANT, COURIER,

                                                          UNIQUE                 DISPATCHER, ADMIN



  description             VARCHAR(255)                    NULL                   Human-readable description







3.2 users



  Column                              Type                         Constraints          Description



  id                                  UUID                         PK                   User identifier



  email                               VARCHAR(255)                 NOT NULL,            Login email

                                                                   UNIQUE (lower

                                                                   case)



  password_hash                       VARCHAR(100)                 NULL                 bcrypt hash. NULL for Google-

                                                                                        only accounts



  full_name                           VARCHAR(150)                 NOT NULL             Display name (2 to 150

                                                                                        characters)



  phone                               VARCHAR(20)                  NULL                 Contact number in E.164 form



  role_id                             SMALLINT                     NOT NULL, FK         User role

                                                                   roles.id



  auth_provider                       VARCHAR(10)                  NOT NULL,            LOCAL or GOOGLE

                                                                   DEFAULT 'LOCAL'



  google_sub                          VARCHAR(100)                 NULL, UNIQUE         Google account subject

                                                                                        identifier



  email_verified                      BOOLEAN                      NOT NULL,            Set true for Google sign-ups.

                                                                   DEFAULT false        No email-verification flow

                                                                                        exists in API v1.1



  is_active                           BOOLEAN                      NOT NULL,            Disabled users cannot log in

                                                                   DEFAULT true         (403 ACCOUNT_DISABLED )



  failed_login_count                  INT                          NOT NULL,            Drives lockout. Reset to 0 on

                                                                   DEFAULT 0            success



  locked_until                        TIMESTAMPTZ                  NULL                 Temporary lockout end (423

                                                                                         ACCOUNT_LOCKED )



  last_login_at                       TIMESTAMPTZ                  NULL                 Last successful login

                                                                                        (returned by 2.12)


<!-- PDF page 6 -->

Column                              Type                            Constraints          Description



  created_at                          TIMESTAMPTZ                     NOT NULL             Creation time



  updated_at                          TIMESTAMPTZ                     NOT NULL             Last update time





Indexes: unique on lower(email) , unique on google_sub , index on role_id , index on

 (role_id, is_active) for the admin user list filters.





3.3 courier_profiles



  Column                          Type                             Constraints           Description



  user_id                         UUID                             PK, FK users.id       The courier user



  vehicle_type                    VARCHAR(30)                      NULL                  BIKE, VAN, TRUCK and so on



  vehicle_number                  VARCHAR(20)                      NULL                  Registration plate



  home_zone_id                    UUID                             NULL, ref zones       Preferred operating zone

                                                                                         (Pricing service)



  is_available                    BOOLEAN                          NOT NULL,             Can receive assignments

                                                                   DEFAULT true



  created_at                      TIMESTAMPTZ                      NOT NULL              Creation time



  updated_at                      TIMESTAMPTZ                      NOT NULL              Last update time







3.4 refresh_tokens



  Column                    Type                            Constraints              Description



  id                        UUID                            PK                       Token record ID



  user_id                   UUID                            NOT NULL, FK             Owner

                                                            users.id



  token_hash                CHAR(64)                        NOT NULL,                SHA-256 of the refresh token. The

                                                            UNIQUE                   raw token is never stored



  expires_at                TIMESTAMPTZ                     NOT NULL                 Expiry (7 days after issue)



  revoked_at                TIMESTAMPTZ                     NULL                     Set on logout, rotation, password

                                                                                     change, password reset or user

                                                                                     deactivation


<!-- PDF page 7 -->

Column                    Type                            Constraints             Description



  replaced_by               UUID                            NULL                    Next token in a rotation chain



  user_agent                VARCHAR(255)                    NULL                    Client description



  ip_address                INET                            NULL                    Client IP



  created_at                TIMESTAMPTZ                     NOT NULL                Issue time





Indexes: user_id , expires_at .





3.5 oauth_states



  Column                      Type                           Constraints   Description



  state                       VARCHAR(64)                    PK            Random state value sent to Google



  code_verifier               VARCHAR(128)                   NULL          PKCE verifier if PKCE is used



  redirect_to                 VARCHAR(255)                   NULL          Relative frontend path to return to



  browser_binding_hash        CHAR(64)                       NULL          SHA-256 of the browser-binding cookie value (internal security column; NULL on pre-000003 rows)



  expires_at                  TIMESTAMPTZ                    NOT NULL      Short expiry (about 10 minutes)



  created_at                  TIMESTAMPTZ                    NOT NULL      Creation time







3.6 audit_logs

Used for administrator actions. The same structure is kept in the Identity, Pricing and

Merchant schemas, each service writing only its own rows.





  Column                      Type                          Constraints    Description



  id                          UUID                          PK             Log ID



  actor_user_id               UUID                          NOT NULL       Who did it (ref users)



  action                      VARCHAR(60)                   NOT NULL       For example USER_DEACTIVATED



  entity_type                 VARCHAR(40)                   NOT NULL       For example user, merchant, pricing_rule



  entity_id                   UUID                          NULL           Affected record



  details                     JSONB                         NULL           Before and after values. Never store

                                                                           secrets, tokens or passwords



  created_at                  TIMESTAMPTZ                   NOT NULL       Time of action


<!-- PDF page 8 -->

Actions the API marks as audited:





  Service               Entity types                                              Endpoints



  Identity              user                                                      2.11, 2.13, 2.14



  Pricing               zone, pricing_rule, weight_band, size_surcharge           3.3, 3.6, 3.7, 3.9, 3.12



  Merchant              merchant                                                  8.6, 8.7, 8.8





Index: (entity_type, entity_id) and (created_at) .





3.7 password_reset_tokens (new)

Supports POST /auth/forgot-password (2.16) and POST /auth/reset-password (2.17).

Only a hash of the token is stored. The raw token exists only in the emailed link.





  Column                      Type                         Constraints    Description



  id                          UUID                         PK             Token record ID



  user_id                     UUID                         NOT NULL, FK   Account being reset

                                                           users.id



  token_hash                  CHAR(64)                     NOT NULL,      SHA-256 of the high-entropy random

                                                           UNIQUE         token



  expires_at                  TIMESTAMPTZ                  NOT NULL       Short expiry. Recommended 30

                                                                          minutes after creation



  used_at                     TIMESTAMPTZ                  NULL           Set when the token is consumed. A

                                                                          token is valid only while used_at is

                                                                          NULL and expires_at is in the

                                                                          future



  requested_ip                INET                         NULL           IP that requested the reset, for

                                                                          abuse review



  created_at                  TIMESTAMPTZ                  NOT NULL       Creation time





Indexes: user_id , expires_at .



Rules:



     Issuing a new token for a user marks that user's earlier unused tokens as used, so only

     the newest link works.


<!-- PDF page 9 -->

Consuming a token runs in one transaction: update users.password_hash , set

       used_at , and set revoked_at on every active refresh_tokens row of that user.

     Invalid, expired and used tokens are indistinguishable to the caller (400

       INVALID_OR_EXPIRED_RESET_TOKEN ).

     Never write the raw token to logs, audit_logs or error messages.







## 4. Pricing and Zones Service Tables



4.1 zones



  Column                   Type                           Constraints            Description



  id                       UUID                           PK                     Zone ID



  code                     VARCHAR(20)                    NOT NULL,              Short code, for example CMB, KDY

                                                          UNIQUE                 (409 ZONE_CODE_EXISTS )



  name                     VARCHAR(100)                   NOT NULL               Zone name



  districts                TEXT[]                         NULL                   Districts or cities that map to this

                                                                                 zone



  is_active                BOOLEAN                        NOT NULL,              Inactive zones are not selectable

                                                          DEFAULT true



  created_at               TIMESTAMPTZ                    NOT NULL               Creation time



  updated_at               TIMESTAMPTZ                    NOT NULL               Last update time







4.2 pricing_rules



  Column                                 Type                      Constraints                 Description



  id                                     UUID                      PK                          Rule ID



  origin_zone_id                         UUID                      NOT NULL, FK zones.id       Origin zone



  destination_zone_id                    UUID                      NOT NULL, FK zones.id       Destination zone



  base_fee                               NUMERIC(12,2)             NOT NULL, CHECK >= 0        Base fee for the zone

                                                                                               pair



  currency                               CHAR(3)                   NOT NULL, DEFAULT           Currency

                                                                   'LKR'


<!-- PDF page 10 -->

Column                                 Type                           Constraints            Description



  is_active                              BOOLEAN                        NOT NULL, DEFAULT      Rule enabled

                                                                        true



  updated_by                             UUID                           NULL                   Admin user (ref users)



  created_at                             TIMESTAMPTZ                    NOT NULL               Creation time



  updated_at                             TIMESTAMPTZ                    NOT NULL               Last update time





Constraint: partial UNIQUE (origin_zone_id, destination_zone_id) WHERE is_active

(409 RULE_EXISTS ). A missing active rule gives 422 ROUTE_NOT_SERVICEABLE . Rule

changes never alter parcels.price_amount already stored.





4.3 weight_bands



  Column                         Type                              Constraints                     Description



  id                             UUID                              PK                              Band ID



  min_weight_kg                  NUMERIC(6,2)                      NOT NULL                        Lower bound

                                                                                                   (exclusive)



  max_weight_kg                  NUMERIC(6,2)                      NOT NULL, CHECK >               Upper bound

                                                                   min_weight_kg                   (inclusive)



  charge                         NUMERIC(12,2)                     NOT NULL, CHECK >= 0            Charge for this band



  is_active                      BOOLEAN                           NOT NULL, DEFAULT true          Band enabled



  created_at                     TIMESTAMPTZ                       NOT NULL                        Creation time



  updated_at                     TIMESTAMPTZ                       NOT NULL                        Last update time





Active bands must not overlap (409 BAND_OVERLAP ). Enforce this in the service layer and

with an exclusion constraint on numrange(min_weight_kg, max_weight_kg, '(]')

where is_active . A weight outside every active band gives 422 WEIGHT_NOT_PRICED .





4.4 size_surcharges



  Column                       Type                           Constraints              Description



  id                           UUID                           PK                       Record ID



  size_category                VARCHAR(12)                    NOT NULL, UNIQUE         SMALL, MEDIUM, LARGE,

                                                                                       OVERSIZED


<!-- PDF page 11 -->

Column                       Type                           Constraints            Description



  surcharge                    NUMERIC(12,2)                  NOT NULL, CHECK        Extra charge

                                                              >= 0



  updated_at                   TIMESTAMPTZ                    NOT NULL               Last update time





The unused max_length_cm column from v1.0 is removed. Add it back only if a dimension

limit is introduced in the API.





4.5 audit_logs (Pricing schema)

Same structure as 3.6. Written for zone, pricing rule, weight band and size surcharge

changes.





## 5. Parcel Operations Service Tables



5.1 parcels



  Column                                Type                       Constraints          Description



  id                                    UUID                       PK                   Internal parcel ID



  tracking_number                       VARCHAR(20)                NOT NULL,            Public number: SDK plus 8

                                                                   UNIQUE               random characters for

                                                                                        customer parcels, SDN plus

                                                                                        8 for merchant parcels (for

                                                                                        example SDK7M2Q9XP4,

                                                                                        SDN4T8W2LC6). High

                                                                                        entropy, no ambiguous

                                                                                        characters



  customer_user_id                      UUID                       NULL, ref users      Owner when created by a

                                                                                        customer



  merchant_id                           UUID                       NULL, ref            Owner when created by a

                                                                   merchants            merchant



  external_order_id                     VARCHAR(100)               NULL                 Merchant's own order ID



  idempotency_key                       VARCHAR(100)               NULL                 Idempotency-Key header

                                                                                        value (new). Replay returns

                                                                                        the original parcel



  sender_name                           VARCHAR(150)               NOT NULL             Pickup contact name


<!-- PDF page 12 -->

Column                                Type                       Constraints       Description



  sender_phone                          VARCHAR(20)                NOT NULL          Pickup contact phone



  pickup_address                        VARCHAR(255)               NOT NULL          Pickup address line



  pickup_city                           VARCHAR(100)               NOT NULL          Pickup city



  pickup_zone_id                        UUID                       NOT NULL, ref     Pickup zone

                                                                   zones



  recipient_name                        VARCHAR(150)               NOT NULL          Drop-off contact name



  recipient_phone                       VARCHAR(20)                NOT NULL          Drop-off contact phone



  dropoff_address                       VARCHAR(255)               NOT NULL          Drop-off address line



  dropoff_city                          VARCHAR(100)               NOT NULL          Drop-off city



  dropoff_zone_id                       UUID                       NOT NULL, ref     Drop-off zone

                                                                   zones



  weight_kg                             NUMERIC(6,2)               NOT NULL,         Parcel weight

                                                                   CHECK > 0



  size_category                         VARCHAR(12)                NOT NULL          SMALL, MEDIUM, LARGE,

                                                                                     OVERSIZED



  description                           VARCHAR(255)               NULL              Contents description



  status                                VARCHAR(20)                NOT NULL,         Current status

                                                                   DEFAULT

                                                                   'CREATED'



  failure_reason                        VARCHAR(255)               NULL              Set when status is FAILED (5

                                                                                     to 255 characters)



  current_courier_id                    UUID                       NULL, ref users   Currently assigned courier

                                                                                     (copy of the active

                                                                                     assignment for fast lookup)



  price_amount                          NUMERIC(12,2)              NOT NULL          Total fee applied at creation



  currency                              CHAR(3)                    NOT NULL,         Currency

                                                                   DEFAULT 'LKR'


<!-- PDF page 13 -->

Column                                Type                       Constraints   Description



  price_breakdown                       JSONB                      NOT NULL      Keys base_fee ,

                                                                                 weight_charge ,

                                                                                 size_surcharge . Maps to

                                                                                 the API fee object.

                                                                                  price_amount equals their

                                                                                 sum



  created_by_user_id                    UUID                       NOT NULL      Who created the record

                                                                                 (customer, or the merchant

                                                                                 owner user)



  created_at                            TIMESTAMPTZ                NOT NULL      Creation time



  updated_at                            TIMESTAMPTZ                NOT NULL      Last update time



  delivered_at                          TIMESTAMPTZ                NULL          Set when DELIVERED



  failed_at                             TIMESTAMPTZ                NULL          Set when FAILED (new).

                                                                                 Needed for the courier day

                                                                                 list and reports



  delivered_to_name                     VARCHAR(150)               NULL          Recipient who accepted the

                                                                                 parcel. Required when

                                                                                 DELIVERED (new, API 6.3)





Constraints:



      CHECK that exactly one of customer_user_id and merchant_id is set (v1.0 allowed

     both).

      UNIQUE (merchant_id, external_order_id) where both are not null.

      UNIQUE (customer_user_id, idempotency_key) where both are not null.



      CHECK that failure_reason is set when status = 'FAILED' , and delivered_at is

     set when status = 'DELIVERED' .



Indexes: customer_user_id , merchant_id , current_courier_id , (status,

created_at) , created_at , delivered_at , failed_at , and (pickup_zone_id,

created_at) for the dispatch filter (5.1).





5.2 parcel_status_history

Append-only. Every status change inserts one row.


<!-- PDF page 14 -->

Column                          Type                             Constraints       Description



  id                              UUID                             PK                History ID



  parcel_id                       UUID                             NOT NULL, FK      Parcel

                                                                   parcels.id



  previous_status                 VARCHAR(20)                      NULL              NULL for the first row



  new_status                      VARCHAR(20)                      NOT NULL          New status



  actor_user_id                   UUID                             NULL              Who made the change (ref

                                                                                     users)



  actor_role                      VARCHAR(20)                      NULL              Role at the time (returned as

                                                                                     actorRole by 4.4)



  note                            VARCHAR(500)                     NULL              Delivery note



  failure_reason                  VARCHAR(255)                     NULL              Required when new_status is

                                                                                     FAILED



  created_at                      TIMESTAMPTZ                      NOT NULL          Time of change





Indexes: (parcel_id, created_at) and (new_status, created_at) for the daily and

outcome reports (7.1, 7.2).





5.3 parcel_assignments



  Column                        Type                           Constraints                Description



  id                            UUID                           PK                         Assignment ID



  parcel_id                     UUID                           NOT NULL, FK parcels.id    Parcel



  courier_id                    UUID                           NOT NULL, ref users        Courier



  assigned_by                   UUID                           NOT NULL, ref users        Dispatcher or admin



  assigned_at                   TIMESTAMPTZ                    NOT NULL                   Assignment time



  unassigned_at                 TIMESTAMPTZ                    NULL                       Set on reassignment



  is_current                    BOOLEAN                        NOT NULL, DEFAULT true     Active assignment



  reason                        VARCHAR(255)                   NULL                       Reassignment reason


<!-- PDF page 15 -->

Constraint: partial unique index on (parcel_id) where is_current . This guarantees

one active courier per parcel. Index on (courier_id, is_current) for workload queries

(5.3 in the API). In one transaction, reassignment closes the old row ( is_current false,

 unassigned_at set), inserts the new row and updates parcels.current_courier_id .





5.4 delivery_proofs



  Column                              Type                         Constraints            Description



  id                                  UUID                         PK                     Proof ID



  parcel_id                           UUID                         NOT NULL, FK           Parcel

                                                                   parcels.id



  recipient_name                      VARCHAR(150)                 NULL                   Person who received it



  note                                VARCHAR(500)                 NULL                   Courier note



  photo_path                          VARCHAR(255)                 NULL                   Safe generated storage

                                                                                          name



  photo_mime_type                     VARCHAR(30)                  NULL                   image/jpeg or image/png

                                                                                          only



  photo_size_bytes                    INT                          NULL                   Size limit enforced (5 MB)



  captured_by                         UUID                         NOT NULL               Courier (ref users)



  created_at                          TIMESTAMPTZ                  NOT NULL               Time captured





Index: (parcel_id, created_at) . The API returns the most recent proof as proof and

derives hasPhoto from photo_path IS NOT NULL .





5.5 outbox_events

Supports the outbox pattern so an event is never lost if RabbitMQ is down at commit

time.

The `event_type` and `routing_key` values in this table are storage-level labels retained for the existing outbox baseline. They are not the published RabbitMQ event names or routing keys. The relay maps `parcel.created`, `parcel.assigned`, and `parcel.status_changed` to the versioned events `parcel.created.v1`, `parcel.assigned.v1`, and `parcel.status_changed.v1`, using the versioned event name as the published routing key. The stored `routing_key` is legacy/internal and must not be copied directly to the broker. The `payload` column stores the versioned event's `data` object; the relay builds the canonical envelope from the row. No database schema change is introduced by this mapping. See [`contracts/events/README.md`](../contracts/events/README.md).





  Column                     Type                            Constraints          Description



  id                         UUID                            PK                   Same value as the event's

                                                                                  eventId



  event_type                 VARCHAR(50)                     NOT NULL             Storage label: parcel.created,

                                                                                  parcel.assigned, or

                                                                                  parcel.status_changed


<!-- PDF page 16 -->

Column                     Type                            Constraints          Description



  routing_key                VARCHAR(60)                     NOT NULL              Legacy storage key: parcel.created,

                                                                                   parcel.assigned, or

                                                                                   parcel.status.changed



  payload                    JSONB                           NOT NULL             Event data object; relay builds the

                                                                                  canonical envelope. No addresses, phones,

                                                                                  tokens or photos.



  status                     VARCHAR(12)                     NOT NULL,            PENDING, PUBLISHED, FAILED

                                                             DEFAULT

                                                             'PENDING'



  attempts                   INT                             NOT NULL,            Publish attempts

                                                             DEFAULT 0



  last_error                 VARCHAR(255)                    NULL                 Safe error text from the last failed

                                                                                  publish (new)



  created_at                 TIMESTAMPTZ                     NOT NULL             Insert time



  published_at               TIMESTAMPTZ                     NULL                 Publish time



  claim_token                UUID                            NULL                 Relay claim token (additive,

                                                                                  migration 000007); scopes

                                                                                  terminal updates to one relay



  claimed_at                 TIMESTAMPTZ                     NULL                 Claim time (additive, migration

                                                                                  000007); a claim older than

                                                                                  the stale window is reclaimable



Index: (status, created_at) . The event type uses an underscore while the routing key

uses dots. This matches API section 11.3.

The relay claims a batch atomically with `FOR UPDATE SKIP LOCKED` before the

broker publish, so two Parcel replicas take disjoint rows and never publish the

same event. The claim commits before the publish, so no row lock is held across

the network call; a crashed replica's stale claim is reclaimed after the window,

and every terminal update is scoped to the claim token. See `migrations/parcel/000007_outbox_claim.up.sql`.







## 6. Merchant Integration Service Tables



6.1 merchants



  Column                                Type                        Constraints          Description



  id                                    UUID                        PK                   Merchant ID



  owner_user_id                         UUID                        NOT NULL,            Merchant login account.

                                                                    UNIQUE, ref users    One merchant per owner

                                                                                         (new unique)



  business_name                         VARCHAR(150)                NOT NULL             Business name



  business_reg_no                       VARCHAR(50)                 NULL                 Registration number



  contact_email                         VARCHAR(255)                NOT NULL             Contact email



  contact_phone                         VARCHAR(20)                 NOT NULL             Contact phone



  pickup_address                        VARCHAR(255)                NOT NULL             Default pickup address


<!-- PDF page 17 -->

Column                                Type                       Constraints          Description



  pickup_zone_id                        UUID                       NOT NULL, ref        Default pickup zone. Now

                                                                   zones                required, because

                                                                                        registration (8.1) requires it

                                                                                        and orders fall back to it

                                                                                        (8.16)



  status                                VARCHAR(12)                NOT NULL,            PENDING, APPROVED,

                                                                   DEFAULT              REJECTED, SUSPENDED

                                                                   'PENDING'



  approved_by                           UUID                       NULL                 Admin (ref users)



  approved_at                           TIMESTAMPTZ                NULL                 Approval time



  rejection_reason                      VARCHAR(255)               NULL                 Reason if rejected



  suspension_reason                     VARCHAR(255)               NULL                 Reason if suspended (new,

                                                                                        API 8.8). Cleared on re-

                                                                                        approval



  created_at                            TIMESTAMPTZ                NOT NULL             Registration time



  updated_at                            TIMESTAMPTZ                NOT NULL             Last update time





Index: (status, created_at) for the admin list (8.4). State changes follow the API rules:

approve from PENDING or SUSPENDED, reject from PENDING, suspend from APPROVED

(409 INVALID_MERCHANT_STATE otherwise).





6.2 merchant_api_clients

An API key looks like sdk_live_<lookup_id>_<secret> , for example

 sdk_live_ab12cd34_N8x2... . The full key is shown once at creation.





  Column                     Type                           Constraints            Description



  id                         UUID                           PK                     Client ID



  merchant_id                UUID                           NOT NULL, FK           Owner

                                                            merchants.id



  name                       VARCHAR(100)                   NOT NULL               Label, for example Production


<!-- PDF page 18 -->

Column                     Type                           Constraints          Description



  key_prefix                 VARCHAR(20)                    NOT NULL             Display prefix returned by 8.9, for

                                                                                 example sdk_live_ab12 . Not

                                                                                 unique. Widened from 12

                                                                                 characters, because the example

                                                                                 is 13



  lookup_id                  VARCHAR(16)                    NOT NULL, UNIQUE     The random segment, for

                                                                                 example ab12cd34 . Used to find

                                                                                 the row on each request (new).

                                                                                 v1.0 used a unique prefix, which

                                                                                 would collide often with only 4

                                                                                 random characters



  key_hash                   CHAR(64)                       NOT NULL             SHA-256 of the full API key



  status                     VARCHAR(10)                    NOT NULL, DEFAULT    ACTIVE or REVOKED

                                                            'ACTIVE'



  last_used_at               TIMESTAMPTZ                    NULL                 Last successful use



  created_at                 TIMESTAMPTZ                    NOT NULL             Creation time



  revoked_at                 TIMESTAMPTZ                    NULL                 Revocation time





Index: (merchant_id, status) . Authentication: split the key, load by lookup_id ,

compare the hash in constant time, then require status = 'ACTIVE' and the merchant

to be APPROVED (suspended merchants are rejected, 8.8). A merchant may hold at most

5 ACTIVE keys (409 KEY_LIMIT_REACHED ); check it in a transaction that locks the

merchant row. The unused expires_at column from v1.0 is removed.





6.3 merchant_webhooks



  Column                                Type                       Constraints           Description



  id                                    UUID                       PK                    Webhook ID



  merchant_id                           UUID                       NOT NULL, UNIQUE,     Owner. Unique, because

                                                                   FK merchants.id       the API exposes one

                                                                                         webhook per merchant

                                                                                         (new)



  url                                   VARCHAR(500)               NOT NULL              HTTPS callback URL, not

                                                                                         a private or loopback

                                                                                         address


<!-- PDF page 19 -->

Column                                Type                       Constraints         Description



  signing_secret_enc                    BYTEA                      NOT NULL            Encrypted HMAC secret.

                                                                                       Returned only by the

                                                                                       internal API



  event_types                           TEXT[]                     NOT NULL            Subscribed event types



  is_active                             BOOLEAN                    NOT NULL, DEFAULT   Enabled

                                                                   true



  created_at                            TIMESTAMPTZ                NOT NULL            Creation time



  updated_at                            TIMESTAMPTZ                NOT NULL            Last update time







6.4 merchant_orders

Merchant view of orders. Stores the link to the parcel created in Parcel Operations,

supports idempotency, and keeps a snapshot of the fields the dashboard lists (8.17, 8.18)

so the list does not call Parcel Operations once per row.





  Column                               Type                        Constraints     Description



  id                                   UUID                        PK              Order ID ( orderId in the

                                                                                   API)



  merchant_id                          UUID                        NOT NULL, FK    Owner

                                                                   merchants.id



  external_order_id                    VARCHAR(100)                NOT NULL        Merchant's own order

                                                                                   number



  idempotency_key                      VARCHAR(100)                NULL            Optional client-supplied key



  request_hash                         CHAR(64)                    NOT NULL        SHA-256 of the normalised

                                                                                   request payload (new). Same

                                                                                    external_order_id with a

                                                                                   different hash returns 409

                                                                                    DUPLICATE_ORDER ; the same

                                                                                   hash returns the original

                                                                                   order



  parcel_id                            UUID                        NOT NULL,       Parcel in Parcel Operations

                                                                   UNIQUE, ref

                                                                   parcels


<!-- PDF page 20 -->

Column                               Type                         Constraints     Description



  tracking_number                      VARCHAR(20)                  NOT NULL,       Public tracking number

                                                                    UNIQUE



  recipient_name                       VARCHAR(150)                 NOT NULL        Snapshot for the list view

                                                                                    (new)



  dropoff_city                         VARCHAR(100)                 NOT NULL        Snapshot for the list view

                                                                                    (new)



  weight_kg                            NUMERIC(6,2)                 NOT NULL        Snapshot for the detail view

                                                                                    (new)



  size_category                        VARCHAR(12)                  NOT NULL        Snapshot for the detail view

                                                                                    (new)



  description                          VARCHAR(255)                 NULL            Snapshot for the detail view

                                                                                    (new)



  fee_amount                           NUMERIC(12,2)                NOT NULL        Total fee returned at creation

                                                                                    (new)



  fee_breakdown                        JSONB                        NOT NULL        Base fee, weight charge, size

                                                                                    surcharge (new)



  currency                             CHAR(3)                      NOT NULL,       Currency (new)

                                                                    DEFAULT 'LKR'



  last_known_status                    VARCHAR(20)                  NOT NULL        Updated from status events



  last_status_at                       TIMESTAMPTZ                  NOT NULL        occurredAt of the last

                                                                                    applied event (new). Older

                                                                                    events are ignored



  created_at                           TIMESTAMPTZ                  NOT NULL        Creation time



  updated_at                           TIMESTAMPTZ                  NOT NULL        Last update time





Migration 000010 adds the pickup and recipient contact snapshots used by API
8.16: pickup_name, pickup_phone, pickup_address, pickup_city,
pickup_zone_id, recipient_phone, recipient_address and recipient_zone_id.
They are nullable for pre-M8 rows and are populated for every order created by
the merchant order API.

Constraints: UNIQUE (merchant_id, external_order_id) and partial UNIQUE

(merchant_id, idempotency_key) where not null. Indexes: (merchant_id,

created_at) , (merchant_id, updated_at) , (merchant_id, last_known_status) .





6.5 webhook_delivery_attempts



  Column                            Type                           Constraints            Description



  id                                UUID                           PK                     Attempt ID


<!-- PDF page 21 -->

Column                            Type                           Constraints               Description



  event_id                          UUID                           NOT NULL                  The RabbitMQ event ID



  webhook_id                        UUID                           NOT NULL, FK              Target webhook

                                                                   merchant_webhooks.id



  merchant_id                       UUID                           NOT NULL, FK              Merchant (now a real

                                                                   merchants.id              foreign key, same

                                                                                             service)



  parcel_id                         UUID                           NOT NULL                  Parcel (ref)



  tracking_number                   VARCHAR(20)                    NOT NULL                  Needed to filter the log

                                                                                             by trackingNumber

                                                                                             (new, API 8.15)



  attempt_number                    INT                            NOT NULL, CHECK           1 to 6 as in the retry

                                                                   BETWEEN 1 AND 6           policy



  http_status                       INT                            NULL                      Response code, NULL

                                                                                             on timeout



  success                           BOOLEAN                        NOT NULL                  Outcome



  error_message                     VARCHAR(255)                   NULL                      Safe error text (no

                                                                                             payloads)



  next_retry_at                     TIMESTAMPTZ                    NULL                      Scheduled retry



  dead_lettered                     BOOLEAN                        NOT NULL, DEFAULT false   Set after attempt 6 fails



  duration_ms                       INT                            NULL                      Request duration



  created_at                        TIMESTAMPTZ                    NOT NULL                  Attempt time





Constraint: UNIQUE (webhook_id, event_id, attempt_number) , so a retried or

redelivered message cannot record the same attempt twice. Indexes: (event_id,

webhook_id) , (merchant_id, created_at) , (merchant_id, tracking_number,

created_at) , (merchant_id, success, created_at) .





6.6 merchant_order_status_events (new)

Filled by the Merchant Integration consumer of merchant.sync.queue . It gives the status

timeline for 8.18, which no other table in this service holds, and makes the consumer

idempotent.


<!-- PDF page 22 -->

Column                          Type                             Constraints           Description



  id                              UUID                             PK                    Row ID



  order_id                        UUID                             NOT NULL, FK          Order

                                                                   merchant_orders.id



  event_id                        UUID                             NOT NULL, UNIQUE      RabbitMQ eventId .

                                                                                         Duplicates are ignored



  previous_status                 VARCHAR(20)                      NULL                  Previous status



  status                          VARCHAR(20)                      NOT NULL              New status



  occurred_at                     TIMESTAMPTZ                      NOT NULL              Event occurredAt



  created_at                      TIMESTAMPTZ                      NOT NULL              Insert time





Index: (order_id, occurred_at) . The timeline is returned ordered by occurred_at . The

first row (CREATED) is inserted with the order.





6.7 audit_logs (Merchant schema)

Same structure as 3.6. Written for merchant approve, reject and suspend (8.6, 8.7, 8.8).







## 7. Notification Worker Tables



7.1 notification_logs



  Column                        Type                               Constraints          Description



  id                            UUID                               PK                   Log ID



  event_id                      UUID                               NOT NULL             Source event



  parcel_id                     UUID                               NOT NULL             Parcel



  channel                       VARCHAR(10)                        NOT NULL             EMAIL or SMS



  recipient                     VARCHAR(255)                       NOT NULL             Email address or phone

                                                                                        (mask in logs)



  subject                       VARCHAR(200)                       NULL                 Email subject



  status                        VARCHAR(10)                        NOT NULL, DEFAULT    PENDING, SENT, FAILED

                                                                   'PENDING'


<!-- PDF page 23 -->

Column                        Type                               Constraints                  Description



  error_message                 VARCHAR(255)                       NULL                         Safe failure text



  created_at                    TIMESTAMPTZ                        NOT NULL                     Created time



  sent_at                       TIMESTAMPTZ                        NULL                         Sent time





Constraint: UNIQUE (event_id, channel, recipient) to stop duplicate sends.

Password-reset emails are sent by the Identity service, not by this worker, so they are not

logged here and no reset link is ever stored.





7.2 processed_events

Idempotency table for the Notification Worker. The Webhook Worker deduplicates

through webhook_delivery_attempts (6.5) and the Merchant sync consumer through

 merchant_order_status_events (6.6). The Realtime Service is stateless.





  Column                      Type                          Constraints          Description



  consumer                    VARCHAR(40)                   PK part 1            Consumer name



  event_id                    UUID                          PK part 2            Event processed



  processed_at                TIMESTAMPTZ                   NOT NULL             Time of processing









## 8. Logical Relationships (ER Summary)



  From                                     To                                            Cardinality        Notes



  roles                                    users                                         1 to many          Each user has one

                                                                                                            role



  users                                    courier_profiles                              1 to 0..1          Only couriers



  users                                    refresh_tokens                                1 to many          Sessions



  users                                    password_reset_tokens                         1 to many          Only the newest

                                                                                                            unused token is

                                                                                                            valid



  zones                                    pricing_rules                                 1 to many          Origin and

                                                                                         (twice)            destination



  parcels                                  parcel_status_history                         1 to many          Full audit trail


<!-- PDF page 24 -->

From                                     To                             Cardinality   Notes



  parcels                                  parcel_assignments             1 to many     One current at a

                                                                                        time



  parcels                                  delivery_proofs                1 to many     Usually one. The

                                                                                        API returns the

                                                                                        latest



  merchants                                merchant_api_clients           1 to many     Up to 5 active

                                                                                        keys



  merchants                                merchant_webhooks              1 to 0..1     One webhook per

                                                                                        merchant



  merchants                                merchant_orders                1 to many     Unique on

                                                                                        external order ID



  merchant_orders                          merchant_order_status_events   1 to many     Status timeline

                                                                                        for the dashboard



  merchant_webhooks                        webhook_delivery_attempts      1 to many     Retry log, up to 6

                                                                                        attempts per

                                                                                        event



  users (ref)                              parcels                        1 to many     Customer owner,

                                                                                        by UUID only



  merchants (ref)                          parcels                        1 to many     Merchant owner,

                                                                                        by UUID only



  zones (ref)                              parcels                        1 to many     Pickup and drop-

                                                                                        off, by UUID only



  parcels (ref)                            merchant_orders                1 to 0..1     Merchant parcels

                                                                                        only, by UUID only





Cross-service links (ref) are not database foreign keys. The owning service validates them

through its REST API at write time.





## 9. Status Transition Rules (data level)



  From                                     Allowed to



  CREATED                                  PICKED_UP, FAILED



  PICKED_UP                                IN_TRANSIT, FAILED


<!-- PDF page 25 -->

From                                     Allowed to



  IN_TRANSIT                               OUT_FOR_DELIVERY, FAILED



  OUT_FOR_DELIVERY                         DELIVERED, FAILED



  DELIVERED                                none (terminal)



  FAILED                                   none (terminal in the MVP)





Every change must, in one transaction: update parcels.status (and delivered_at or

 failed_at , failure_reason , delivered_to_name where relevant), insert a

 parcel_status_history row, and insert an outbox_events row when the outbox

pattern is used. This matches API 6.3.







## 10. Seed Data



  Table                           Seed



  roles                           The five role names



  zones                           Sample zones such as Colombo (CMB), Gampaha,

                                  Kandy (KDY), Galle (GLE), Jaffna



  pricing_rules                   A base fee for every active zone pair (illustrative

                                  LKR values, for example 450.00 for Colombo to

                                  Kandy)



  weight_bands                    0 to 1 kg charge 0.00, 1 to 5 kg charge 100.00, 5 to 10

                                  kg charge 250.00, 10 to 25 kg an illustrative higher

                                  charge



  size_surcharges                 SMALL 0, MEDIUM 150, LARGE 400, OVERSIZED

                                  900



  users                           One admin, one dispatcher, two couriers, one

                                  customer for demos (passwords from

                                  environment, never committed)





With these seeds the API example prices as 450.00 + 100.00 + 0.00 = 550.00 LKR for a 2 kg

SMALL parcel from Colombo to Kandy.


<!-- PDF page 26 -->

## 11. Security and Retention Notes

     Passwords are stored only as bcrypt hashes. Refresh tokens, password-reset tokens

     and API keys are stored only as SHA-256 hashes.

     Webhook signing secrets are encrypted at rest with a key from the environment.

     Personal data columns (names, phones, addresses) and reset tokens are never written

     to logs, audit_logs or error messages.

     Public tracking queries must select only the designated public columns:

      tracking_number , status , history timestamps and statuses, and coarse locations

     such as city.

     Delivery proof photos are stored outside the database. Only a generated safe file name

     is kept.

     Recommended clean-up job (daily): delete oauth_states past expires_at ; delete

      refresh_tokens and password_reset_tokens expired more than 7 days ago; delete

      outbox_events PUBLISHED more than 14 days ago; delete processed_events older

     than 30 days. These retention periods are recommendations, not API requirements.





## Open decisions



  Decision                                                         Default used in this document



  May a Google-only account (no password)                          No. The endpoint still returns the same 202

  set a password through reset?                                    response, but no token is created for

                                                                   accounts with password_hash NULL



  One delivery proof per parcel or several?                        Several rows allowed; the API returns the

                                                                   latest



  Should merchant_orders keep a copy of the                        Yes, so the dashboard list needs no per-row

  recipient name and fee?                                          call to Parcel Operations


## Addendum: registration and merchant onboarding (implemented)

`identity.email_verification_tokens` stores a SHA-256 token hash, expiry
(default 30 minutes), request IP and used_at. A partial unique index allows one
unused token per user; rows cascade with their user.

`identity.users.session_version INTEGER NOT NULL DEFAULT 0` is the session
authority epoch. It is included in access tokens as `sv` and increments exactly
once for every newly applied merchant lifecycle transition.
`identity.merchant_lifecycle_transitions` records transition_id, user_id,
sequence, operation and applied_at, with a unique (user_id, sequence) and a
primary key on transition_id.

`merchant.merchants` gains business_address, a required non-blank
business_reg_no for new rows (NOT VALID check), a unique normalized business
registration index and lifecycle_sequence. rejection_reason is nullable and
widened to 1,000 characters; the old "reason required for REJECTED" check is
removed. Suspension still requires a reason.

`merchant.merchant_registration_attempts` persists idempotent registration saga
state. `merchant.merchant_status_transitions` records merchant-owned lifecycle
transitions including the Identity transition ID, sequence, actor, optional
merchant-facing rejection reason and synchronization status.
