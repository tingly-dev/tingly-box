// Projects (frontend/packages/projects) mounted inside Tingly Box. The route
// table lazy-loads this module. See .design/projects.md §3.
import { ChatSessionPage, ImSettingsPage, NotificationsBridge, ProjectPage, ProjectsHostProvider, ProjectsPage, WorkdirPage, WorkspaceChatsPage, WorkspaceHomePage } from '@tingly/projects';
import { projectsHost } from './projectsHost';

const ProjectsRoute = ({ page }: { page: 'home' | 'list' | 'project' | 'folder' | 'im' | 'chats' | 'chat' }) => (
    <ProjectsHostProvider host={projectsHost}>
        {/* Hands the browser-notification engine this app's strings and routes. */}
        <NotificationsBridge />
        {page === 'home' ? <WorkspaceHomePage />
            : page === 'list' ? <ProjectsPage />
            : page === 'folder' ? <WorkdirPage />
            : page === 'chats' ? <WorkspaceChatsPage />
            : page === 'chat' ? <ChatSessionPage />
            : page === 'im' ? <ImSettingsPage setupPath="/bots/overview" />
            : <ProjectPage />}
    </ProjectsHostProvider>
);

export default ProjectsRoute;
