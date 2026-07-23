// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: entity-list
// parlay-artifact: test
import { render, screen, fireEvent } from '@testing-library/react';
import { EntityList } from './EntityList';
import { useEditorStore } from '../../store/editorStore';
import { populatedEnvelope, emptyEnvelope, clone } from '../../test/fixtures';

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('EntityList', () => {
  it('renders one row per entity', () => {
    useEditorStore.getState().hydrate(clone(populatedEnvelope));
    render(<EntityList />);
    expect(screen.getAllByTestId('entity-rows')).toHaveLength(2);
  });

  it('shows the empty-state entry point for a fresh project', () => {
    useEditorStore.getState().hydrate(clone(emptyEnvelope));
    render(<EntityList />);
    expect(screen.getByTestId('empty-entities')).toBeInTheDocument();
    expect(screen.queryByTestId('entity-rows')).not.toBeInTheDocument();
  });

  it('blocks deleting an entity that is still referenced, naming the referent', () => {
    useEditorStore.getState().hydrate(clone(populatedEnvelope));
    render(<EntityList />);
    // Customer is referenced by Order.customer_id.
    const customerDelete = screen
      .getAllByTestId('delete-entity')
      .find((el) => el.getAttribute('data-entity') === 'Customer')!;
    fireEvent.click(customerDelete);

    const msg = screen.getByTestId('delete-blocked-message');
    expect(msg).toHaveTextContent("Customer can't be deleted");
    expect(msg).toHaveTextContent('Order.customer_id');
    // Nothing was removed.
    expect(useEditorStore.getState().model.entities).toHaveLength(2);
  });
});
