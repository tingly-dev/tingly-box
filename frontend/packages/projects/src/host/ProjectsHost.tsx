import { createContext, useContext, type ReactNode } from 'react';
import { setApiFetch } from '../client';

/**
 * What the Projects pages need from the app they run in. The standalone app
 * and an embedding host (Tingly Box) each provide one, so the pages never
 * assume where they are mounted or how requests are authenticated.
 */
export interface ProjectsHost {
    /**
     * fetch with the host's authentication. Every Projects request goes
     * through it, including the event stream and library downloads.
     * Defaults to the browser's fetch.
     */
    fetch?: typeof fetch;
    /** Where the Projects pages live in the host's router. */
    paths: {
        list: string;
        project: (id: string) => string;
    };
    /** A project was created, changed or deleted: refresh any navigation that lists projects. */
    onProjectsChanged?: () => void;
}

const Ctx = createContext<ProjectsHost | null>(null);

export function ProjectsHostProvider({ host, children }: { host: ProjectsHost; children: ReactNode }) {
    // The API client is module-level, so it learns the host's fetch here,
    // before any page below renders and requests anything.
    setApiFetch(host.fetch);
    return <Ctx.Provider value={host}>{children}</Ctx.Provider>;
}

const noop = () => {};

export function useProjectsHost(): ProjectsHost & { onProjectsChanged: () => void } {
    const host = useContext(Ctx);
    if (!host) throw new Error('useProjectsHost outside ProjectsHostProvider');
    return { ...host, onProjectsChanged: host.onProjectsChanged ?? noop };
}
