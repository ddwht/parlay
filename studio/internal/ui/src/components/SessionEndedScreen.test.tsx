// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: session-ended-screen
// parlay-artifact: test
import { render, screen, act } from '@testing-library/react';
import { SessionEndedScreen } from './SessionEndedScreen';
import { useEditorStore } from '../store/editorStore';

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('SessionEndedScreen', () => {
  it('is hidden while the session is live', () => {
    render(<SessionEndedScreen />);
    expect(screen.queryByTestId('session-ended-message')).not.toBeInTheDocument();
  });

  it('shows the restart command and stops once the session has ended', () => {
    act(() => useEditorStore.setState({ sessionEnded: true }));
    render(<SessionEndedScreen />);

    expect(screen.getByTestId('session-ended-message')).toBeInTheDocument();
    expect(screen.getByTestId('restart-command')).toHaveTextContent(
      'parlay-studio',
    );
  });
});
