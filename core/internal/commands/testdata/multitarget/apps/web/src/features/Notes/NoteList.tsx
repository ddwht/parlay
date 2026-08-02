// parlay-feature: notes
// parlay-component: note-list
//
// Presentation target (react-antd adapter, root apps/web). Widget: Table.
// action refresh -> call @notes/operation:notes.list-notes (GET /notes on the
// application target). Ant Design Table per the adapter's shows mapping.

import { useEffect, useState } from 'react';
import { Table } from 'antd';

export interface Note {
  id: string;
  title: string;
  body: string;
  createdAt: string;
}

export function NoteList() {
  const [notes, setNotes] = useState<Note[]>([]);

  // action: refresh — effect: call — target: @notes/operation:notes.list-notes
  const refresh = async () => {
    const res = await fetch('/notes');
    setNotes((await res.json()) as Note[]);
  };

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <Table<Note>
      rowKey="id"
      dataSource={notes}
      columns={[
        { title: 'Title', dataIndex: 'title' },
        { title: 'Body', dataIndex: 'body' },
        { title: 'Created', dataIndex: 'createdAt' },
      ]}
    />
  );
}
