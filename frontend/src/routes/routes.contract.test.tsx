// Route contract: every path another surface hands to the router must land
// on a real page, not fall through to the `*` catch-all (which silently
// redirects to /agent). Surfaces checked here:
//   - the Wails tray and app menus (gui/wails3/routes.go), which navigate by
//     string and have no compiler to tell them a route moved;
//   - the tray hub panel's jumps into the main window (shellRoutes.ts);
//   - every <Navigate to=...> inside the route table itself, so a legacy
//     redirect can't point at a path that no longer exists.
import { isValidElement, type ReactNode } from 'react';
import { createRoutesFromElements, matchRoutes, Navigate, type RouteObject } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { appRoutes } from './appRoutes';
import { SHELL_ROUTES } from './shellRoutes';
// Read through Vite (?raw) rather than node:fs so the suite needs no
// @types/node, same as i18n/locales/tKeyCoverage.test.ts.
import appMenuGo from '../../../gui/wails3/appmenu.go?raw';
import routesGo from '../../../gui/wails3/routes.go?raw';
import runGo from '../../../gui/wails3/run.go?raw';
import systrayGo from '../../../gui/wails3/systray.go?raw';
import windowGo from '../../../gui/wails3/window.go?raw';

const routes = createRoutesFromElements(appRoutes);

function resolvesToPage(path: string): boolean {
    const pathname = path.split(/[?#]/)[0];
    const matches = matchRoutes(routes, pathname);
    if (!matches || matches.length === 0) return false;
    return matches[matches.length - 1].route.path !== '*';
}

// Collect the `to` of every <Navigate> reachable from a route element,
// including ones wrapped in gates like <ExperimentalFeatureGate>.
function navigateTargets(node: ReactNode, out: string[] = []): string[] {
    if (Array.isArray(node)) {
        node.forEach((n) => navigateTargets(n, out));
    } else if (isValidElement<{ to?: unknown; children?: ReactNode }>(node)) {
        if (node.type === Navigate && typeof node.props.to === 'string') out.push(node.props.to);
        navigateTargets(node.props.children, out);
    }
    return out;
}

function allNavigateTargets(rs: RouteObject[]): string[] {
    return rs.flatMap((r) => [...navigateTargets(r.element), ...allNavigateTargets(r.children ?? [])]);
}

describe('route contract', () => {
    it('resolves the paths the Wails tray navigates to', () => {
        const paths = [...routesGo.matchAll(/^\s*Route\w+\s*=\s*"([^"]+)"/gm)].map((m) => m[1]);
        expect(paths.length).toBeGreaterThan(0);
        expect(paths.filter((p) => !resolvesToPage(p))).toEqual([]);
    });

    it('keeps tray navigation on the shared route constants', () => {
        // A raw "/..." literal passed to the main-window navigation helpers
        // would bypass routes.go and therefore this contract.
        for (const src of [systrayGo, runGo, windowGo, appMenuGo]) {
            expect(src).not.toMatch(/showMainWindow\([^)]*"\//);
            expect(src).not.toMatch(/openMain\("\//);
        }
    });

    it('resolves the paths the tray hub panel jumps to', () => {
        const paths = Object.values(SHELL_ROUTES);
        expect(paths.filter((p) => !resolvesToPage(p))).toEqual([]);
    });

    it('points every redirect at a real page', () => {
        const targets = allNavigateTargets(routes);
        expect(targets.length).toBeGreaterThan(0);
        expect(targets.filter((t) => !resolvesToPage(t))).toEqual([]);
    });

    it('keeps both MCP tool usages on their own pages', () => {
        expect(
            ['/mcp/routes', '/mcp/tools', '/mcp/server-tools', '/mcp/sources', '/mcp/clients'].filter(
                (path) => !resolvesToPage(path)
            )
        ).toEqual([]);
        const legacy = matchRoutes(routes, '/tools/servertool')!.at(-1)!.route;
        expect(navigateTargets(legacy.element)).toEqual(['/mcp/server-tools']);
    });

    it('falls through to the catch-all for unknown paths', () => {
        // Guards the helper itself: if this ever passes, resolvesToPage is
        // not actually detecting the catch-all.
        expect(resolvesToPage('/agent/claude-code')).toBe(false);
        expect(resolvesToPage('/agent/claude_code')).toBe(true);
    });
});
