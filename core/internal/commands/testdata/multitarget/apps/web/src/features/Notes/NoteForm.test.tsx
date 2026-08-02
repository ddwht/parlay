// parlay-feature: notes
// parlay-component: note-form
// parlay-artifact: test
//
// Presentation target test (react-antd, root apps/web).

import { render, screen } from '@testing-library/react';
import { NoteForm } from './NoteForm';

describe('NoteForm', () => {
  it('renders the create-note form fields', () => {
    render(<NoteForm />);
    expect(screen.getByLabelText('Title')).toBeInTheDocument();
    expect(screen.getByLabelText('Body')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /create note/i })).toBeInTheDocument();
  });
});
