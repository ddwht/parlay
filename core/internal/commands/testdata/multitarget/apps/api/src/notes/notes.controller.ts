// parlay-feature: notes
// parlay-component: notes-controller
//
// Application target (nestjs-application adapter, root apps/api). Exposes the
// operations over HTTP per targets.application.operations projection metadata
// (POST /notes, GET /notes) and delegates orchestration to NotesService.

import { Body, Controller, Get, Post } from '@nestjs/common';
import { CreateNoteInput, NotesService } from './notes.service';
import { Note } from '@prisma/client';

@Controller('notes')
export class NotesController {
  constructor(private readonly notes: NotesService) {}

  // @notes/operation:notes.create-note — http: POST /notes
  @Post()
  create(@Body() input: CreateNoteInput): Promise<Note> {
    return this.notes.createNote(input);
  }

  // @notes/operation:notes.list-notes — http: GET /notes
  @Get()
  list(): Promise<Note[]> {
    return this.notes.listNotes();
  }
}
