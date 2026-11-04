import { describe, expect, it } from 'vitest';
import { bareId, entityText, shortId } from './entityLabel';

describe('shortId', () => {
    it('keeps short external ids whole', () => {
        expect(shortId('direct', '123456789')).toEqual({ text: '123456789', shortened: false });
        expect(shortId('group', '-10024681012')).toEqual({ text: '-10024681012', shortened: false });
        expect(shortId('direct', '12345678901234567890')).toEqual({ text: '12345678901234567890', shortened: false });
    });

    it('shortens long external ids to head…tail', () => {
        expect(shortId('group', 'oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5')).toEqual({ text: 'oc_a1b…d4e5', shortened: true });
    });

    it('shortens generated bot ids to the distinguishing tail only', () => {
        expect(shortId('bot', '1730600000123456789-imbot')).toEqual({ text: '…456789', shortened: true });
        expect(shortId('bot', '1730600044998877665-imbot')).toEqual({ text: '…877665', shortened: true });
    });

    it('keeps short bot ids whole (mock and imported ids)', () => {
        expect(shortId('bot', 'mock-bot-001')).toEqual({ text: 'mock-bot-001', shortened: false });
    });
});

describe('bareId', () => {
    it('drops only the id\'s own platform prefix', () => {
        expect(bareId('telegram', 'telegram:123456789')).toBe('123456789');
        expect(bareId('telegram', 'telegram:-10024681012')).toBe('-10024681012');
        expect(bareId('telegram', '-10024681012')).toBe('-10024681012');
        expect(bareId('feishu', 'oc_abc')).toBe('oc_abc');
        expect(bareId('feishu', 'telegram:1')).toBe('telegram:1');
    });
});

describe('entityText', () => {
    it('renders 平台 · alias · 群 · id and omits empty or inapplicable segments', () => {
        expect(entityText({ platform: 'Telegram', alias: 'Alice', id: '123456789' })).toBe('Telegram · Alice · 123456789');
        expect(entityText({ platform: 'Telegram', id: '123456789' })).toBe('Telegram · 123456789');
        expect(entityText({ platform: 'Feishu', group: true, alias: 'Dev team', id: 'oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5' }, { groupLabel: '群' })).toBe('Feishu · Dev team · 群 · oc_a1b…d4e5');
        expect(entityText({ platform: 'Feishu', group: true, id: 'oc_1' })).toBe('Feishu · Group · oc_1');
        expect(entityText({ platform: 'Telegram', alias: '  ', id: '42' })).toBe('Telegram · 42');
    });

    it('keeps the full id when shortening is off', () => {
        const id = 'oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5';
        expect(entityText({ platform: 'Feishu', group: true, id }, { shorten: false, groupLabel: '群' })).toBe(`Feishu · 群 · ${id}`);
    });

    it('uses the bot shortening rule for bots', () => {
        expect(entityText({ platform: 'Telegram', alias: '@ops', id: '1730600000123456789-imbot' }, { kind: 'bot' })).toBe('Telegram · @ops · …456789');
    });
});
