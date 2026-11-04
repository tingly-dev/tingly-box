import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import LimitsEditor, {compactLimits, isEmptyLimits} from './LimitsEditor';
import {buildRuleUpdatePayload} from '@/components/rule-card/ruleUpdatePayload';
import type {ConfigRecord} from '@/components/RoutingGraphTypes';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({t: (key: string) => key}),
}));

describe('LimitsEditor', () => {
    it('emits parsed numbers and drops empty fields', () => {
        const onChange = vi.fn();
        render(<LimitsEditor value={{}} onChange={onChange}/>);
        fireEvent.change(screen.getByLabelText('limits.maxConcurrent'), {target: {value: '4'}});
        expect(onChange).toHaveBeenLastCalledWith({max_concurrent: 4});
        fireEvent.change(screen.getByLabelText('limits.maxTokens'), {target: {value: '1000000'}});
        expect(onChange).toHaveBeenLastCalledWith({max_tokens: 1000000});
    });

    it('rejects out-of-range steering thresholds and only shows quota for accounts', () => {
        const onChange = vi.fn();
        const {rerender} = render(<LimitsEditor value={{max_concurrent: 2}} onChange={onChange}/>);
        fireEvent.change(screen.getByLabelText('limits.softPercent'), {target: {value: '150'}});
        expect(onChange).toHaveBeenLastCalledWith({max_concurrent: 2, soft_percent: undefined});
        expect(screen.queryByLabelText('limits.quotaPercent')).toBeNull();
        rerender(<LimitsEditor value={{}} onChange={onChange} showQuota/>);
        expect(screen.getByLabelText('limits.quotaPercent')).toBeTruthy();
    });

    it('compacts to {} when everything is cleared (wire format for "clear limits")', () => {
        expect(compactLimits({max_concurrent: undefined, window_seconds: 3600})).toEqual({});
        expect(compactLimits({max_requests: 5, window_seconds: 600})).toEqual({max_requests: 5, window_seconds: 600});
        expect(isEmptyLimits({})).toBe(true);
        expect(isEmptyLimits({soft_percent: 80})).toBe(false);
    });
});

describe('rule update payload', () => {
    it('carries service limits through the full-replace PUT (omitting them would wipe them)', () => {
        const config = {
            uuid: 'r1', scenario: 'team', requestModel: 'm', responseModel: '', active: true, description: '',
            flags: {}, smartEnabled: false, smartRouting: [],
            providers: [{uuid: 's1', provider: 'p1', model: 'm1', tier: 0, limits: {max_concurrent: 2}}],
        } as unknown as ConfigRecord;
        const payload = buildRuleUpdatePayload({uuid: 'r1', scenario: 'team'} as never, config);
        expect(payload.services[0].limits).toEqual({max_concurrent: 2});
    });
});

describe('PolicyLimitsFields (person / team)', () => {
    it('hides steering, and offers overflow only once a concurrency cap is set', async () => {
        const {default: PolicyLimitsFields} = await import('./PolicyLimitsFields');
        const onChange = vi.fn();
        const {rerender} = render(
            <PolicyLimitsFields value={{limits: {}, overflow: 'queue'}} onChange={onChange}/>,
        );
        expect(screen.queryByLabelText('limits.softPercent')).toBeNull();
        expect(screen.queryByText('limits.overflow.label')).toBeNull();

        rerender(<PolicyLimitsFields value={{limits: {max_concurrent: 2}, overflow: 'queue'}} onChange={onChange}/>);
        expect(screen.getByLabelText('limits.overflow.timeout')).toBeTruthy();

        rerender(<PolicyLimitsFields value={{limits: {max_concurrent: 2}, overflow: 'reject'}} onChange={onChange}/>);
        expect(screen.queryByLabelText('limits.overflow.timeout')).toBeNull();
    });
});
