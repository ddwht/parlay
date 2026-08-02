// parlay-feature: notes
// parlay-component: note-list
// parlay-artifact: test
//
// Presentation target test (react-antd, root apps/web).

import { render, screen } from '@testing-library/react';
import { NoteList } from './NoteList';

describe('NoteList', () => {
  it('renders the notes table headers', () => {
    render(<NoteList />);
    expect(screen.getByText('Title')).toBeInTheDocument();
    expect(screen.getByText('Body')).toBeInTheDocument();
    expect(screen.getByText('Created')).toBeInTheDocument();
  });
});
