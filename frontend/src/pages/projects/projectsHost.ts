// What Tingly Box hands @tingly/projects: its fetch, routes and toasts, and
// the package's strings. Shared by the Projects pages (ProjectsRoute) and the
// Projects sidebar (ProjectsNav); both are lazy, so the package, its strings
// and everything they import stay out of the eager bundle.
import type { ProjectsHost } from '@tingly/projects';
import { en as projectsEn } from '@tingly/projects/locales/en';
import { zh as projectsZh } from '@tingly/projects/locales/zh';
import i18n from '@/i18n';
import { notify } from '@/utils/notify';
import { notifyProjectsChanged, projectsFetch } from '@/services/projectsApi';
import { sessionSlots } from './session/sessionSlots';

// The package's own strings live in the `projects` namespace, apart from
// Tingly Box's (they share key names such as projects.*, common.*, nav.*).
// Added once, when this chunk first loads and before anything renders. Only
// en and zh exist; Russian falls back to English like any missing key.
// (The transcript inside the thread view uses projects.transcript.* in the
// default namespace, which ships with Tingly Box's own bundles.)
for (const [lng, bundle] of [['en', projectsEn], ['zh', projectsZh]] as const) {
    if (!i18n.hasResourceBundle(lng, 'projects')) {
        i18n.addResourceBundle(lng, 'projects', bundle, true, true);
    }
}

export const projectsHost: ProjectsHost = {
    fetch: projectsFetch,
    paths: {
        list: '/projects',
        project: (id) => `/projects/${id}`,
        folder: (path) => (path ? `/projects/folder?path=${encodeURIComponent(path)}` : '/projects/folder'),
    },
    // The rail's waiting dot is the nav that reflects projects.
    onProjectsChanged: notifyProjectsChanged,
    notify: (message, severity = 'info') => {
        notify.show(severity, message);
    },
    embedded: true,
    // Folder sessions run through Claude Code profiles and show their route.
    session: sessionSlots,
};
