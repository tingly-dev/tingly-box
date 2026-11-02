// The Projects panel's sidebar: @tingly/projects' ProjectsSidebar (the user's
// projects and working folders) with Tingly Box's host wiring.
// Lazy-loaded by the nav (useActivityItems), like the Projects pages.
import { ProjectsHostProvider, ProjectsSidebar } from '@tingly/projects';
import { projectsHost } from './projectsHost';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Robot, Terminal, Bell } from '@/components/icons';

interface ProjectsNavProps {
    onNavigate?: () => void;
}

const ProjectsNav = ({ onNavigate }: ProjectsNavProps) => {
    const { pathname } = useLocation();
    const { t } = useTranslation();
    const imRows = [
        { key: 'bots', label: t('layout.imBots', { defaultValue: 'IM Bots' }), to: '/bots/overview', icon: <Robot />, selected: pathname.startsWith('/bots') },
        { key: 'remote', label: t('layout.remoteControl', { defaultValue: 'Remote Control' }), to: '/remote-agent', icon: <Terminal />, selected: pathname.startsWith('/remote-agent') },
        { key: 'notify', label: t('layout.notify', { defaultValue: 'IM Notify' }), to: '/notify', icon: <Bell />, selected: pathname.startsWith('/notify') },
    ];
    return (
    <ProjectsHostProvider host={projectsHost}>
        <ProjectsSidebar showProjects showWorkdirs imRows={imRows} onNavigate={onNavigate} />
    </ProjectsHostProvider>
);
};

export default ProjectsNav;
