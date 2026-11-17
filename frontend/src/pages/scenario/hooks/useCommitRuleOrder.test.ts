import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Rule } from '@/components/RoutingGraphTypes.ts';
import { useCommitRuleOrder } from './useCommitRuleOrder';

const { reorderRules } = vi.hoisted(() => ({ reorderRules: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { reorderRules } }));

const rules = ['a', 'b', 'c'].map(uuid => ({ uuid }) as unknown as Rule);

const setup = () => {
    const onRulesChange = vi.fn();
    const loadRules = vi.fn(async () => {});
    const showNotification = vi.fn();
    const { result } = renderHook(() => useCommitRuleOrder({
        scenario: 'openai', rules, onRulesChange, loadRules, showNotification,
    }));
    return { commit: result.current, onRulesChange, loadRules, showNotification };
};

describe('useCommitRuleOrder', () => {
    beforeEach(() => reorderRules.mockReset());

    it('applies the order optimistically and persists the whole order', async () => {
        reorderRules.mockResolvedValue({ success: true });
        const { commit, onRulesChange, loadRules, showNotification } = setup();
        await act(async () => { await commit(['c', 'a', 'b']); });
        expect(onRulesChange).toHaveBeenCalledOnce();
        expect(onRulesChange.mock.calls[0][0].map((r: Rule) => r.uuid)).toEqual(['c', 'a', 'b']);
        expect(reorderRules).toHaveBeenCalledWith('openai', ['c', 'a', 'b']);
        expect(loadRules).not.toHaveBeenCalled();
        expect(showNotification).toHaveBeenCalledOnce();
        expect(showNotification).toHaveBeenCalledWith('templatePage.reorder.savedPlain', 'success');
    });

    it('names the moved rule and its new position in the success notice', async () => {
        reorderRules.mockResolvedValue({ success: true });
        const { commit, showNotification } = setup();
        await act(async () => { await commit(['c', 'a', 'b'], 'c'); });
        expect(showNotification).toHaveBeenCalledWith('templatePage.reorder.saved', 'success');
    });

    it('rolls back, toasts and reloads when the server rejects the order', async () => {
        reorderRules.mockResolvedValue({ success: false, error: 'nope' });
        const { commit, onRulesChange, loadRules, showNotification } = setup();
        await act(async () => { await commit(['c', 'a', 'b']); });
        expect(onRulesChange).toHaveBeenCalledTimes(2);
        expect(onRulesChange.mock.calls[1][0]).toBe(rules);
        expect(showNotification).toHaveBeenCalledWith(expect.stringContaining('saveFailed'), 'error');
        expect(showNotification).not.toHaveBeenCalledWith(expect.anything(), 'success');
        expect(loadRules).toHaveBeenCalledWith('openai');
    });

    it('ignores an order that no longer matches the list (a refetch changed it mid-drag)', async () => {
        const { commit, onRulesChange } = setup();
        await act(async () => { await commit(['a', 'b', 'gone']); });
        expect(onRulesChange).not.toHaveBeenCalled();
        expect(reorderRules).not.toHaveBeenCalled();
    });
});
