// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: er-diagram-view
// parlay-artifact: test
import { render, screen, fireEvent, within, act } from '@testing-library/react';
import { ERDiagramView } from './ERDiagramView';
import { useEditorStore, renameEntityInModel } from '../../store/editorStore';
import type { DomainModelDocument } from '../../types/domain';

function relatedModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [
      { name: 'Customer', fields: [{ name: 'id', type: 'uuid', required: true }] },
      { name: 'Order', fields: [{ name: 'id', type: 'uuid', required: true }] },
    ],
    relationships: [
      { name: 'customer-orders', from: 'Customer', to: 'Order', cardinality: 'one-to-many' },
    ],
  };
}

function threeEntityModel(withRel: boolean): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [
      { name: 'Customer', fields: [] },
      { name: 'Order', fields: [] },
      { name: 'Product', fields: [] },
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
    entities: [{ name: 'Employee', fields: [] }],
    relationships: [],
  };
}

function refFieldModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [
      { name: 'Customer', fields: [] },
      {
        name: 'Order',
        fields: [{ name: 'customer_id', type: 'ref', target: 'Customer', required: true }],
      },
    ],
    relationships: [],
  };
}

function oneEntityModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [{ name: 'Customer', fields: [] }],
    relationships: [],
  };
}

function clickByData(testId: string, attr: string, value: string) {
  const el = screen
    .getAllByTestId(testId)
    .find((e) => e.getAttribute(attr) === value)!;
  fireEvent.click(el);
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('ERDiagramView', () => {
  it('renders one node per entity and one labelled edge per relationship', () => {
    useEditorStore.getState().hydrate({ model: relatedModel(), etag: 'e1' });
    render(<ERDiagramView />);
    expect(screen.getAllByTestId('entity-node')).toHaveLength(2);
    expect(screen.getAllByTestId('relationship-edge')).toHaveLength(1);
    expect(screen.getAllByTestId('cardinality-markers').length).toBeGreaterThan(0);
  });

  it('clicking a node opens the entity form; clicking an edge opens the relationship form', () => {
    useEditorStore.getState().hydrate({ model: relatedModel(), etag: 'e1' });
    render(<ERDiagramView />);

    clickByData('click-node', 'data-node', 'Order');
    expect(screen.getByTestId('node-side-panel')).toBeInTheDocument();

    clickByData('click-edge', 'data-edge', 'customer-orders');
    expect(screen.getByTestId('edge-side-panel')).toBeInTheDocument();
  });

  it('a cancelled draw-to-connect leaves the draft untouched', () => {
    useEditorStore.getState().hydrate({ model: relatedModel(), etag: 'e1' });
    render(<ERDiagramView />);

    act(() => useEditorStore.getState().proposeRelationship('Customer', 'Order'));
    expect(screen.getByTestId('connect-proposal-form')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('cancel-connection'));
    expect(useEditorStore.getState().model.relationships).toHaveLength(1);
    expect(useEditorStore.getState().isDirty).toBe(false);
  });

  it('a committed draw-to-connect adds an edge and serializes the relationship', () => {
    useEditorStore.getState().hydrate({ model: threeEntityModel(true), etag: 'e1' });
    render(<ERDiagramView />);

    act(() => useEditorStore.getState().proposeRelationship('Customer', 'Product'));
    const form = screen.getByTestId('connect-proposal-form');
    const opt = within(form)
      .getAllByTestId('cardinality-option')
      .find((e) => e.getAttribute('data-value') === 'one-to-many')!;
    fireEvent.click(opt);
    fireEvent.click(within(form).getByTestId('commit-connection'));

    expect(screen.getAllByTestId('relationship-edge')).toHaveLength(2);
    const names = (useEditorStore.getState().model.relationships ?? []).map(
      (r) => r.name,
    );
    expect(names).toContain('customer-products');
  });

  it('a draw-to-connect self-loop is a legal self-referential relationship', () => {
    useEditorStore.getState().hydrate({ model: selfRefModel(), etag: 'e1' });
    render(<ERDiagramView />);

    act(() => useEditorStore.getState().proposeRelationship('Employee', 'Employee'));
    const form = screen.getByTestId('connect-proposal-form');
    fireEvent.change(within(form).getByTestId('edit-name'), {
      target: { value: 'reports-to' },
    });
    const opt = within(form)
      .getAllByTestId('cardinality-option')
      .find((e) => e.getAttribute('data-value') === 'many-to-one')!;
    fireEvent.click(opt);
    fireEvent.click(within(form).getByTestId('commit-connection'));

    const rel = (useEditorStore.getState().model.relationships ?? [])[0];
    expect(rel).toMatchObject({ name: 'reports-to', from: 'Employee', to: 'Employee' });
  });

  it('a draw-to-connect on an already-related pair degrades to duplicate-name rejection', () => {
    useEditorStore.getState().hydrate({ model: threeEntityModel(true), etag: 'e1' });
    render(<ERDiagramView />);

    act(() => useEditorStore.getState().proposeRelationship('Customer', 'Order'));
    const form = screen.getByTestId('connect-proposal-form');
    expect(within(form).getByTestId('duplicate-name-message')).toBeInTheDocument();

    fireEvent.change(within(form).getByTestId('edit-name'), {
      target: { value: 'customer-returns' },
    });
    fireEvent.click(within(form).getByTestId('commit-connection'));

    expect(screen.getAllByTestId('relationship-edge')).toHaveLength(2);
  });

  it('an edit in one view is reflected in the other from a single draft mutation', () => {
    useEditorStore.getState().hydrate({ model: relatedModel(), etag: 'e1' });
    render(<ERDiagramView />);

    clickByData('click-node', 'data-node', 'Order');
    act(() => {
      const next = renameEntityInModel(
        useEditorStore.getState().model,
        'Order',
        'Invoice',
      );
      useEditorStore.getState().applyModel(next);
    });

    const entityNames = useEditorStore
      .getState()
      .model.entities.map((e) => e.name);
    expect(entityNames).toContain('Invoice');
    expect(useEditorStore.getState().model.relationships![0].to).toBe('Invoice');
    // The node label follows the same draft.
    expect(
      screen
        .getAllByTestId('entity-node')
        .some((el) => el.getAttribute('data-entity') === 'Invoice'),
    ).toBe(true);
  });

  it('a ref-typed field renders inside its node, not as an edge', () => {
    useEditorStore.getState().hydrate({ model: refFieldModel(), etag: 'e1' });
    render(<ERDiagramView />);
    expect(screen.getAllByTestId('field-badge').length).toBeGreaterThan(0);
    expect(screen.queryAllByTestId('relationship-edge')).toHaveLength(0);
  });

  it('is usable at small scale — a one-entity model renders one node', () => {
    useEditorStore.getState().hydrate({ model: oneEntityModel(), etag: 'e1' });
    render(<ERDiagramView />);
    expect(screen.getAllByTestId('entity-node')).toHaveLength(1);
  });
});
