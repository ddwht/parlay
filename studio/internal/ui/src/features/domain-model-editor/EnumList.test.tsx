// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: enum-list
// parlay-artifact: test
import { render, screen, fireEvent } from '@testing-library/react';
import { EnumList } from './EnumList';
import { useEditorStore } from '../../store/editorStore';
import { populatedEnvelope, clone } from '../../test/fixtures';

beforeEach(() => {
  useEditorStore.getState().resetStore();
  useEditorStore.getState().hydrate(clone(populatedEnvelope));
});

describe('EnumList', () => {
  it('renders one row per enum', () => {
    render(<EnumList />);
    expect(screen.getAllByTestId('enum-rows')).toHaveLength(1);
  });

  it('rejects a new enum name that collides with a built-in scalar type', () => {
    render(<EnumList />);
    fireEvent.change(screen.getByTestId('new-enum-name'), {
      target: { value: 'string' },
    });
    fireEvent.click(screen.getByTestId('new-enum'));

    expect(screen.getByTestId('new-enum-error')).toHaveTextContent(
      'is a built-in field type name',
    );
    // No enum was created.
    expect(useEditorStore.getState().model.enums).toHaveLength(1);
  });
});
