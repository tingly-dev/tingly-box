// The en dict is split by feature area to keep each file short (same layout
// as the host app's src/i18n/locales/<lang>/<area>.ts); this file only
// composes them. `en` is the key source of truth — zh mirrors it via
// Dict<typeof en>. Put every new key in the area file that owns it:
//   workspace.ts  landing page, nav/sidebar/rail, layout chrome, shared labels
//   session.ts    workdir sessions, composer/queue, model & effort, chats
//   project.ts    projects, threads, overview, library, project settings
//   settings.ts   global settings, IM binding, directory picker, workdirs
import { workspace } from './locales/en/workspace';
import { session } from './locales/en/session';
import { project } from './locales/en/project';
import { settings } from './locales/en/settings';

export const en = {
    ...workspace,
    ...session,
    ...project,
    ...settings,
};
