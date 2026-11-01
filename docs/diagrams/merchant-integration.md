# Merchant Integration diagrams

## Figure 2 — Merchant use cases

```mermaid
flowchart LR
  merchant([Merchant])
  admin([Admin])
  system[[SwiftDrop Merchant Integration]]
  ext([External Merchant System])
  merchant --- reg(Register as merchant)
  merchant --- keys(Manage API keys)
  merchant --- profile(Manage business profile)
  merchant --- dashboard(View order dashboard)
  merchant --- webhook(Configure and test webhook)
  admin --- approval(Approve, reject, or suspend merchant)
  ext --- create(Create order through API)
  ext --- receive(Receive status webhooks)
  reg --- system
  keys --- system
  profile --- system
  dashboard --- system
  webhook --- system
  approval --- system
  create --- system
  receive --- system
```

## Figure 10 — Merchant order sequence

```mermaid
sequenceDiagram
  participant M as Merchant System
  participant I as Merchant Integration
  participant P as Parcel Operations
  participant Z as Pricing
  participant R as RabbitMQ
  M->>I: POST /merchant/orders + X-API-Key
  I->>I: Authenticate, approve, validate, deduplicate
  I->>P: POST /internal/parcels (internal auth)
  P->>Z: Calculate fee
  Z-->>P: Fee breakdown
  P-->>I: parcelId, trackingNumber, fee
  I->>I: Save order snapshot and CREATED timeline
  I-->>M: 201 order response
  P->>R: parcel.status_changed event
  R-->>I: status event
  I->>I: Deduplicate and update snapshot/timeline
```

## Figure 11 — Merchant order activity

```mermaid
flowchart TD
  start((Start)) --> request[Receive order request]
  request --> auth{API key valid and merchant approved?}
  auth -- No --> error401[Return 401/403] --> end((End))
  auth -- Yes --> validate{Payload valid?}
  validate -- No --> error422[Return 422] --> end
  validate -- Yes --> duplicate{Duplicate request?}
  duplicate -- Same payload --> original[Return original order] --> end
  duplicate -- Conflicting payload --> error409[Return 409] --> end
  duplicate -- New --> pickup{Pickup supplied?}
  pickup -- No --> defaults[Use merchant defaults]
  pickup -- Yes --> parcel[Create parcel]
  defaults --> parcel
  parcel --> save[Save order snapshot and timeline]
  save --> success[Return 201] --> end
```
