# How SwiftDrop works

## Creating a delivery

A customer provides pickup details, drop-off details, the parcel weight and the parcel size. The platform obtains an authoritative delivery quote, stores the parcel and creates an initial status-history entry.

## Tracking number

A SwiftDrop tracking number starts with `SDK` or `SDN` followed by eight uppercase letters or digits, for example `SDK7M2Q9XP4`.

## Delivery statuses

The confirmed lifecycle statuses are:

- `CREATED`
- `PICKED_UP`
- `IN_TRANSIT`
- `OUT_FOR_DELIVERY`
- `DELIVERED`
- `FAILED`

## Status timeline

Each parcel keeps an append-only status history. The assistant shows a signed-in owner's parcel status and timeline. The separate public tracking page shows the status and timestamp of each public event without signing in.

## Parcel size categories

The confirmed size categories are `SMALL`, `MEDIUM`, `LARGE` and `OVERSIZED`. The web app shows draft guidance volumes (up to 5 L, up to 20 L, up to 60 L and over 60 L) but the volume limit itself is not enforced by the API. Treat those volumes as guidance, not a guarantee.

## What the customer needs to provide

- Pickup location and drop-off location.
- Parcel weight in kilograms.
- Parcel size category.
- For tracking: the tracking number.
- For private parcel views: sign in first.
