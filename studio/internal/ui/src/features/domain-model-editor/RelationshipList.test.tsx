// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: relationship-list
// parlay-artifact: test
import { render, screen, fireEvent } from '@testing-library/react';
import { RelationshipList } from './RelationshipList';
import { useEditorStore } from '../../store/editorStore';
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

function oneEntityModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [{ name: 'Customer', fields: [] }],
    relationships: [],
  };
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('RelationshipList', () => {
  it('renders one row per declared relationship', () => {
    useEditorStore.getState().hydrate({ model: relatedModel(), etag: 'e1' });
    render(<RelationshipList />);
    expect(screen.getAllByTestId('relationship-rows')).toHaveLength(1);
  });

  it('shows the empty-state when the model has no relationships', () => {
    useEditorStore.getState().hydrate({ model: oneEntityModel(), etag: 'e1' });
    render(<RelationshipList />);
    expect(screen.getByTestId('empty-relationships')).toBeInTheDocument();
    expect(screen.queryByTestId('relationship-rows')).not.toBeInTheDocument();
  });

  it('deletes immediately — no dependent check', () => {
    useEditorStore.getState().hydrate({ model: relatedModel(), etag: 'e1' });
    render(<RelationshipList />);

    const del = screen
      .getAllByTestId('delete-relationship')
      .find((el) => el.getAttribute('data-relationship') === 'customer-orders')!;
    fireEvent.click(del);

    const relationships =
      useEditorStore.getState().model.relationships ?? [];
    expect(relationships.some((r) => r.name === 'customer-orders')).toBe(false);
    expect(relationships).toHaveLength(0);
  });
});
