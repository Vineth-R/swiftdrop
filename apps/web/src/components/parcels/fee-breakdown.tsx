import { formatLkr } from '../../lib/format';
import type { ParcelFee } from '../../lib/parcels-api';

export function FeeBreakdown({ fee }: { fee: ParcelFee }) {
  return (
    <dl className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
      {[
        ['Base fee', fee.baseFee],
        ['Weight charge', fee.weightCharge],
        ['Size surcharge', fee.sizeSurcharge],
        ['Total', fee.total],
      ].map(([label, amount]) => (
        <div key={label} className="flex flex-wrap justify-between gap-3">
          <dt className="text-body-sm text-text-muted">{label}</dt>
          <dd className="text-right font-mono text-body-sm tabular">
            {formatLkr(Number(amount))}
          </dd>
        </div>
      ))}
    </dl>
  );
}
