// The Projects panel's sidebar: @tingly/projects' ProjectsSidebar (the user's
// projects and working folders) with Tingly Box's host wiring.
// Lazy-loaded by the nav (useActivityItems), like the Projects pages.
import { NotificationsBridge, ProjectsHostProvider, ProjectsSidebar } from '@tingly/projects';
import { projectsHost } from './projectsHost';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Robot } from '@/components/icons';

interface ProjectsNavProps {
    onNavigate?: () => void;
}

const ProjectsNav = ({ onNavigate }: ProjectsNavProps) => {
    const { pathname } = useLocation();
    const { t } = useTranslation();
    const imRows = [
        { key: 'bots', label: t('layout.imBots', { defaultValue: 'IM Bots' }), to: '/bots/overview', icon: <Robot />, selected: pathname.startsWith('/bots') },
    ];
    return (
    <ProjectsHostProvider host={projectsHost}>
        <NotificationsBridge />
        <ProjectsSidebar showProjects showWorkdirs imRows={imRows} onNavigate={onNavigate} />
    </ProjectsHostProvider>
);
};

export default ProjectsNav;
