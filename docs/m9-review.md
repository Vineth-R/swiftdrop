# M9 report review

No M9-owned report files were present in the repository during the November 1 review. The following checks remain open until those files are added:

- Section 1 should describe Merchant Integration as the owner of merchant accounts, order snapshots, status timelines, and audit logs; webhook configuration and delivery remain M9-owned.
- Section 2.1 should use the project roles CUSTOMER, MERCHANT, COURIER, DISPATCHER, and ADMIN, and should identify the External Merchant System as an integration actor where applicable.
- Section 2.2 and Figure 1 should use the repository's public prefixes and avoid presenting webhook delivery as a Merchant Integration database responsibility.
- Section 9 should trace merchant API order creation to `POST /api/v1/merchant/orders`, API-key authentication, `parcel.status_changed` events, and M9 webhook delivery without inventing a webhook payload schema.
- Status terminology should remain `CREATED`, `PICKED_UP`, `IN_TRANSIT`, `OUT_FOR_DELIVERY`, `DELIVERED`, and `FAILED`.
