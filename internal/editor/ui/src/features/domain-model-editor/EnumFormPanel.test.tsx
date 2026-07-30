// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: enum-form-panel
// parlay-artifact: test
import { render, screen, fireEvent } from '@testing-library/react';
import { EnumFormPanel } from './EnumFormPanel';
import {
  useEditorStore,
  normalizeEnumValue,
  renameEnumInModel,
} from '../../store/editorStore';
import { populatedModel, populatedEnvelope, clone } from '../../test/fixtures';

beforeEach(() => {
  useEditorStore.getState().resetStore();
  useEditorStore.getState().hydrate(clone(populatedEnvelope));
  useEditorStore.getState().selectEnum('OrderStatus');
});

describe('EnumFormPanel', () => {
  it('offers exactly the five tones as rendered badge previews', () => {
    render(<EnumFormPanel />);
    expect(screen.getAllByTestId('tone-badge-preview')).toHaveLength(5);
    // "none" is offered too, but as its own control rather than a badge.
    expect(screen.getByTestId('tone-none')).toBeInTheDocument();
  });

  it('omits unset label and tone rather than writing empty strings', () => {
    const bare = normalizeEnumValue('paid', '', 'none');
    expect(bare).toEqual({ value: 'paid' });
    expect('label' in bare).toBe(false);
    expect('tone' in bare).toBe(false);

    const full = normalizeEnumValue('pending', 'Pending', 'warning');
    expect(full).toEqual({ value: 'pending', label: 'Pending', tone: 'warning' });
  });

  it('rejects a duplicate value at entry with a field-level message', () => {
    render(<EnumFormPanel />);
    fireEvent.change(screen.getByTestId('edit-value'), {
      target: { value: 'pending' },
    });
    expect(screen.getByTestId('duplicate-value-message')).toHaveTextContent(
      'is already a value in this enum',
    );
    expect(screen.getByTestId('add-value')).toBeDisabled();
  });

  it('rewrites referencing field companion keys on rename', () => {
    const next = renameEnumInModel(populatedModel, 'OrderStatus', 'Status');
    const order = next.entities.find((e) => e.name === 'Order')!;
    const status = order.fields.find((f) => f.name === 'status')!;
    expect(status.type).toBe('Status');
    expect(status.enum).toBe('Status');
  });
});
