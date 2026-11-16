import { describe, expect, it } from 'vitest';
import { connectionPatch, nextMCPId } from './workspaceState';
import { sourceToFormValue } from './types';
describe('connection edits and identifiers', () => {
    it('updates a connection without replacing its usage, enabled state or per-tool policy', () => {
        const form = sourceToFormValue({
            id: 'remote',
            name: 'Docs',
            transport: 'http',
            endpoint: 'https://docs.test/mcp',
            headers: { Authorization: '${TOKEN}' },
            env: { TOKEN: 'saved' },
            tool_policies: { echo: { enabled: false } },
            usage: { client: false, gateway: true },
            enabled: false,
        });
        form.endpoint = 'https://other.test/mcp';
        const patch = connectionPatch(form);
        expect(patch).toMatchObject({
            id: 'remote',
            name: 'Docs',
            endpoint: form.endpoint,
            headers: { Authorization: '${TOKEN}' },
            env: { TOKEN: 'saved' },
        });
        expect(patch).not.toHaveProperty('tool_policies');
        expect(patch).not.toHaveProperty('usage');
        expect(patch).not.toHaveProperty('enabled');
    });
    it('clears transport-specific fields when switching remote/local connections', () => {
        const form = sourceToFormValue({
            id: 'source',
            transport: 'http',
            endpoint: 'https://old.test/mcp',
            headers: { Authorization: 'old' },
        });
        form.transport = 'stdio';
        form.command = 'node';
        expect(connectionPatch(form)).toMatchObject({ command: 'node', endpoint: '', headers: {} });
        form.transport = 'http';
        expect(connectionPatch(form)).toMatchObject({ command: '', args: [], cwd: '' });
    });
    it('creates random prefixed IDs that are unique, name-independent and fit tool-name limits', () => {
        const id = nextMCPId('src', ['src-00000000']);
        expect(id).toMatch(/^src-[0-9a-f]{8}$/);
        expect(nextMCPId('src', [])).not.toBe(nextMCPId('src', []));
        expect(`tingly_box_mcp__${id}__a_tool_name_of_thirty_chars`.length).toBeLessThanOrEqual(64);
    });
});
