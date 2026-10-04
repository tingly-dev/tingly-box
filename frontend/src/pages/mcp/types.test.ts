import { describe, expect, it } from 'vitest';
import { formValueToSource, sourceToFormValue, type MCPSourceConfig } from './types';

describe('MCP source editing', () => {
    it.each(['http', 'sse'])('preserves advanced settings and clears authentication fields for %s', (transport) => {
        const source: MCPSourceConfig = {
            id: 'remote',
            transport,
            endpoint: 'https://example.test/mcp',
            headers: { Authorization: '${TOKEN}' },
            env: { TOKEN: 'secret' },
            proxy_url: 'http://proxy:8080',
            tool_policies: { echo: { enabled: false } },
            allowed_extra_headers: ['X-Trace'],
            usage: { client: true, gateway: true },
        };
        const form = sourceToFormValue(source);
        form.endpoint = 'https://new.test/mcp';
        expect(formValueToSource(form)).toMatchObject({ ...source, endpoint: 'https://new.test/mcp' });
        form.headers = [];
        form.env = [];
        form.envPassthrough = [];
        form.useGlobalProxy = true;
        expect(formValueToSource(form)).toMatchObject({
            headers: {},
            env: {},
            proxy_url: '',
            tool_policies: source.tool_policies,
        });
    });
    it('retains positional empty arguments and inherited variables independently of proxy', () => {
        const form = sourceToFormValue({
            id: 'stdio',
            transport: 'stdio',
            command: 'node',
            args: ['first', '', '  spaced  '],
            env: { TOKEN: '${TOKEN}', FIXED: 'value' },
            proxy_url: 'http://proxy:8080',
        });
        expect(form.envPassthrough).toEqual(['TOKEN']);
        expect(formValueToSource(form).args).toEqual(['first', '', '  spaced  ']);
        form.useGlobalProxy = true;
        expect(formValueToSource(form).env).toEqual({ TOKEN: '${TOKEN}', FIXED: 'value' });
    });
    it('migrates legacy server visibility into gateway usage without losing tool policies', () => {
        const form = sourceToFormValue({
            id: 'server',
            command: 'node',
            visibility: 'server',
            tool_policies: { echo: { usage: { client: true, gateway: false } } },
        });
        expect(form.usage).toEqual({ client: false, gateway: true });
        expect(formValueToSource(form).tool_policies?.echo.usage).toEqual({ client: true, gateway: false });
    });
});
