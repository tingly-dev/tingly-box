import { formValueToSource, type MCPSourceConfig, type MCPSourceFormValue } from './types';

// IDs are addressing, not naming: random, never derived from user input, and
// never reused after a delete (a sequence would re-point old commands and
// grants at a new object). Kept short because the ID is embedded in the
// normalized tool name (tingly_box_mcp__<id>__<tool>), which models cap at 64.
export function nextMCPId(prefix: string, existing: string[]) {
    for (;;) {
        const bytes = crypto.getRandomValues(new Uint8Array(4));
        const id = `${prefix}-${Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')}`;
        if (!existing.includes(id)) return id;
    }
}

// Saving a connection must not restore the usages or policies captured when
// the editor opened. Those can change independently in the same workspace.
export function connectionPatch(form: MCPSourceFormValue): MCPSourceConfig {
    const source = formValueToSource(form);
    const remote = form.transport !== 'stdio';
    return {
        id: source.id,
        name: source.name,
        transport: source.transport,
        origin: form.original?.transport && form.original.transport !== form.transport ? 'external' : source.origin,
        endpoint: remote ? source.endpoint : '',
        headers: remote ? source.headers : {},
        command: remote ? '' : source.command,
        args: remote ? [] : source.args,
        cwd: remote ? '' : source.cwd,
        env: source.env,
        proxy_url: source.proxy_url,
        tools: source.tools,
    };
}
