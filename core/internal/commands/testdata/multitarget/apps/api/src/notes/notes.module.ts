// parlay-feature: notes
// parlay-component: notes-module
//
// Application target (nestjs-application adapter, root apps/api). One feature
// module per parlay feature; wires the controller and service and exports the
// service. apps/api/src/main.ts imports this module.

import { Module } from '@nestjs/common';
import { NotesController } from './notes.controller';
import { NotesService } from './notes.service';

@Module({
  controllers: [NotesController],
  providers: [NotesService],
  exports: [NotesService],
})
export class NotesModule {}
