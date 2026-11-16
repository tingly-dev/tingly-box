import { renderHook, act } from '@testing-library/react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { Rule } from '@/components/RoutingGraphTypes.ts';
import { useTemplatePageRules } from './useTemplatePageRules';

const { createRule } = vi.hoisted(() => ({ createRule: vi.fn() }));
vi.mock('@/services/api.ts', () => ({ api: { createRule } }));

const created = (body: any, uuid: string) => ({
    success: true,
    data: { ...body, uuid },
});

describe('useTemplatePageRules.handleCreateRules', () => {
    beforeEach(() => {
        createRule.mockReset();
    });

    it('creates one rule per model with unique names and reloads once', async () => {
        let n = 0;
        createRule.mockImplementation(async (_uuid: string, body: any) => created(body, `r${++n}`));
        const loadRules = vi.fn(async () => {});
        const showNotification = vi.fn();
        const rules = [{ uuid: 'old', request_model: 'gpt-5' }] as unknown as Rule[];

        const { result } = renderHook(() => useTemplatePageRules({
            rules, showNotification, scenario: 'claude_code', loadRules,
        }));

        let uuids: string[] = [];
        await act(async () => {
            uuids = await result.current.handleCreateRules([
                { providerUuid: 'p1', model: 'gpt-5' },
                { providerUuid: 'p1', model: 'gpt-5-mini' },
                { providerUuid: 'p2', model: 'gpt-5-mini' },
            ]);
        });

        expect(uuids).toEqual(['r1', 'r2', 'r3']);
        const names = createRule.mock.calls.map(([, body]) => body.request_model);
        expect(names[0]).toMatch(/^gpt-5-[0-9a-f]{6}$/);
        expect(names[1]).toBe('gpt-5-mini');
        expect(names[2]).toMatch(/^gpt-5-mini-[0-9a-f]{6}$/);
        expect(createRule.mock.calls[2][1].services[0]).toMatchObject({ provider: 'p2', model: 'gpt-5-mini' });
        expect(loadRules).toHaveBeenCalledTimes(1);
        expect(showNotification).toHaveBeenCalledWith('Added 3 models', 'success');
    });

    it('keeps going past a failure and reports it', async () => {
        createRule.mockImplementation(async (_uuid: string, body: any) =>
            body.request_model === 'bad'
                ? { success: false, error: 'boom' }
                : created(body, `ok-${body.request_model}`));
        const showNotification = vi.fn();

        const { result } = renderHook(() => useTemplatePageRules({
            rules: [], showNotification, scenario: 'claude_code', loadRules: vi.fn(async () => {}),
        }));

        let uuids: string[] = [];
        await act(async () => {
            uuids = await result.current.handleCreateRules([
                { providerUuid: 'p', model: 'bad' },
                { providerUuid: 'p', model: 'good' },
            ]);
        });

        expect(uuids).toEqual(['ok-good']);
        expect(showNotification).toHaveBeenCalledWith('Added 1 of 2 models. Failed: bad: boom', 'warning');
    });
});
