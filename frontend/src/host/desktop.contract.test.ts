// The desktop bridge calls Go methods by name (desktop.ts BOUND_METHODS), so
// nothing at compile time notices a rename on the Go side — the call would
// just fail at runtime inside the desktop window. This pins each name to the
// Go source. Read through Vite (?raw), same as routes.contract.test.tsx.
import { describe, expect, it } from 'vitest';
import tinglyServiceGo from '../../../gui/wails3/services/tingly_service.go?raw';
import { BOUND_METHODS, SAVE_FILE_ROUTE, TINGLY_SERVICE } from './desktop';

describe('desktop bridge contract', () => {
    it('names the Go service by its package path', () => {
        expect(tinglyServiceGo).toMatch(/^package services$/m);
        expect(tinglyServiceGo).toMatch(/^type TinglyService struct/m);
        expect(TINGLY_SERVICE).toBe('github.com/tingly-dev/tingly-box/gui/wails3/services.TinglyService');
    });

    it.each(Object.values(BOUND_METHODS))('%s is an exported method of TinglyService', (fqn) => {
        const method = fqn.slice(TINGLY_SERVICE.length + 1);
        expect(tinglyServiceGo).toMatch(new RegExp(`^func \\(s \\*TinglyService\\) ${method}\\(`, 'm'));
    });

    it('posts saves to the route the service registers', () => {
        expect(tinglyServiceGo).toContain(`POST("${SAVE_FILE_ROUTE}"`);
    });
});
