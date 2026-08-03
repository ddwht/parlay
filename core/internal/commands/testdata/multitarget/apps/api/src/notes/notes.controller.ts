// parlay-feature: notes
// parlay-component: notes-controller
//
// Application target (nestjs-application adapter, root apps/api). Orchestrates
// the operation: the application layer owns validate-input (ValidationPipe) and
// authorize/auth-required (@UseGuards), plus shaping the return value; it
// delegates the persistence-owned create-one/read-many steps to NotesService,
// which calls Prisma. Per the adapter's policy/step decorator mappings:
// auth-required → @UseGuards(AuthGuard); validate-input → ValidationPipe.

import {
  Body,
  CanActivate,
  Controller,
  Get,
  Injectable,
  Post,
  UseGuards,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { CreateNoteInput, NotesService } from './notes.service';
import { Note } from '@prisma/client';

// auth-required policy. A real project supplies its own AuthGuard via the
// blueprint/adapter-set; this placeholder keeps the feature self-contained.
@Injectable()
export class AuthGuard implements CanActivate {
  canActivate(): boolean {
    return true;
  }
}

@Controller('notes')
export class NotesController {
  constructor(private readonly notes: NotesService) {}

  // @notes/operation:notes.create-note — http: POST /notes
  // owns (application): validate-input, return-one; delegates create-one → persistence
  @Post()
  @UseGuards(AuthGuard) // policy: auth-required
  @UsePipes(new ValidationPipe({ whitelist: true })) // step: validate-input
  create(@Body() input: CreateNoteInput): Promise<Note> {
    return this.notes.createNote(input);
  }

  // @notes/operation:notes.list-notes — http: GET /notes
  @Get()
  list(): Promise<Note[]> {
    return this.notes.listNotes();
  }
}
