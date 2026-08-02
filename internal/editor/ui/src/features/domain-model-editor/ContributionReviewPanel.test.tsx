// parlay-feature: domain-model-editor/feature-contributions
// parlay-component: contribution-review-panel
// parlay-artifact: test
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ContributionReviewPanel } from './ContributionReviewPanel';

function stubContribution(body: unknown, ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ ok, status: ok ? 200 : 400, json: async () => body })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ContributionReviewPanel', () => {
  // An ordinary editing session must look exactly as it did before
  // contributions existed. The endpoint distinguishes "not reviewing
  // anything" from "the review is empty" so the panel can stay silent here
  // rather than rendering an empty review.
  it('renders nothing when the session is not reviewing a contribution', async () => {
    stubContribution({ present: false });
    render(<ContributionReviewPanel />);
    await waitFor(() => {
      expect(screen.queryByTestId('contribution-panel')).toBeNull();
    });
  });

  it('names the proposing feature and lists the additions', async () => {
    stubContribution({
      present: true,
      feature: 'submit-expense',
      path: 'spec/intents/submit-expense/domain-model.yaml',
      delta: {
        additions: [
          {
            kind: 'field',
            path: 'entities.ExpenseReport.fields.settledAt',
            entity: 'ExpenseReport',
            name: 'settledAt',
            summary: 'new field on existing entity ExpenseReport: type: datetime',
          },
        ],
        conflicts: [],
        redundant: [],
      },
    });

    render(<ContributionReviewPanel />);

    expect(await screen.findByTestId('contribution-panel')).toBeInTheDocument();
    expect(screen.getByTestId('contribution-feature')).toHaveTextContent('submit-expense');
    expect(screen.getAllByTestId('addition-entry')).toHaveLength(1);
    expect(screen.getByTestId('addition-entry')).toHaveTextContent(
      'entities.ExpenseReport.fields.settledAt',
    );
  });

  // Both descriptions have to be on screen. "These disagree" alone leaves the
  // reader to go and look up what the project model says, which is the lookup
  // this panel exists to save them.
  it('shows both descriptions for a conflict', async () => {
    stubContribution({
      present: true,
      feature: 'submit-expense',
      delta: {
        additions: [],
        conflicts: [
          {
            kind: 'field',
            path: 'entities.ExpenseReport.fields.total',
            entity: 'ExpenseReport',
            name: 'total',
            summary: 'described differently by the root model',
            root: 'type: float',
            proposed: 'type: string',
          },
        ],
        redundant: [],
      },
    });

    render(<ContributionReviewPanel />);

    const entry = await screen.findByTestId('conflict-entry');
    expect(entry).toHaveTextContent('type: float');
    expect(entry).toHaveTextContent('type: string');
  });

  it('says so when everything proposed is already in the model', async () => {
    stubContribution({
      present: true,
      feature: 'submit-expense',
      delta: {
        additions: [],
        conflicts: [],
        redundant: [
          {
            kind: 'field',
            path: 'entities.ExpenseReport.fields.id',
            name: 'id',
            summary: 'already declared identically',
          },
        ],
      },
    });

    render(<ContributionReviewPanel />);
    expect(await screen.findByTestId('contribution-nothing-new')).toBeInTheDocument();
  });

  it('can be hidden', async () => {
    stubContribution({
      present: true,
      feature: 'submit-expense',
      delta: { additions: [], conflicts: [], redundant: [] },
    });

    render(<ContributionReviewPanel />);
    await screen.findByTestId('contribution-panel');
    await userEvent.click(screen.getByTestId('dismiss-contribution'));
    expect(screen.queryByTestId('contribution-panel')).toBeNull();
  });

  // A failed review query must not take the editor down with it. The model
  // loaded through its own endpoint; a working editor beats a blank page.
  it('stays silent when the review query fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network down');
      }),
    );

    render(<ContributionReviewPanel />);
    await waitFor(() => {
      expect(screen.queryByTestId('contribution-panel')).toBeNull();
    });
  });

  // A null collection on the wire reaching a .map unmounts the page. The
  // client normalises at the one boundary every response passes through.
  it('survives null collections in the delta', async () => {
    stubContribution({
      present: true,
      feature: 'submit-expense',
      delta: { additions: null, conflicts: null, redundant: null },
    });

    render(<ContributionReviewPanel />);
    expect(await screen.findByTestId('contribution-panel')).toBeInTheDocument();
  });
});
