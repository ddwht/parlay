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
  it('offers exactly the closed field-type vocabulary (8 scalars + 1 enum)', () => {
    // Customer + Order + one enum (OrderStatus) → 8 base types + OrderStatus.
    expect(fieldTypeOptions(populatedModel)).toHaveLength(9);
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
