// What a host app (Tingly Box) mounts to embed Projects. The host wraps the
// pages in ProjectsHostProvider, which tells them where they are routed and
// which fetch carries its authentication. Packaging (exports, peer
// dependencies, i18n namespace) lands with the host integration.
export { ProjectsHostProvider, type ProjectsHost } from './host/ProjectsHost';
export { default as ProjectsPage } from './pages/ProjectsPage';
export { default as ProjectPage } from './pages/ProjectPage';
