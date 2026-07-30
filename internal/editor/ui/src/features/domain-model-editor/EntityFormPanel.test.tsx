// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: entity-form-panel
// parlay-artifact: test
import { render, screen, fireEvent } from '@testing-library/react';
import { EntityFormPanel } from './EntityFormPanel';
import {
  useEditorStore,
  applyFieldType,
  renameEntityInModel,
} from '../../store/editorStore';
import { fieldTypeOptions } from '../../types/domain';
import { populatedModel, populatedEnvelope, clone } from '../../test/fixtures';

beforeEach(() => {
  useEditorStore.getState().resetStore();
  useEditorStore.getState().hydrate(clone(populatedEnvelope));
});

describe('EntityFormPanel', () => {
  // Asserts the MEMBERS, not the count.
  //
  // This used to assert only a length of 9, and that is exactly how the list
  // drifted from the schema in both directions at once without anything
  // noticing: it offered `timestamp` and `bytes`, which are not in the closed
  // set and fail deep validation with field-type-outside-closed-set, and it
  // omitted `datetime`, which IS in the set and is what real models use for
  // timestamps — so there was no way to author a datetime field through the
  // UI at all, while two of the options on offer were dead ends. A count
  // assertion was satisfied by any eight wrong names.
  //
  // The expected list mirrors domain-model.schema.md's "Closed field-type
  // set" table. If that table changes, this must change with it.
  it('offers exactly the closed field-type vocabulary from the schema', () => {
    expect(fieldTypeOptions(populatedModel)).toEqual([
      'uuid',
      'string',
      'int',
      'float',
      'bool',
      'datetime',
      'ref',
      'OrderStatus', // the one declared enum in the fixture
    ]);
    useEditorStore.getState().selectEntity('Customer');
    render(<EntityFormPanel />);
    expect(screen.getByTestId('choose-field-type')).toBeInTheDocument();
  });

  it('auto-sets the enum companion key when an enum type is chosen', () => {
    const field = { name: 'status', type: 'string', required: true };
    const next = applyFieldType(field, populatedModel, 'OrderStatus');
    expect(next.type).toBe('OrderStatus');
    expect(next.enum).toBe('OrderStatus');
  });

  it('a ref field needs a target chosen separately before it commits', () => {
    const field = { name: 'customer_id', type: 'string', required: true };
    const next = applyFieldType(field, populatedModel, 'ref');
    expect(next.type).toBe('ref');
    expect(next.enum).toBeUndefined();
    // ref carries no target yet — the picker supplies it before commit.
    expect(next.target).toBeUndefined();
  });

  it('rejects a duplicate entity name at entry with a field-level message', () => {
    useEditorStore.getState().selectEntity('Customer');
    render(<EntityFormPanel />);
    fireEvent.change(screen.getByTestId('edit-entity-name'), {
      target: { value: 'Order' },
    });
    expect(screen.getByTestId('duplicate-name-message')).toBeInTheDocument();
  });

  it('propagates a rename atomically to every referencing ref target', () => {
    const next = renameEntityInModel(populatedModel, 'Customer', 'Client');
    const order = next.entities.find((e) => e.name === 'Order')!;
    const customerId = order.fields.find((f) => f.name === 'customer_id')!;
    expect(customerId.target).toBe('Client');
    expect(next.entities.some((e) => e.name === 'Client')).toBe(true);
    expect(next.entities.some((e) => e.name === 'Customer')).toBe(false);
  });

  it('preserves a reordered field order', () => {
    useEditorStore.getState().selectEntity('Order');
    render(<EntityFormPanel />);
    // Order fields: id, status, customer_id. Move the first field down.
    fireEvent.click(screen.getAllByTestId('move-field-down')[0]);
    const order = useEditorStore
      .getState()
      .model.entities.find((e) => e.name === 'Order')!;
    expect(order.fields.map((f) => f.name)).toEqual([
      'status',
      'id',
      'customer_id',
    ]);
  });
});
