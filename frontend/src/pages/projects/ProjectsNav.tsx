// The Projects panel's sidebar: @tingly/projects' ProjectsSidebar (the user's
// projects and working folders) with Tingly Box's host wiring.
// Lazy-loaded by the nav (useActivityItems), like the Projects pages.
import { ProjectsHostProvider, ProjectsSidebar } from '@tingly/projects';
import { projectsHost } from './projectsHost';

interface ProjectsNavProps {
    onNavigate?: () => void;
}

const ProjectsNav = ({ onNavigate }: ProjectsNavProps) => (
    <ProjectsHostProvider host={projectsHost}>
        <ProjectsSidebar showProjects showWorkdirs onNavigate={onNavigate} />
    </ProjectsHostProvider>
);

export default ProjectsNav;
