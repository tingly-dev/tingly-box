// The engine's strings and routes come from whichever surface is mounted:
// the standalone app's layout, or a host page (ProjectsRoute). Rendering
// this component hands both over; it shows nothing itself.
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useProjectsHost } from '../host/ProjectsHost';
import { setNotificationUi } from './engine';

export function NotificationsBridge(): null {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const { paths } = useProjectsHost();
    // Latest strings and router win: a language switch or a route change
    // re-renders this before any ping that would need the new ones.
    setNotificationUi({
        t: (key, values) => t(key, values as Record<string, string>),
        navigate: (to) => void navigate(to),
        paths: { project: paths.project, folder: paths.folder },
    });
    return null;
}
