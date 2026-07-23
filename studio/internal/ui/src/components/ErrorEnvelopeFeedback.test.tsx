// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: error-envelope-feedback
// parlay-artifact: test
import { render, screen, act } from '@testing-library/react';
import { ErrorEnvelopeFeedback } from './ErrorEnvelopeFeedback';
import { useEditorStore } from '../store/editorStore';

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('ErrorEnvelopeFeedback', () => {
  it('renders each validation-failed entry inline at its field, not only as a toast', () => {
    act(() =>
      useEditorStore.setState({
        fieldErrors: [
          { field: 'entities[0].name', message: 'must be unique' },
          { field: 'enums[0].values[1].value', message: 'is duplicated' },
        ],
      }),
    );
    render(<ErrorEnvelopeFeedback />);

    const inline = screen.getAllByTestId('inline-field-error');
    expect(inline).toHaveLength(2);
    expect(inline[0]).toHaveTextContent('entities[0].name');
  });

  it('shows a server-error toast carrying the request id for log correlation', () => {
    act(() =>
      useEditorStore.setState({ serverError: { requestId: 'xyz-456-pqr' } }),
    );
    render(<ErrorEnvelopeFeedback />);

    expect(screen.getByTestId('server-error-toast')).toHaveTextContent(
      'xyz-456-pqr',
    );
  });
});
