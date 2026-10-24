import { formValueToSource, type MCPClientProfile, type MCPSourceConfig, type MCPSourceFormValue } from './types';

export function nextMCPId(name: string, existing: string[], fallback: string) {
    let base =
        name
            .toLowerCase()
            .replace(/[^a-z0-9_-]+/g, '-')
            .replace(/^-+|-+$/g, '') || fallback;
    if (!/^[a-z_]/.test(base)) base = `${fallback}-${base}`;
    base = base.replace(/__+/g, '_');
    let id = base;
    for (let suffix = 2; existing.includes(id); suffix++) id = `${base}-${suffix}`;
    return id;
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

export function toggleClientSource(profile: MCPClientProfile, id: string, checked: boolean, known: string[]) {
    const current = profile.sources?.includes('*')
        ? [...new Set([...known, ...(profile.sources || []).filter((s) => s !== '*')])]
        : [...(profile.sources || [])];
    return {
        ...profile,
        sources: checked ? [...new Set([...current, id])] : current.filter((source) => source !== id),
    };
}
