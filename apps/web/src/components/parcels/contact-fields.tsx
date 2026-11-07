import { TextField } from '../ui/field';
import type { DeliveryValues } from './delivery-form';

export function ContactFields({
  kind,
  values,
  errors,
  onChange,
  onBlur,
}: {
  kind: 'sender' | 'recipient';
  values: DeliveryValues;
  errors: Partial<Record<keyof DeliveryValues, string>>;
  onChange: (key: keyof DeliveryValues, value: string) => void;
  onBlur: (key: keyof DeliveryValues) => void;
}) {
  const title = kind === 'sender' ? 'Sender' : 'Recipient';
  const fields = [
    ['Name', 'name', 150],
    ['Phone', 'tel', 20],
    ['Address', 'street-address', 255],
    ['City', 'address-level2', 100],
  ] as const;
  return (
    <fieldset className="flex min-w-0 flex-col gap-4">
      <legend className="mb-4 text-h4 font-semibold">{title}</legend>
      {fields.map(([field, autocomplete, max]) => {
        const key = `${kind}${field}` as keyof DeliveryValues;
        return (
          <TextField
            key={key}
            id={`delivery-${key}`}
            label={`${title} ${field.toLowerCase()}`}
            isRequired
            value={values[key]}
            error={errors[key]}
            autoComplete={`section-${kind} shipping ${autocomplete}`}
            type={field === 'Phone' ? 'tel' : 'text'}
            maxLength={max}
            helper={
              field === 'Phone'
                ? 'Include the country code, for example +94771234567.'
                : undefined
            }
            onChange={(event) => onChange(key, event.target.value)}
            onBlur={() => onBlur(key)}
          />
        );
      })}
    </fieldset>
  );
}
