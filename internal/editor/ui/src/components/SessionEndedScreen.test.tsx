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

  // The command has to be one that exists. This test used to assert
  // 'parlay-studio', pinning a string that was wrong twice over: that binary
  // is retired to a redirect stub which exits non-zero, and its subcommand
  // form `domain-model edit` never existed in either binary. So the test was
  // holding the terminal error screen's single actionable instruction to a
  // command that errors.
  it('shows a restart command that actually exists', () => {
    act(() => useEditorStore.setState({ sessionEnded: true }));
    render(<SessionEndedScreen />);

    expect(screen.getByTestId('session-ended-message')).toBeInTheDocument();
    expect(screen.getByTestId('restart-command')).toHaveTextContent(
      'parlay domain-edit',
    );
    expect(screen.getByTestId('restart-command')).not.toHaveTextContent(
      'parlay-studio',
    );
  });

  // Ending the session deliberately and losing the server are different
  // events and must not share wording. The screen used to assert the server
  // was "no longer reachable" in both cases, which is false on every Done
  // path — the shutdown is one we requested and the process is exiting
  // normally.
  it('does not claim the server is unreachable when the user ended the session', () => {
    act(() =>
      useEditorStore.setState({ sessionEnded: true, sessionEndReason: 'done' }),
    );
    render(<SessionEndedScreen />);

    const message = screen.getByTestId('session-ended-message');
    expect(message).not.toHaveTextContent('no longer reachable');
    expect(message).toHaveTextContent('You ended this session');
  });

  it('says the server is unreachable when it went away underneath us', () => {
    act(() =>
      useEditorStore.setState({
        sessionEnded: true,
        sessionEndReason: 'unreachable',
      }),
    );
    render(<SessionEndedScreen />);

    expect(screen.getByTestId('session-ended-message')).toHaveTextContent(
      'no longer reachable',
    );
  });
});
