import { describe, expect, it } from 'vitest';
import { aliasOrName, bareId, entityText, groupMark, platformGroupText, shortId } from './entityLabel';

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

describe('groupMark / platformGroupText', () => {
    it('fills the platform into the localised pattern and can drop it again', () => {
        expect(platformGroupText('Feishu')).toBe('Feishu group');
        expect(platformGroupText('飞书', '{{platform}}群')).toBe('飞书群');
        expect(platformGroupText('Feishu', 'Группа {{platform}}')).toBe('Группа Feishu');
        expect(platformGroupText('')).toBe('Group');
        expect(groupMark()).toBe('Group');
        expect(groupMark('{{platform}}群')).toBe('群');
    });
});

describe('entityText', () => {
    it('renders 平台 · alias · id, merges the group marker into the platform and omits empty or inapplicable segments', () => {
        expect(entityText({ platform: 'Telegram', alias: 'Alice', id: '123456789' })).toBe('Telegram · Alice · 123456789');
        expect(entityText({ platform: 'Telegram', id: '123456789' })).toBe('Telegram · 123456789');
        expect(entityText({ platform: 'Feishu', group: true, alias: 'Dev team', id: 'oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5' }, { groupLabel: '{{platform}}群' })).toBe('Feishu群 · Dev team · oc_a1b…d4e5');
        expect(entityText({ platform: 'Feishu', group: true, id: 'oc_1' })).toBe('Feishu group · oc_1');
        expect(entityText({ platform: 'Telegram', alias: '  ', id: '42' })).toBe('Telegram · 42');
    });

    it('keeps the full id when shortening is off', () => {
        const id = 'oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5';
        expect(entityText({ platform: 'Feishu', group: true, id }, { shorten: false, groupLabel: '{{platform}}群' })).toBe(`Feishu群 · ${id}`);
    });

    it('uses the bot shortening rule for bots', () => {
        expect(entityText({ platform: 'Telegram', alias: '@ops', id: '1730600000123456789-imbot' }, { kind: 'bot' })).toBe('Telegram · @ops · …456789');
    });
});

describe('aliasOrName', () => {
    it('prefers a non-blank alias, then the platform name, else blank', () => {
        expect(aliasOrName('Boss', 'Alice')).toBe('Boss');
        expect(aliasOrName('  ', 'Alice')).toBe('Alice');
        expect(aliasOrName(undefined, ' Alice ')).toBe('Alice');
        expect(aliasOrName('', '')).toBe('');
    });
});
