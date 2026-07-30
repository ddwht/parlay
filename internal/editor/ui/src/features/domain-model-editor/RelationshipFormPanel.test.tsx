// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: relationship-form-panel
// parlay-artifact: test
import { render, screen, fireEvent, within } from '@testing-library/react';
import { RelationshipFormPanel } from './RelationshipFormPanel';
import { useEditorStore, renameEntityInModel } from '../../store/editorStore';
import type { DomainModelDocument } from '../../types/domain';

function threeEntityModel(withRel: boolean): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [
      { name: 'Customer', fields: [{ name: 'id', type: 'uuid', required: true }] },
      { name: 'Order', fields: [{ name: 'id', type: 'uuid', required: true }] },
      { name: 'Product', fields: [{ name: 'id', type: 'uuid', required: true }] },
    ],
    relationships: withRel
      ? [{ name: 'customer-orders', from: 'Customer', to: 'Order', cardinality: 'one-to-many' }]
      : [],
  };
}

function selfRefModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [{ name: 'Employee', fields: [{ name: 'id', type: 'uuid', required: true }] }],
    relationships: [],
  };
}

/** Click the option button (radio) with the given data-value in a picker. */
function pick(pickerTestId: string, value: string) {
  const picker = screen.getByTestId(pickerTestId);
  const opt = within(picker)
    .getAllByRole('radio')
    .find((el) => el.getAttribute('data-value') === value)!;
  fireEvent.click(opt);
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('RelationshipFormPanel', () => {
  it('endpoint pickers offer exactly the declared entities', () => {
    useEditorStore.getState().hydrate({ model: threeEntityModel(false), etag: 'e1' });
    useEditorStore.getState().createRelationship();
    render(<RelationshipFormPanel />);
    expect(screen.getAllByTestId('from-option')).toHaveLength(3);
    expect(screen.getAllByTestId('to-option')).toHaveLength(3);
  });

  it('cardinality picker offers exactly the four closed values', () => {
    useEditorStore.getState().hydrate({ model: threeEntityModel(false), etag: 'e1' });
    useEditorStore.getState().createRelationship();
    render(<RelationshipFormPanel />);
    expect(screen.getAllByTestId('cardinality-option')).toHaveLength(4);
  });

  it('creates Customer → Order one-to-many, pre-filling the name and serializing to shape', () => {
    useEditorStore.getState().hydrate({ model: threeEntityModel(false), etag: 'e1' });
    useEditorStore.getState().createRelationship();
    render(<RelationshipFormPanel />);

    pick('from-picker', 'Customer');
    pick('to-picker', 'Order');
    expect(screen.getByTestId('name-field')).toHaveTextContent('customer-orders');

    pick('cardinality-picker', 'one-to-many');

    const rel = useEditorStore.getState().model.relationships![0];
    expect(rel).toEqual({
      name: 'customer-orders',
      from: 'Customer',
      to: 'Order',
      cardinality: 'one-to-many',
    });
  });

  it('rejects a duplicate relationship name at entry with a field-level message', () => {
    useEditorStore.getState().hydrate({ model: threeEntityModel(true), etag: 'e1' });
    useEditorStore.getState().createRelationship(); // second, blank, selected
    render(<RelationshipFormPanel />);

    fireEvent.change(screen.getByTestId('edit-name'), {
      target: { value: 'customer-orders' },
    });
    expect(screen.getByTestId('duplicate-name-message')).toBeInTheDocument();
  });

  it('accepts a self-referential relationship without special-casing', () => {
    useEditorStore.getState().hydrate({ model: selfRefModel(), etag: 'e1' });
    useEditorStore.getState().createRelationship();
    render(<RelationshipFormPanel />);

    pick('from-picker', 'Employee');
    pick('to-picker', 'Employee');
    pick('cardinality-picker', 'many-to-one');

    const rel = useEditorStore.getState().model.relationships![0];
    expect(rel.from).toBe('Employee');
    expect(rel.to).toBe('Employee');
    expect(rel.cardinality).toBe('many-to-one');
  });

  it('name pre-fill is a convenience — a manual rename survives an endpoint change', () => {
    useEditorStore.getState().hydrate({ model: threeEntityModel(false), etag: 'e1' });
    useEditorStore.getState().createRelationship();
    render(<RelationshipFormPanel />);

    pick('from-picker', 'Customer');
    pick('to-picker', 'Order'); // name pre-fills to customer-orders
    fireEvent.change(screen.getByTestId('edit-name'), {
      target: { value: 'places' },
    });
    pick('to-picker', 'Product'); // endpoint changes, but the manual name stays

    expect(screen.getByTestId('name-field')).toHaveTextContent('places');
    expect(useEditorStore.getState().model.relationships![0].name).toBe('places');
  });

  it('entity rename propagates into endpoints while a manual name is left untouched', () => {
    const model = threeEntityModel(false);
    model.relationships = [
      { name: 'places', from: 'Customer', to: 'Order', cardinality: 'one-to-many' },
    ];
    useEditorStore.getState().hydrate({ model, etag: 'e1' });

    // Renaming Order → Invoice rewrites the endpoint; the manual name stays put.
    const next = renameEntityInModel(
      useEditorStore.getState().model,
      'Order',
      'Invoice',
    );
    useEditorStore.getState().applyModel(next);

    const rel = useEditorStore.getState().model.relationships![0];
    expect(rel.to).toBe('Invoice');
    expect(rel.name).toBe('places');
  });
});
