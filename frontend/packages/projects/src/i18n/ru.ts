import type { en } from './en';
import type { PluralDict } from './dict';
import { workspace } from './locales/ru/workspace';
import { session } from './locales/ru/session';
import { project } from './locales/ru/project';
import { settings } from './locales/ru/settings';

// Russian mirrors en key for key, plus its _few/_many plural forms.
export const ru: PluralDict<typeof en> = {
    ...workspace,
    ...session,
    ...project,
    ...settings,
};
