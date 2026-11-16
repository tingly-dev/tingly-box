import { describe, expect, it } from 'vitest';
import {
    addProfileTools,
    defaultProfile,
    profileRows,
    profileServerName,
    removeProfileTool,
    setProfileToolOn,
} from './profileState';
import type { MCPCatalogTool, MCPProfile, MCPRoutingSnapshot } from './types';

const tool = (name: string, enabled = true, client = true) =>
    ({
        source_id: 'docs',
        name,
        normalized_name: `tingly_box_mcp__docs__${name}`,
        enabled,
        implementation: client ? 'mcp' : 'virtual',
        usage: { client, gateway: !client },
        input_schema: {},
    }) as MCPCatalogTool;
const routing = {
    sources: [{ id: 'docs', name: 'Docs', tools: [tool('read'), tool('write', false), tool('advisor', true, false)] }],
    profiles: [],
    server_tools: [],
} as unknown as MCPRoutingSnapshot;
const n = (name: string) => `tingly_box_mcp__docs__${name}`;

describe('profile tool state', () => {
    it('lists every client tool in Default and keeps Server Tools out', () => {
        const rows = profileRows(defaultProfile(), routing);
        expect(rows.map((r) => r.name)).toEqual([n('read'), n('write')]);
        expect(rows.every((r) => r.on)).toBe(true);
    });
    it('stores only switched-off tools for Default, so new tools join switched on', () => {
        const off = setProfileToolOn(defaultProfile(), n('read'), false);
        expect(off.tools).toEqual([{ name: n('read'), enabled: false }]);
        expect(profileRows(off, routing).map((r) => r.on)).toEqual([false, true]);
        expect(setProfileToolOn(off, n('read'), true).tools).toEqual([]);
    });
    it('lists only added tools in a custom profile and keeps a switched-off tool listed', () => {
        let profile: MCPProfile = { id: 'prof-1', name: 'Review', tools: [] };
        expect(profileRows(profile, routing)).toEqual([]);
        profile = addProfileTools(profile, [n('read'), n('write')]);
        profile = setProfileToolOn(profile, n('read'), false);
        expect(profileRows(profile, routing).map((r) => [r.name, r.on])).toEqual([
            [n('read'), false],
            [n('write'), true],
        ]);
        expect(addProfileTools(profile, [n('read')]).tools).toHaveLength(2);
        expect(removeProfileTool(profile, n('read')).tools.map((t) => t.name)).toEqual([n('write')]);
    });
    it('keeps a tool that no longer exists visible in a custom profile', () => {
        const rows = profileRows({ id: 'prof-1', name: 'Old', tools: [{ name: n('gone') }] }, routing);
        expect(rows).toEqual([{ name: n('gone'), tool: undefined, sourceName: '', on: true }]);
    });
    it('registers each profile under its own name, Default as tb', () => {
        expect(profileServerName(defaultProfile())).toBe('tb');
        expect(profileServerName({ id: 'prof-1', name: 'Code Review!', tools: [] })).toBe('tb-code-review');
        expect(profileServerName({ id: 'prof-1', name: '审查', tools: [] })).toBe('tb-prof-1');
    });
});
