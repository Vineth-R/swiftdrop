import { BrandPanel } from './brand-panel';

/** One line per page, in the voice of the brand: short, plain, about the parcel. */

export function LoginPanel() {
  return (
    <BrandPanel headline="Every parcel, point to point.">
      Log in and see exactly where yours is.
    </BrandPanel>
  );
}

export function RegisterPanel() {
  return (
    <BrandPanel headline="From first parcel to hundredth.">
      One account to send, track and receive.
    </BrandPanel>
  );
}

export function RecoveryPanel() {
  return (
    <BrandPanel headline="Back on the route in a minute.">
      A new password is two steps away.
    </BrandPanel>
  );
}

export function SignedOutPanel() {
  return (
    <BrandPanel headline="See you at the next stop.">
      Your parcels keep moving while you are away.
    </BrandPanel>
  );
}
