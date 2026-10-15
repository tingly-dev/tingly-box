// Projects (frontend/packages/projects) mounted inside Tingly Box. The route
// table lazy-loads this module. See .design/projects.md §3.
import { ProjectPage, ProjectsHostProvider, ProjectsPage } from '@tingly/projects';
import { projectsHost } from './projectsHost';

const ProjectsRoute = ({ page }: { page: 'list' | 'project' }) => (
    <ProjectsHostProvider host={projectsHost}>
        {page === 'list' ? <ProjectsPage /> : <ProjectPage />}
    </ProjectsHostProvider>
);

export default ProjectsRoute;
