// The Projects panel's sidebar: @tingly/projects' ProjectsSidebar (the user's
// projects) with Desk's working folders passed in as a section. The package
// knows nothing about Desk; everything Desk-specific stays on this side.
// Lazy-loaded by the nav (useActivityItems), like the Projects pages.
import { useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { IconFolderCode } from '@tabler/icons-react';
import { ProjectsHostProvider, ProjectsSidebar, type ProjectsSidebarSection } from '@tingly/projects';
import { tablerMui } from '@/components/icons';
import AddProjectDialog from '@/components/desk/AddProjectDialog';
import { deskScope } from '@/components/desk/deskEvents';
import { folderName } from '@/components/desk/deskUtils';
import { useDeskProjects } from '@/components/desk/useDeskProjects';
import { useDeskWorkdirs } from '@/components/desk/useDeskWorkdirs';
import { useNotify } from '@/hooks/useNotify';
import { projectsHost } from './projectsHost';

const FolderCode = tablerMui(IconFolderCode);

const deskFolderPath = (path: string) => `/desk?folder=${encodeURIComponent(path)}`;

// The folder a working directory sits in, home shortened to ~.
const parentPath = (path: string) => path.replace(/[\\/]+$/, '').replace(/[\\/][^\\/]*$/, '').replace(/^\/(Users|home)\/[^/]+/, '~') || '/';

interface ProjectsNavProps {
    enableProjects: boolean;
    enableDesk: boolean;
    onNavigate?: () => void;
}

const ProjectsNav = ({ enableProjects, enableDesk, onNavigate }: ProjectsNavProps) => {
    const { t } = useTranslation();
    const notify = useNotify();
    const navigate = useNavigate();
    const location = useLocation();
    const [params] = useSearchParams();
    const { addProject } = useDeskProjects();
    const { workdirs, sessions } = useDeskWorkdirs(enableDesk);
    const [adding, setAdding] = useState(false);

    const scope = location.pathname === '/desk' ? deskScope(params, sessions) : undefined;
    const sections: ProjectsSidebarSection[] = enableDesk ? [{
        key: 'workdirs',
        title: t('layout.workingFolders'),
        to: '/desk',
        action: { label: t('desk.addProject'), onClick: () => setAdding(true) },
        empty: t('layout.noWorkingFolders'),
        rows: workdirs.map((w) => ({
            key: w.path,
            label: folderName(w.path),
            title: w.path,
            icon: <FolderCode />,
            // What waits here, else where the folder is.
            subtitle: w.waiting > 0 ? t('layout.workingFolderWaiting', { count: w.waiting }) : parentPath(w.path),
            to: deskFolderPath(w.path),
            selected: scope === w.path,
            attention: w.waiting > 0,
        })),
    }] : [];

    return (
        <ProjectsHostProvider host={projectsHost}>
            <ProjectsSidebar showProjects={enableProjects} showWorkdirs={enableProjects && !enableDesk} sections={sections} onNavigate={onNavigate} />
            {adding && (
                <AddProjectDialog
                    onClose={() => setAdding(false)}
                    onAdd={(path) => {
                        if (!addProject(path)) {
                            notify.error(t('desk.projectSaveFailed', { defaultValue: 'Could not save the working folder in this browser. Please try again.' }));
                            return false;
                        }
                        setAdding(false);
                        navigate(deskFolderPath(path));
                        onNavigate?.();
                        return true;
                    }}
                />
            )}
        </ProjectsHostProvider>
    );
};

export default ProjectsNav;
