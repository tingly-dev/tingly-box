import { describe, expect, it } from 'vitest';
import { connectionSummary, isServerToolSource, isToolRestricted, toolPolicyPatch } from './toolPresentation';

const tool = { source_id: 'advisor', name: 'advisor', normalized_name: 'x', enabled: false, usage: { client: false, gateway: true }, implementation: 'virtual', input_schema: {} } as never;

describe('connectionSummary', () => {
    it('flags what a connection still needs', () => {
        expect(connectionSummary({ id: 'a', transport: 'http', endpoint: '' }).missing).toEqual(['endpoint']);
        expect(connectionSummary({ id: 'a', transport: 'stdio', command: '' }).missing).toEqual(['command']);
        expect(connectionSummary({ id: 'a', transport: 'advisor', advisor: {} }).missing).toEqual(['advisorModel']);
        expect(
            connectionSummary({ id: 'a', transport: 'http', endpoint: 'https://x', headers: { Authorization: '' } }).missing
        ).toEqual(['headerValue']);
    });
    it('summarises what is set', () => {
        const s = connectionSummary({
            id: 'a',
            transport: 'stdio',
            command: 'npx',
            args: ['-y', 'srv'],
            env: { KEY: 'v' },
        });
        expect(s.target).toBe('npx -y srv');
        expect(s.details).toEqual([{ key: 'env', count: 1 }]);
        expect(s.missing).toEqual([]);
    });
});

describe('built-in sources', () => {
    const advisor = { id: 'advisor', transport: 'advisor', origin: 'builtin', enabled: false, tools: ['advisor'] };
    it('keeps server web tools separate from their MCP version', () => {
        expect(isServerToolSource({ id: 'server-webtools', transport: 'stdio' })).toBe(true);
        expect(isServerToolSource({ id: 'webtools', transport: 'stdio' })).toBe(false);
        expect(connectionSummary({ id: 'server-webtools', transport: 'stdio', command: '/app/tingly-box' })).toMatchObject({
            target: 'Serper / Jina Reader',
            missing: ['searchKey'],
        });
        expect(connectionSummary({ id: 'server-webtools', env: { SERPER_API_KEY: 'configured' } }).missing).toEqual([]);
    });
    it('does not require a connection to enable a configured server web tool', () => {
        const source = { id: 'server-webtools', enabled: false, tool_policies: {
            mcp_web_search: { enabled: false }, mcp_web_fetch: { enabled: false },
        } };
        const fetch = { source_id: 'server-webtools', name: 'mcp_web_fetch', normalized_name: 'fetch', enabled: false, usage: { client: false, gateway: true }, implementation: 'mcp', input_schema: {} };
        expect(isToolRestricted(fetch, source)).toBe(false);
        expect(toolPolicyPatch(source, fetch, { enabled: true })).toMatchObject({
            enabled: true,
            tool_policies: { mcp_web_search: { enabled: false }, mcp_web_fetch: { enabled: true } },
        });
    });
    it('can be switched on even while the source is off', () => {
        expect(isToolRestricted(tool, advisor)).toBe(false);
        expect(isToolRestricted(tool, { id: 'remote', transport: 'http', enabled: false })).toBe(true);
    });
    it('switching a built-in tool on also turns its source on', () => {
        expect(toolPolicyPatch(advisor, tool, { enabled: true })).toMatchObject({ id: 'advisor', enabled: true });
        expect(toolPolicyPatch(advisor, tool, { enabled: false })).not.toHaveProperty('enabled');
        expect(toolPolicyPatch({ id: 'remote' }, tool, { enabled: true })).not.toHaveProperty('enabled');
    });
});
