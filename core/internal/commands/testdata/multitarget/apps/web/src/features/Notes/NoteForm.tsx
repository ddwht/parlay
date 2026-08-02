// parlay-feature: notes
// parlay-component: note-form
//
// Presentation target (react-antd adapter, root apps/web). Widget: Form.
// action submit -> call @notes/operation:notes.create-note (POST /notes on the
// application target). Ant Design Form per the adapter's shows/actions mapping.

import { Button, Form, Input } from 'antd';

export interface CreateNoteInput {
  title: string;
  body: string;
}

export interface NoteFormProps {
  onCreated?: () => void;
}

export function NoteForm({ onCreated }: NoteFormProps) {
  const [form] = Form.useForm<CreateNoteInput>();

  // action: submit — effect: call — target: @notes/operation:notes.create-note
  const submit = async (values: CreateNoteInput) => {
    await fetch('/notes', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(values),
    });
    form.resetFields();
    onCreated?.();
  };

  return (
    <Form form={form} layout="vertical" onFinish={submit}>
      <Form.Item name="title" label="Title" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="body" label="Body" rules={[{ required: true }]}>
        <Input.TextArea />
      </Form.Item>
      <Button type="primary" htmlType="submit">
        Create note
      </Button>
    </Form>
  );
}
