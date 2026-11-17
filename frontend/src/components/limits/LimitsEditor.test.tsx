import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import LimitsEditor, {compactLimits, isEmptyLimits} from './LimitsEditor';
import {buildRuleUpdatePayload} from '@/components/rule-card/ruleUpdatePayload';
import type {ConfigRecord} from '@/components/RoutingGraphTypes';

vi.mock('@/services/api', () => ({api: {getLimitsStatus: vi.fn().mockResolvedValue({success: true, data: []})}}));
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

    it('reads K and M suffixes in the token budget and settles to the short form on blur', () => {
        const onChange = vi.fn();
        render(<LimitsEditor value={{}} onChange={onChange}/>);
        const field = screen.getByLabelText('limits.maxTokens') as HTMLInputElement;
        fireEvent.change(field, {target: {value: '2M'}});
        expect(onChange).toHaveBeenLastCalledWith({max_tokens: 2_000_000});
        fireEvent.change(field, {target: {value: '1.5m'}});
        expect(onChange).toHaveBeenLastCalledWith({max_tokens: 1_500_000});
        fireEvent.change(field, {target: {value: '500 K'}});
        expect(onChange).toHaveBeenLastCalledWith({max_tokens: 500_000});
        fireEvent.change(field, {target: {value: '12x'}});
        expect(field.getAttribute('aria-invalid')).toBe('true');
        expect(onChange).toHaveBeenLastCalledWith({max_tokens: 500_000}); // unreadable text changes nothing
        fireEvent.change(field, {target: {value: ''}});
        expect(onChange).toHaveBeenLastCalledWith({max_tokens: undefined});
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

describe('limits at rest on a service node', () => {
    it('summarizes configured caps, one line each', async () => {
        const {summarizeLimits, formatWindow} = await import('./limitsSummary');
        const t = (key: string, o?: Record<string, unknown>) => `${key}:${JSON.stringify(o ?? {})}`;
        expect(summarizeLimits(undefined, t)).toEqual([]);
        expect(summarizeLimits({}, t)).toEqual([]);
        const lines = summarizeLimits({max_concurrent: 4, max_tokens: 2_000_000, window_seconds: 18000, soft_percent: 80, quota_percent: 95}, t);
        expect(lines).toHaveLength(4);
        expect(lines[0]).toContain('limits.summary.concurrent');
        expect(lines[1]).toContain('"window":"5h"');
        expect(formatWindow(3600)).toBe('1h');
        expect(formatWindow(604800)).toBe('7d');
        expect(formatWindow(90)).toBe('90s');
    });

    it('shows a badge only when the service or its account is limited', async () => {
        const {default: LimitsBadge} = await import('./LimitsBadge');
        const {rerender} = render(<LimitsBadge />);
        expect(screen.queryByTestId('limits-badge')).toBeNull();

        rerender(<LimitsBadge serviceLimits={{}} accountLimits={{}} />);
        expect(screen.queryByTestId('limits-badge')).toBeNull(); // empty objects are "no limits"

        rerender(<LimitsBadge serviceLimits={{max_concurrent: 2}} />);
        expect(screen.getByTestId('limits-badge')).toBeTruthy();

        rerender(<LimitsBadge accountLimits={{max_requests: 100}} />);
        expect(screen.getByTestId('limits-badge')).toBeTruthy(); // account cap alone is enough
    });
});

describe('LimitsBadge as a control', () => {
    it('is clickable without bubbling to the node, and quiet (revealed on hover) while nothing is set', async () => {
        const {default: LimitsBadge, LIMITS_QUIET_CLASS} = await import('./LimitsBadge');
        const open = vi.fn();
        const node = vi.fn();
        const {container, rerender} = render(
            <div onClick={node}><LimitsBadge onClick={open} /></div>,
        );
        // editable but unlimited: present, flagged quiet so the node can reveal it on hover
        expect(container.querySelector(`.${LIMITS_QUIET_CLASS}`)).toBeTruthy();
        fireEvent.click(screen.getByTestId('limits-badge'));
        expect(open).toHaveBeenCalledTimes(1);
        expect(node).not.toHaveBeenCalled();

        fireEvent.keyDown(screen.getByTestId('limits-badge'), {key: 'Enter'});
        expect(open).toHaveBeenCalledTimes(2);

        // once limited it is always shown
        rerender(<div onClick={node}><LimitsBadge serviceLimits={{max_concurrent: 2}} onClick={open} /></div>);
        expect(container.querySelector(`.${LIMITS_QUIET_CLASS}`)).toBeNull();
        expect(screen.getByTestId('limits-badge').getAttribute('role')).toBe('button');
    });

    it('renders nothing for an unlimited, read-only node', async () => {
        const {default: LimitsBadge} = await import('./LimitsBadge');
        render(<LimitsBadge serviceLimits={{}} accountLimits={{}} />);
        expect(screen.queryByTestId('limits-badge')).toBeNull();
    });
});

describe('token count info icon', () => {
    it('sits in the Max tokens field of every editor and spells out how tokens are counted on hover', async () => {
        const {default: PolicyLimitsFields} = await import('./PolicyLimitsFields');
        const {unmount} = render(<LimitsEditor value={{}} onChange={vi.fn()} showQuota/>);
        const icon = screen.getByLabelText('limits.maxTokensInfoLabel');
        expect(icon.closest('[data-testid="limits-editor"]')).toBeTruthy();
        fireEvent.mouseOver(icon);
        expect(await screen.findByText('limits.maxTokensInfo', {}, {timeout: 3000})).toBeTruthy();
        unmount();

        // person / team editors reuse the same editor, so the same icon
        render(<PolicyLimitsFields value={{limits: {}, overflow: 'queue'}} onChange={vi.fn()}/>);
        expect(screen.getByLabelText('limits.maxTokensInfoLabel')).toBeTruthy();
    });
});

describe('amount parsing', () => {
    it('reads counts with K / M suffixes and writes them back short', async () => {
        const {parseAmount, formatAmount} = await import('./amount');
        expect(parseAmount('2M')).toEqual({kind: 'ok', value: 2_000_000});
        expect(parseAmount(' 1,500k ')).toEqual({kind: 'ok', value: 1_500_000});
        expect(parseAmount('0.5M')).toEqual({kind: 'ok', value: 500_000});
        expect(parseAmount('1.')).toEqual({kind: 'ok', value: 1});
        expect(parseAmount('1234567')).toEqual({kind: 'ok', value: 1_234_567});
        expect(parseAmount('')).toEqual({kind: 'empty'});
        expect(parseAmount('0')).toEqual({kind: 'empty'});
        expect(parseAmount('-5')).toEqual({kind: 'invalid'});
        expect(parseAmount('2G')).toEqual({kind: 'invalid'});
        expect(formatAmount(2_000_000)).toBe('2M');
        expect(formatAmount(1_500_000)).toBe('1.5M');
        expect(formatAmount(500_000)).toBe('500K');
        expect(formatAmount(1_234_567)).toBe('1234567');
        expect(formatAmount(undefined)).toBe('');
    });
});

describe('LimitsBadge tooltip', () => {
    it('opens with the numbers on hover and does not leak mouseover to an enclosing tooltip', async () => {
        const {default: LimitsBadge} = await import('./LimitsBadge');
        const outer = vi.fn();
        render(
            <div onMouseOver={outer}>
                <LimitsBadge serviceLimits={{max_concurrent: 2}} accountLimits={{max_tokens: 1000}} />
            </div>,
        );
        fireEvent.mouseOver(screen.getByTestId('limits-badge'));
        expect(await screen.findByTestId('limits-badge-tooltip', {}, {timeout: 3000})).toBeTruthy();
        expect(outer).not.toHaveBeenCalled();
    });
});

describe('Account limits section (provider dialogs)', () => {
    it('is behind Advanced, closed by default, and says how many caps are set', async () => {
        const {default: AccountLimitsSection} = await import('./AccountLimitsSection');
        const {rerender} = render(<AccountLimitsSection providerUuid="p1" value={{}} onChange={vi.fn()}/>);
        const toggle = screen.getByRole('button', {name: /common.advanced/});
        expect(toggle.getAttribute('aria-expanded')).toBe('false');
        expect(screen.queryByTestId('limits-editor')).toBeNull();
        expect(screen.queryByText('common.advancedCustomized')).toBeNull();

        // set caps stay visible on the closed header without forcing it open
        rerender(<AccountLimitsSection providerUuid="p1" value={{max_concurrent: 4, max_tokens: 2_000_000, quota_percent: 95}} onChange={vi.fn()}/>);
        expect(screen.getByText('common.advancedCustomized')).toBeTruthy();
        expect(screen.queryByTestId('limits-editor')).toBeNull();

        fireEvent.click(toggle);
        expect(screen.getByTestId('limits-editor')).toBeTruthy();
    });
});
