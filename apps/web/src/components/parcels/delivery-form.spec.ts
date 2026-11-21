import { describe, expect, it } from 'vitest';
import { deliveryRules, initialValues, parcelInput } from './delivery-form';

describe('delivery input', () => {
  it('requires route, names, phones, addresses and a positive weight', () => {
    for (const [field, rule] of Object.entries(deliveryRules)) {
      if (field !== 'size' && field !== 'description')
        expect(rule(initialValues)).toBeTruthy();
    }
  });
  it.each(['0', '-1', '1.001', '10000', 'NaN', '1e3'])(
    'rejects unsupported weight %s',
    (weight) => {
      expect(deliveryRules.weight({ ...initialValues, weight })).toBeTruthy();
    },
  );
  it.each(['0.01', '1', '9999.99'])('accepts weight %s', (weight) => {
    expect(deliveryRules.weight({ ...initialValues, weight })).toBeUndefined();
  });
  it('normalizes contact whitespace and leaves the accepted fee unchanged', () => {
    const input = parcelInput(
      {
        ...initialValues,
        senderName: ' Sender ',
        senderPhone: '+94 771234567',
        senderAddress: ' Road ',
        senderCity: ' Colombo ',
        senderZone: 'zone-a',
        recipientName: ' Recipient ',
        recipientPhone: '+94712345678',
        recipientAddress: ' Street ',
        recipientCity: ' Kandy ',
        recipientZone: 'zone-b',
        weight: '2.50',
        description: ' Books ',
      },
      550,
    );
    expect(input).toEqual({
      sender: {
        name: 'Sender',
        phone: '+94771234567',
        address: 'Road',
        city: 'Colombo',
        zoneId: 'zone-a',
      },
      recipient: {
        name: 'Recipient',
        phone: '+94712345678',
        address: 'Street',
        city: 'Kandy',
        zoneId: 'zone-b',
      },
      weightKg: 2.5,
      sizeCategory: 'SMALL',
      description: 'Books',
      expectedFee: 550,
    });
  });
});
