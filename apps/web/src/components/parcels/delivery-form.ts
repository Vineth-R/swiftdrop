import type { NewParcel } from '../../lib/parcels-api';
import type { SizeCategory } from '../../lib/pricing-api';
import type { FormRules } from '../../lib/use-form';
import {
  toInternational,
  validateInternationalPhone,
} from '../../lib/validation';

export type DeliveryValues = {
  senderName: string;
  senderPhone: string;
  senderAddress: string;
  senderCity: string;
  senderZone: string;
  recipientName: string;
  recipientPhone: string;
  recipientAddress: string;
  recipientCity: string;
  recipientZone: string;
  weight: string;
  size: string;
  description: string;
};

export const initialValues: DeliveryValues = {
  senderName: '',
  senderPhone: '',
  senderAddress: '',
  senderCity: '',
  senderZone: '',
  recipientName: '',
  recipientPhone: '',
  recipientAddress: '',
  recipientCity: '',
  recipientZone: '',
  weight: '',
  size: 'SMALL',
  description: '',
};

const required = (value: string, max: number) =>
  value.trim().length > 0 && [...value.trim()].length <= max
    ? undefined
    : `Enter between 1 and ${max} characters.`;
const phone = (value: string) =>
  validateInternationalPhone(value) ??
  (/^\+[1-9]\d{8,14}$/.test(toInternational(value))
    ? undefined
    : 'Include the country code and between 9 and 15 digits.');
export const deliveryRules: FormRules<DeliveryValues> = {
  senderName: (v) => required(v.senderName, 150),
  senderPhone: (v) => phone(v.senderPhone),
  senderAddress: (v) => required(v.senderAddress, 255),
  senderCity: (v) => required(v.senderCity, 100),
  senderZone: (v) => (v.senderZone ? undefined : 'Choose a pickup zone.'),
  recipientName: (v) => required(v.recipientName, 150),
  recipientPhone: (v) => phone(v.recipientPhone),
  recipientAddress: (v) => required(v.recipientAddress, 255),
  recipientCity: (v) => required(v.recipientCity, 100),
  recipientZone: (v) =>
    v.recipientZone ? undefined : 'Choose a destination zone.',
  weight: (v) =>
    /^\d+(\.\d{1,2})?$/.test(v.weight) &&
    Number(v.weight) > 0 &&
    Number(v.weight) <= 9999.99
      ? undefined
      : 'Enter a weight above 0 and up to 9999.99 kg, with at most two decimals.',
  size: (v) =>
    ['SMALL', 'MEDIUM', 'LARGE', 'OVERSIZED'].includes(v.size)
      ? undefined
      : 'Choose a size.',
  description: (v) =>
    [...v.description].length <= 255
      ? undefined
      : 'Use no more than 255 characters.',
};

export const routeFields: (keyof DeliveryValues)[] = [
  'senderZone',
  'recipientZone',
  'weight',
  'size',
];

export function parcelInput(
  values: DeliveryValues,
  expectedFee: number,
): NewParcel {
  return {
    sender: {
      name: values.senderName.trim(),
      phone: toInternational(values.senderPhone),
      address: values.senderAddress.trim(),
      city: values.senderCity.trim(),
      zoneId: values.senderZone,
    },
    recipient: {
      name: values.recipientName.trim(),
      phone: toInternational(values.recipientPhone),
      address: values.recipientAddress.trim(),
      city: values.recipientCity.trim(),
      zoneId: values.recipientZone,
    },
    weightKg: Number(values.weight),
    sizeCategory: values.size as SizeCategory,
    ...(values.description.trim()
      ? { description: values.description.trim() }
      : {}),
    expectedFee,
  };
}
