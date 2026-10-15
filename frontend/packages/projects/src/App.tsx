import type { ReactNode } from 'react';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { ThemeModeProvider } from './contexts/ThemeModeContext';
import { AppDataProvider, useAppData } from './contexts/AppDataContext';
import { ProjectsHostProvider, type ProjectsHost } from './host/ProjectsHost';
import { NotificationProvider } from './hooks/useNotify';
import { appRoutes } from './routes/appRoutes';

const router = createBrowserRouter(appRoutes);

const paths: ProjectsHost['paths'] = { list: '/', project: (id) => `/project/${id}` };

/** The standalone app is its own Projects host: browser fetch, its own routes and sidebar. */
function StandaloneHost({ children }: { children: ReactNode }) {
    const { refresh } = useAppData();
    return <ProjectsHostProvider host={{ paths, onProjectsChanged: refresh }}>{children}</ProjectsHostProvider>;
}

export default function App() {
    return (
        <ThemeModeProvider>
            <NotificationProvider>
                <AppDataProvider>
                    <StandaloneHost>
                        <RouterProvider router={router} />
                    </StandaloneHost>
                </AppDataProvider>
            </NotificationProvider>
        </ThemeModeProvider>
    );
}
