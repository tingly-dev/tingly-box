import { describe, expect, it } from 'vitest';
import { defaultMCPSourceFormValue, formValueToSource } from '../../features/mcp/types';
import { applyParsedMCPServer, MCPPasteError, parseMCPPaste, splitShellWords } from '../../features/mcp/pasteImport';

const errorCode = (input: string) => {
    try {
        parseMCPPaste(input);
    } catch (e) {
        return e instanceof MCPPasteError ? e.code : 'other';
    }
    return 'none';
};

describe('splitShellWords', () => {
    it('honours quotes, escapes and line continuations', () => {
        expect(splitShellWords(`a 'b c' "d \\"e\\" $f" g\\ h`)).toEqual(['a', 'b c', 'd "e" $f', 'g h']);
        expect(splitShellWords('npx -y \\\n  pkg \\\n  /path')).toEqual(['npx', '-y', 'pkg', '/path']);
        expect(splitShellWords(`'it''s' ""`)).toEqual(['its', '']);
    });

    it('rejects an unclosed quote', () => {
        expect(() => splitShellWords('npx "pkg')).toThrow(MCPPasteError);
    });
});

describe('parseMCPPaste', () => {
    it('splits a command line into command and args, with leading env assignments', () => {
        expect(parseMCPPaste('$ API_KEY=abc npx -y @modelcontextprotocol/server-filesystem "/My Docs"')).toEqual({
            format: 'command',
            servers: [
                {
                    name: 'server-filesystem',
                    transport: 'stdio',
                    command: 'npx',
                    args: ['-y', '@modelcontextprotocol/server-filesystem', '/My Docs'],
                    env: { API_KEY: 'abc' },
                },
            ],
        });
        expect(parseMCPPaste('docker run -i --rm ghcr.io/github/github-mcp-server').servers[0]).toMatchObject({
            command: 'docker',
            args: ['run', '-i', '--rm', 'ghcr.io/github/github-mcp-server'],
        });
    });

    it('reads Claude Desktop JSON with several servers', () => {
        const result = parseMCPPaste(`{
          "mcpServers": {
            "filesystem": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"] },
            "github": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"], "env": { "GITHUB_TOKEN": "x" } },
            "remote": { "type": "sse", "url": "https://example.com/events" },
          }
        }`);
        expect(result.format).toBe('json');
        expect(result.servers).toEqual([
            { name: 'filesystem', transport: 'stdio', command: 'npx', args: ['-y', '@modelcontextprotocol/server-filesystem', '/tmp'] },
            { name: 'github', transport: 'stdio', command: 'npx', args: ['-y', '@modelcontextprotocol/server-github'], env: { GITHUB_TOKEN: 'x' } },
            { name: 'remote', transport: 'sse', endpoint: 'https://example.com/events' },
        ]);
    });

    it('reads other client config shapes and fragments', () => {
        expect(parseMCPPaste('{"servers": {"api": {"type": "http", "url": "https://x.test/mcp", "headers": {"Authorization": "Bearer t"}}}}').servers).toEqual([
            { name: 'api', transport: 'http', endpoint: 'https://x.test/mcp', headers: { Authorization: 'Bearer t' } },
        ]);
        expect(parseMCPPaste('{"mcp": {"fs": {"type": "local", "command": ["npx", "-y", "pkg"], "environment": {"A": "1"}}}}').servers).toEqual([
            { name: 'fs', transport: 'stdio', command: 'npx', args: ['-y', 'pkg'], env: { A: '1' } },
        ]);
        expect(parseMCPPaste('"memory": {"command": "npx -y @modelcontextprotocol/server-memory"},').servers).toEqual([
            { name: 'memory', transport: 'stdio', command: 'npx', args: ['-y', '@modelcontextprotocol/server-memory'] },
        ]);
        expect(parseMCPPaste('{"command": "uvx", "args": ["mcp-server-git"]}').servers[0]).toMatchObject({ command: 'uvx' });
    });

    it('reads a Codex config.toml entry and the opencode fragment the profile page shows', () => {
        const toml = [
            '[mcp_servers.ctx]',
            'command = "npx"',
            'args = [',
            '  "-y",',
            "  '@upstash/context7-mcp',",
            ']',
            'env = { API_KEY = "k", "OTHER" = "o" }',
            '',
            '[mcp_servers.tb]',
            'url = "http://localhost:12580/api/v1/mcp/default"',
            '[mcp_servers.tb.http_headers]',
            'Authorization = "Bearer t"',
        ].join('\n');
        expect(parseMCPPaste(toml)).toEqual({
            format: 'toml',
            servers: [
                { name: 'ctx', transport: 'stdio', command: 'npx', args: ['-y', '@upstash/context7-mcp'], env: { API_KEY: 'k', OTHER: 'o' } },
                { name: 'tb', transport: 'http', endpoint: 'http://localhost:12580/api/v1/mcp/default', headers: { Authorization: 'Bearer t' } },
            ],
        });
        expect(
            parseMCPPaste('"mcp": {\n  "tb": {\n    "type": "remote",\n    "url": "http://h/mcp",\n    "oauth": false\n  }\n}').servers
        ).toEqual([{ name: 'tb', transport: 'http', endpoint: 'http://h/mcp' }]);
    });

    it('reads a JSON array as the argument list', () => {
        expect(parseMCPPaste('["-y", "pkg", "/path"]')).toEqual({
            format: 'args',
            servers: [{ transport: 'stdio', args: ['-y', 'pkg', '/path'] }],
        });
    });

    it('reads a URL as a remote connection', () => {
        expect(parseMCPPaste('https://mcp.notion.com/mcp').servers[0]).toEqual({
            name: 'mcp.notion.com',
            transport: 'http',
            endpoint: 'https://mcp.notion.com/mcp',
        });
        expect(parseMCPPaste('https://x.test/sse').servers[0].transport).toBe('sse');
    });

    it('reads client registration commands', () => {
        expect(parseMCPPaste('claude mcp add --transport http notion https://mcp.notion.com/mcp -H "X-Key: k"').servers[0]).toEqual({
            name: 'notion',
            transport: 'http',
            endpoint: 'https://mcp.notion.com/mcp',
            headers: { 'X-Key': 'k' },
        });
        expect(parseMCPPaste('claude mcp add -s user --env A=1 B=2 airtable -- npx -y airtable-mcp-server').servers[0]).toEqual({
            name: 'airtable',
            transport: 'stdio',
            command: 'npx',
            args: ['-y', 'airtable-mcp-server'],
            env: { A: '1', B: '2' },
        });
        expect(parseMCPPaste('claude mcp add fs npx -y pkg /p').servers[0]).toMatchObject({ name: 'fs', command: 'npx', args: ['-y', 'pkg', '/p'] });
        expect(parseMCPPaste('codex mcp add ctx --env K=V -- npx -y @upstash/context7-mcp').servers[0]).toEqual({
            name: 'ctx',
            transport: 'stdio',
            command: 'npx',
            args: ['-y', '@upstash/context7-mcp'],
            env: { K: 'V' },
        });
        expect(parseMCPPaste(`claude mcp add-json w '{"type":"http","url":"https://w.test/mcp"}'`).servers[0]).toEqual({
            name: 'w',
            transport: 'http',
            endpoint: 'https://w.test/mcp',
        });
    });

    it('names what it cannot read', () => {
        expect(errorCode('   ')).toBe('empty');
        expect(errorCode('{"mcpServers": {')).toBe('invalidJson');
        expect(errorCode('{"name": "x"}')).toBe('noServer');
        expect(errorCode('[1, {"a": 2}]')).toBe('notStrings');
        expect(errorCode("npx 'pkg")).toBe('unclosedQuote');
        expect(errorCode('claude mcp add only-a-name')).toBe('noCommand');
        expect(errorCode('[mcp_servers.x]\ncommand npx')).toBe('invalidToml');
    });
});

describe('applyParsedMCPServer', () => {
    it('fills the form and keeps a name the user typed', () => {
        const form = { ...defaultMCPSourceFormValue(), id: 'src-1', name: 'Mine', transport: 'http' as const };
        const next = applyParsedMCPServer(form, {
            name: 'github',
            transport: 'stdio',
            command: 'npx',
            args: ['-y', 'pkg'],
            env: { GITHUB_TOKEN: 'x', HOME: '${HOME}' },
        });
        expect(next).toMatchObject({
            name: 'Mine',
            transport: 'stdio',
            command: 'npx',
            args: ['-y', 'pkg'],
            env: [{ key: 'GITHUB_TOKEN', value: 'x' }],
            envPassthrough: ['HOME'],
        });
        expect(formValueToSource(next)).toMatchObject({ command: 'npx', args: ['-y', 'pkg'], env: { GITHUB_TOKEN: 'x', HOME: '${HOME}' } });
    });

    it('fills an empty name and leaves the command when only args are pasted', () => {
        const form = { ...defaultMCPSourceFormValue(), command: 'uvx' };
        expect(applyParsedMCPServer(form, { name: 'n', endpoint: 'https://x', transport: 'http' })).toMatchObject({
            name: 'n',
            endpoint: 'https://x',
        });
        expect(applyParsedMCPServer(form, { transport: 'stdio', args: ['a'] })).toMatchObject({ command: 'uvx', args: ['a'] });
    });
});
