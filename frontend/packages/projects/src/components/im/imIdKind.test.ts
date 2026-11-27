import { describe, expect, it } from 'vitest';
import { classifyImId, isBindable, isOfferedForBinding } from './imIdKind';

describe('classifyImId', () => {
    describe('Feishu / Lark', () => {
        it('treats a conversation id as a receiver', () => {
            const v = classifyImId('feishu', 'group', 'oc_a1b2c3d4');
            expect(v).toMatchObject({ kind: 'chat', class: 'sendable', receiveIdType: 'chat_id' });
            expect(isBindable(v)).toBe(true);
        });

        it('treats an open_id and a union_id as receivers', () => {
            expect(classifyImId('feishu', 'direct', 'ou_u42')).toMatchObject({ kind: 'open_id', class: 'sendable', receiveIdType: 'open_id' });
            expect(classifyImId('feishu', 'direct', 'on_u42')).toMatchObject({ kind: 'union_id', class: 'sendable', receiveIdType: 'union_id' });
        });

        it('treats a prefix-less user_id as a receiver', () => {
            expect(classifyImId('feishu', 'direct', '1g9b7c4')).toMatchObject({ kind: 'user_id', class: 'sendable', receiveIdType: 'user_id' });
        });

        it('hides a department and an app id from the selector', () => {
            for (const id of ['od-4e6ac4d14bcd5071a37a39de902c7141', 'cli_a1b2c3']) {
                const v = classifyImId('feishu', 'direct', id);
                expect(v.class).toBe('unsendable');
                expect(v.reasonKey).toBeTruthy();
                expect(isOfferedForBinding(v)).toBe(false);
                expect(isBindable(v)).toBe(false);
            }
        });

        it('treats an email receiver as sendable', () => {
            expect(classifyImId('feishu', 'direct', 'alice@example.com')).toMatchObject({ kind: 'email', class: 'sendable', receiveIdType: 'email' });
        });

        it('classifies Lark the same way and strips either platform prefix', () => {
            expect(classifyImId('lark', 'group', 'lark:oc_x')).toMatchObject({ kind: 'chat', class: 'sendable' });
            expect(classifyImId('feishu', 'direct', 'feishu:ou_x')).toMatchObject({ kind: 'open_id', class: 'sendable' });
            expect(classifyImId('lark', 'direct', 'feishu:od-x')).toMatchObject({ kind: 'department', class: 'unsendable' });
        });

        it('never offers an empty id', () => {
            expect(classifyImId('feishu', 'direct', '  ')).toMatchObject({ kind: 'unknown', class: 'unknown' });
        });
    });

    describe('Slack', () => {
        it('treats D/C/G channel ids as receivers', () => {
            for (const id of ['D012ABCDEF', 'C012ABCDEF', 'G012ABCDEF']) {
                expect(classifyImId('slack', 'direct', id)).toMatchObject({ kind: 'chat', class: 'sendable' });
            }
        });

        it('keeps a user id visible but disabled', () => {
            const v = classifyImId('slack', 'direct', 'U012ABCDEF');
            expect(v).toMatchObject({ kind: 'unknown', class: 'unknown', reasonKey: 'im.idReason.userNotChat' });
            expect(isOfferedForBinding(v)).toBe(true);
            expect(isBindable(v)).toBe(false);
        });
    });

    describe('Telegram / Discord', () => {
        it('treats a numeric Telegram chat id and an @name as receivers', () => {
            expect(classifyImId('telegram', 'direct', '123456789')).toMatchObject({ kind: 'chat', class: 'sendable' });
            expect(classifyImId('telegram', 'group', '-10024681012')).toMatchObject({ kind: 'chat', class: 'sendable' });
            expect(classifyImId('telegram', 'group', 'telegram:-10024681012')).toMatchObject({ kind: 'chat', class: 'sendable' });
            expect(classifyImId('telegram', 'group', '@release_crew')).toMatchObject({ kind: 'chat', class: 'sendable' });
        });

        it('treats a Discord snowflake as a channel and anything else as unknown', () => {
            expect(classifyImId('discord', 'direct', '123456789012345678')).toMatchObject({ kind: 'chat', class: 'sendable' });
            expect(classifyImId('discord', 'direct', 'general')).toMatchObject({ class: 'unknown' });
        });
    });

    describe('DingTalk / WeCom / Weixin', () => {
        it('takes a DingTalk conversation id and keeps a staff id unknown', () => {
            expect(classifyImId('dingtalk', 'direct', 'cidAbCdEf')).toMatchObject({ kind: 'chat', class: 'sendable' });
            expect(classifyImId('dingtalk', 'direct', 'manager1234')).toMatchObject({ class: 'unknown' });
        });

        it('takes WeCom chatids and external user ids', () => {
            expect(classifyImId('wecom', 'group', 'wrAbCdEf')).toMatchObject({ kind: 'chat', class: 'sendable' });
            expect(classifyImId('wecom', 'direct', 'wmAbCdEf')).toMatchObject({ kind: 'user_id', class: 'sendable' });
        });

        it('takes Weixin wxid and chatroom ids', () => {
            expect(classifyImId('weixin', 'direct', 'wxid_abc')).toMatchObject({ kind: 'user_id', class: 'sendable' });
            expect(classifyImId('weixin', 'group', '123@chatroom')).toMatchObject({ kind: 'chat', class: 'sendable' });
        });
    });

    describe('WhatsApp', () => {
        it('takes a phone number and hides a group JID', () => {
            expect(classifyImId('whatsapp', 'direct', '15551234567')).toMatchObject({ kind: 'phone', class: 'sendable' });
            expect(classifyImId('whatsapp', 'direct', '15551234567@s.whatsapp.net')).toMatchObject({ kind: 'phone', class: 'sendable' });
            const group = classifyImId('whatsapp', 'group', '1203630@g.us');
            expect(group.class).toBe('unsendable');
            expect(group.reasonKey).toBe('im.idReason.whatsappGroup');
            expect(isOfferedForBinding(group)).toBe(false);
        });
    });

    it('keeps an unknown platform visible but disabled', () => {
        const v = classifyImId('googlechat', 'direct', 'spaces/AAAA');
        expect(v).toMatchObject({ kind: 'unknown', class: 'unknown', reasonKey: 'im.idReason.unknownPlatform' });
        expect(isOfferedForBinding(v)).toBe(true);
        expect(isBindable(v)).toBe(false);
    });

    it('takes any non-empty id on the local Tingly platform', () => {
        expect(classifyImId('tingly', 'direct', 'local-1')).toMatchObject({ kind: 'chat', class: 'sendable' });
    });
});
