import type { DomainModelDocument } from '../types/domain';
import type { ModelEnvelope } from '../lib/api';

export const populatedModel: DomainModelDocument = {
  schema_version: 1,
  enums: [
    {
      name: 'OrderStatus',
      values: [
        { value: 'pending', label: 'Pending', tone: 'warning' },
        { value: 'paid', tone: 'success' },
      ],
    },
  ],
  entities: [
    {
      name: 'Customer',
      fields: [
        { name: 'id', type: 'uuid', required: true },
        { name: 'name', type: 'string', required: true },
      ],
    },
    {
      name: 'Order',
      fields: [
        { name: 'id', type: 'uuid', required: true },
        { name: 'status', type: 'OrderStatus', enum: 'OrderStatus', required: true },
        { name: 'customer_id', type: 'ref', target: 'Customer', required: true },
      ],
    },
  ],
  relationships: [],
  operations: [{ name: 'placeOrder', kind: 'command' }],
};

export const emptyModel: DomainModelDocument = {
  schema_version: 1,
  enums: [],
  entities: [],
};

export const LOAD_ETAG = 'sha256:9f2c-load';
export const CURRENT_ETAG = 'sha256:77e0-current';
export const EMPTY_ETAG = 'empty';

export const populatedEnvelope: ModelEnvelope = {
  model: populatedModel,
  etag: LOAD_ETAG,
};

export const emptyEnvelope: ModelEnvelope = {
  model: emptyModel,
  etag: EMPTY_ETAG,
};

/** deep clone helper so tests never mutate the shared fixture */
export function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}
