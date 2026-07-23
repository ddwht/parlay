// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: save-bar
// parlay-artifact: test
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { SaveBar } from './SaveBar';
import { useEditorStore } from '../store/editorStore';
import { populatedModel, populatedEnvelope, clone } from '../test/fixtures';

function stubFetch(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ ok: true, status: 200, json: async () => body })),
  );
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
  useEditorStore.getState().hydrate(clone(populatedEnvelope));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('SaveBar', () => {
  it('is disabled when clean, enables on a dirty edit, and returns to the last-saved time after a save', async () => {
    stubFetch({ model: populatedModel, etag: 'sha256:saved' });
    render(<SaveBar />);

    expect(screen.getByTestId('save-button')).toBeDisabled();

    // An uncommitted edit flips the store to dirty.
    act(() => {
      useEditorStore.getState().applyModel(clone(populatedModel));
    });
    expect(screen.getByTestId('dirty-indicator')).toBeInTheDocument();
    expect(screen.getByTestId('save-button')).toBeEnabled();

    fireEvent.click(screen.getByTestId('save-button'));

    await waitFor(() =>
      expect(screen.getByTestId('last-saved-time')).toBeInTheDocument(),
    );
    expect(screen.queryByTestId('dirty-indicator')).not.toBeInTheDocument();
  });
});
