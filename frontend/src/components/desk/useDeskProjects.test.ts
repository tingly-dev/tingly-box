import {act, cleanup, renderHook} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {isAbsoluteProjectPath, normalizeProjectPath, PROJECTS_KEY, useDeskProjects} from './useDeskProjects';

afterEach(() => {cleanup(); localStorage.clear(); vi.restoreAllMocks();});

describe('Browser project shortcuts', () => {
    it('saves a project without a task and restores it after remount', () => {
        const first = renderHook(useDeskProjects);
        act(() => {expect(first.result.current.addProject(' /projects/new app/// ')).toBe(true);});
        act(() => {expect(first.result.current.addProject('/projects/new app')).toBe(true);});
        expect(first.result.current.projects).toEqual(['/projects/new app']);
        first.unmount();
        const second = renderHook(useDeskProjects);
        expect(second.result.current.projects).toEqual(['/projects/new app']);
        act(() => {expect(second.result.current.removeProject('/projects/new app')).toBe(true);});
        expect(second.result.current.projects).toEqual([]);
        expect(JSON.parse(localStorage.getItem(PROJECTS_KEY)!)).toEqual({});
    });

    it('updates when another tab changes the stored projects', () => {
        const {result} = renderHook(useDeskProjects);
        act(() => {
            localStorage.setItem(PROJECTS_KEY, JSON.stringify({'/projects/shared': true}));
            window.dispatchEvent(new StorageEvent('storage', {key: PROJECTS_KEY}));
        });
        expect(result.current.projects).toEqual(['/projects/shared']);
    });

    it('ignores unsupported stored paths and rejects relative paths', () => {
        localStorage.setItem(PROJECTS_KEY, JSON.stringify({'relative/app': true, '/valid/': true}));
        const {result} = renderHook(useDeskProjects);
        expect(result.current.projects).toEqual(['/valid']);
        act(() => {expect(result.current.addProject('../app')).toBe(false);});
        expect(result.current.projects).toEqual(['/valid']);
    });

    it('does not claim success when browser storage fails', () => {
        const {result} = renderHook(useDeskProjects);
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {throw new Error('quota');});
        vi.spyOn(console, 'error').mockImplementation(() => {});
        act(() => {expect(result.current.addProject('/projects/unsaved')).toBe(false);});
        expect(result.current.projects).toEqual([]);
    });

    it('accepts absolute POSIX, Windows and network paths while keeping filesystem roots', () => {
        for (const path of ['/', '/home/me/app', 'C:\\', 'D:/projects/app', '\\\\server\\share\\app']) {
            expect(isAbsoluteProjectPath(path)).toBe(true);
        }
        for (const path of ['', 'app', './app', '~/app', 'C:app', '\\app']) {
            expect(isAbsoluteProjectPath(path)).toBe(false);
        }
        expect(normalizeProjectPath(' / ')).toBe('/');
        expect(normalizeProjectPath(' C:\\ ')).toBe('C:\\');
        expect(normalizeProjectPath(' C:\\projects\\app\\ ')).toBe('C:\\projects\\app');
        expect(normalizeProjectPath('/projects/name\\')).toBe('/projects/name\\');
    });
});
