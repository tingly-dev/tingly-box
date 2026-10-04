import { describe, expect, it } from 'vitest';
import { connectionPatch, nextMCPId, toggleClientSource } from './workspaceState';
import { sourceToFormValue } from './types';
describe('independent connection and client access edits', () => {
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
    it('materializes a wildcard before removing one source and preserves individual tool grants', () => {
        const original = { id: 'client', name: 'Client', sources: ['*'], tools: ['tingly_box_mcp__b__echo'] };
        expect(toggleClientSource(original, 'a', false, ['a', 'b'])).toEqual({ ...original, sources: ['b'] });
        expect(original.sources).toEqual(['*']);
    });
    it('creates valid unique IDs for names in different scripts and names containing reserved separators', () => {
        expect(nextMCPId('Team docs', ['team-docs'], 'source')).toBe('team-docs-2');
        expect(nextMCPId('知识库', ['source'], 'source')).toBe('source-2');
        expect(nextMCPId('1 Team__Docs', [], 'source')).toBe('source-1-team_docs');
    });
});
