# Pricing FAQ

## How a delivery fee is calculated

The SwiftDrop pricing service calculates a fee from four inputs:

1. Pickup zone.
2. Drop-off zone.
3. Weight in kilograms.
4. Size category: `SMALL`, `MEDIUM`, `LARGE` or `OVERSIZED`.

A quote contains the currency, base fee, weight charge, size surcharge and total. Quotes are in Sri Lankan rupees (`LKR`).

## Why the assistant must call the pricing tool

Pricing is configuration-driven. Zones, base fees, weight bands and surcharges can change. The assistant must never quote a remembered or invented amount. It must call `calculate_delivery_fee` and report only the returned quote.

## Weight bands

The baseline pricing configuration uses these weight bands, with a lower-exclusive and upper-inclusive rule:

- More than 0 kg up to 1 kg.
- More than 1 kg up to 5 kg.
- More than 5 kg up to 10 kg.
- More than 10 kg up to 25 kg.

A weight outside the active bands has no price. Pricing returns a clear "weight not priced" result. The assistant must explain that the weight cannot be priced and must not guess.

## Serviceable routes

Pricing returns a quote only for configured routes. If a route is not serviceable, the assistant must say that no quote is available rather than inventing one.

## Missing details

If the pickup location, drop-off location, weight or size is missing, ask for the missing details before calling the pricing tool.

## Not confirmed

Do not promise fixed prices, discounts, promotions, free delivery, refunds or price matching. Only live Pricing service results are authoritative.
