import { RouteLine } from '../ui/route-line';

/** Login panel: a parcel's journey as two stops on a vertical route. */
export function LoginPanel() {
  return (
    <div className="flex flex-col gap-10">
      <div className="flex items-stretch gap-8">
        <RouteLine
          orientation="vertical"
          state="travelling"
          nodeSize={10}
          cycleMs={3000}
          className="min-h-64"
        />
        <div className="flex flex-col justify-between gap-8 py-0.5">
          <Stop place="Colombo" text="Picked up at 14:02" />
          <Stop place="Kandy" text="Delivered today" />
        </div>
      </div>
      <p className="max-w-[42ch] text-body-sm text-text-muted">
        Sign in to follow your parcels from pickup to doorstep.
      </p>
    </div>
  );
}

function Stop({ place, text }: { place: string; text: string }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-overline uppercase text-text-faint">{place}</span>
      <span className="font-display text-h2">{text}</span>
    </div>
  );
}

const benefits = [
  {
    title: 'See the fee first',
    text: 'Priced from the zones and the weight, before you confirm.',
  },
  {
    title: 'Every status, as it happens',
    text: 'Six statuses from created to delivered, with the courier’s name.',
  },
  {
    title: 'Upgrade to a merchant account later',
    text: 'Same login, plus API keys and signed webhooks.',
  },
] as const;

/** Register panel: why one account is enough. */
export function RegisterPanel() {
  return (
    <div className="flex flex-col gap-6">
      <h2 className="max-w-[20ch] text-balance font-display text-h2">
        One account, from first parcel to hundredth.
      </h2>
      <ul className="flex flex-col gap-5">
        {benefits.map((benefit, index) => (
          <li key={benefit.title} className="flex items-start gap-4">
            <span
              aria-hidden="true"
              className={
                index === 0
                  ? 'mt-1.5 size-2 shrink-0 rounded-full bg-accent'
                  : 'mt-1.5 size-2 shrink-0 rounded-full border-[1.5px] border-route-node'
              }
            />
            <div className="flex flex-col gap-0.5">
              <span className="text-label font-semibold">{benefit.title}</span>
              <span className="text-body-sm text-text-muted">
                {benefit.text}
              </span>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Reset and forgot-password panel. */
export function RecoveryPanel() {
  return (
    <div className="flex flex-col gap-4">
      <h2 className="max-w-[20ch] text-balance font-display text-h2">
        Back in, in a minute.
      </h2>
      <p className="max-w-[42ch] text-body-sm text-text-muted">
        A reset link works once and expires after 30 minutes. Using it signs you
        out everywhere else.
      </p>
    </div>
  );
}
