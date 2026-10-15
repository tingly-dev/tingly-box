// Projects (frontend/packages/projects) mounted inside Tingly Box. The route
// table lazy-loads this module, so the package, its strings and everything
// they import stay out of the eager bundle. See .design/projects.md §3.
import { ProjectPage, ProjectsHostProvider, ProjectsPage, type ProjectsHost } from '@tingly/projects';
import { en as projectsEn } from '@tingly/projects/locales/en';
import { zh as projectsZh } from '@tingly/projects/locales/zh';
import i18n from '@/i18n';
import { notify } from '@/utils/notify';
import { notifyProjectsChanged, projectsFetch } from '@/services/projectsApi';

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

const host: ProjectsHost = {
    fetch: projectsFetch,
    paths: { list: '/projects', project: (id) => `/projects/${id}` },
    // The rail's waiting dot is the nav that reflects projects.
    onProjectsChanged: notifyProjectsChanged,
    notify: (message, severity = 'info') => {
        notify.show(severity, message);
    },
    embedded: true,
};

const ProjectsRoute = ({ page }: { page: 'list' | 'project' }) => (
    <ProjectsHostProvider host={host}>
        {page === 'list' ? <ProjectsPage /> : <ProjectPage />}
    </ProjectsHostProvider>
);

export default ProjectsRoute;
