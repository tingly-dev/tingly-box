// Projects (frontend/packages/projects) mounted inside Tingly Box. The route
// table lazy-loads this module. See .design/projects.md §3.
import { ImSettingsPage, ProjectPage, ProjectsHostProvider, ProjectsPage, WorkdirPage } from '@tingly/projects';
import { projectsHost } from './projectsHost';

const ProjectsRoute = ({ page }: { page: 'list' | 'project' | 'folder' | 'im' }) => (
    <ProjectsHostProvider host={projectsHost}>
        {page === 'list' ? <ProjectsPage /> : page === 'folder' ? <WorkdirPage /> : page === 'im' ? <ImSettingsPage setupPath="/bots/overview" /> : <ProjectPage />}
    </ProjectsHostProvider>
);

export default ProjectsRoute;
