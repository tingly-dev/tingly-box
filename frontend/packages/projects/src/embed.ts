// What a host app (Tingly Box) mounts to embed Projects. The host wraps the
// pages in ProjectsHostProvider, which tells them where they are routed and
// which fetch carries its authentication. Their strings are in the i18next
// namespace `projects`: the host adds the bundles from
// `@tingly/projects/locales/{en,zh}` under that namespace before rendering.
export { ProjectsHostProvider, type ProjectsHost } from './host/ProjectsHost';
export { default as ProjectsPage } from './pages/ProjectsPage';
export { default as ProjectPage } from './pages/ProjectPage';
export {
    ProjectsSidebar,
    type ProjectsSidebarProps,
    type ProjectsSidebarRow,
    type ProjectsSidebarSection,
} from './components/ProjectsSidebar';
export { WorkspaceLayout, useWorkspacePane, type WorkspacePane } from './components/workspace/WorkspaceLayout';
export { Composer, type ComposerProps } from './components/Composer';
export { StatusGroups, StatusHeading, type StatusGroup, type StatusRow } from './components/workspace/StatusGroups';
export { timeAgo } from './utils/format';
