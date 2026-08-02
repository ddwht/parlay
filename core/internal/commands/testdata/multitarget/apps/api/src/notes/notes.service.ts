// parlay-feature: notes
// parlay-component: notes-service
//
// Application target (nestjs-application adapter, root apps/api). Orchestrates
// the capability operations' steps into Prisma calls: create-one/read-many map
// to prisma.note.create / prisma.note.findMany per the adapter's
// step-to-prisma-mapping convention.

import { Injectable } from '@nestjs/common';
import { PrismaClient, Note } from '@prisma/client';

export interface CreateNoteInput {
  title: string;
  body: string;
}

@Injectable()
export class NotesService {
  private readonly prisma = new PrismaClient();

  // operation: @notes/operation:notes.create-note (command)
  // steps: create-one -> return-one
  async createNote(input: CreateNoteInput): Promise<Note> {
    return this.prisma.note.create({ data: input });
  }

  // operation: @notes/operation:notes.list-notes (query)
  // steps: read-many -> return-many
  async listNotes(): Promise<Note[]> {
    return this.prisma.note.findMany({ orderBy: { createdAt: 'desc' } });
  }
}
