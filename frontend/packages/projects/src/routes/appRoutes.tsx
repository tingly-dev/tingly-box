import { lazy } from 'react';
import { Navigate, type RouteObject } from 'react-router-dom';
import { Layout } from '../layout/Layout';

// Every page is lazy (same rule as tingly-box's frontend/CLAUDE.md).
const HomePage = lazy(() => import('../pages/HomePage'));
const ChatPage = lazy(() => import('../pages/ChatPage'));
const ChatsPage = lazy(() => import('../pages/ChatsPage'));
const ProjectsPage = lazy(() => import('../pages/ProjectsPage'));
const ProjectPage = lazy(() => import('../pages/ProjectPage'));
const WorkdirPage = lazy(() => import('../pages/WorkdirPage'));
const SettingsPage = lazy(() => import('../pages/SettingsPage'));

export const appRoutes: RouteObject[] = [
    {
        element: <Layout />,
        children: [
            { path: '/', element: <ProjectsPage /> },
            { path: '/projects', element: <Navigate to="/" replace /> },
            { path: '/chat', element: <HomePage /> },
            { path: '/chat/:id', element: <ChatPage /> },
            { path: '/chats', element: <ChatsPage /> },
            { path: '/project/:id', element: <ProjectPage /> },
            { path: '/folder', element: <WorkdirPage /> },
            { path: '/settings', element: <SettingsPage /> },
            { path: '*', element: <Navigate to="/" replace /> },
        ],
    },
];
