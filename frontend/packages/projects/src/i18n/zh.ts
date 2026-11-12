import type { en } from './en';
import type { Dict } from './dict';
import { workspace } from './locales/zh/workspace';
import { session } from './locales/zh/session';
import { project } from './locales/zh/project';
import { settings } from './locales/zh/settings';

// Each zh area file is typed Dict<typeof its en area>, and this annotation
// re-checks the composed dict as a whole, so en/zh stay key-for-key identical.
export const zh: Dict<typeof en> = {
    ...workspace,
    ...session,
    ...project,
    ...settings,
};
