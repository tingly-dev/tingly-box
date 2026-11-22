import type { MCPKVPair, MCPSourceFormValue } from './types';

// Reads what MCP READMEs hand out — a command line, a client registration
// command, a client config file or a URL — into the connection form, so the
// user reviews fields instead of retyping them. Pure: no network, no storage.
// Deliberately not a shell: no pipes, `&&`, globbing or variable expansion.

export type MCPPasteFormat = 'command' | 'registration' | 'json' | 'toml' | 'args' | 'url';

export interface ParsedMCPServer {
    name?: string;
    transport?: MCPSourceFormValue['transport'];
    command?: string;
    args?: string[];
    env?: Record<string, string>;
    headers?: Record<string, string>;
    endpoint?: string;
    cwd?: string;
}

export interface MCPPasteResult {
    format: MCPPasteFormat;
    servers: ParsedMCPServer[];
}

export type MCPPasteErrorCode = 'empty' | 'unclosedQuote' | 'invalidJson' | 'invalidToml' | 'noServer' | 'notStrings' | 'noCommand';

export class MCPPasteError extends Error {
    constructor(
        readonly code: MCPPasteErrorCode,
        readonly detail = ''
    ) {
        super(detail ? `${code}: ${detail}` : code);
    }
}

// POSIX-style word splitting: single quotes are literal, double quotes honour
// \" \\ \$ \`, a backslash outside quotes escapes the next character and a
// backslash-newline joins lines (the usual multi-line README command).
export function splitShellWords(input: string): string[] {
    const words: string[] = [];
    let word = '';
    let inWord = false;
    let quote: '"' | "'" | null = null;
    for (let i = 0; i < input.length; i++) {
        const c = input[i];
        if (quote === "'") {
            if (c === "'") quote = null;
            else word += c;
        } else if (quote === '"') {
            if (c === '"') quote = null;
            else if (c === '\\' && i + 1 < input.length && '"\\$`\n'.includes(input[i + 1])) {
                if (input[i + 1] !== '\n') word += input[i + 1];
                i++;
            } else word += c;
        } else if (c === "'" || c === '"') {
            quote = c;
            inWord = true;
        } else if (c === '\\') {
            const next = input[i + 1];
            if (next === undefined) {
                word += c;
                inWord = true;
            } else if (next !== '\n') {
                word += next;
                inWord = true;
            }
            i++;
        } else if (/\s/.test(c)) {
            if (inWord) words.push(word);
            word = '';
            inWord = false;
        } else {
            word += c;
            inWord = true;
        }
    }
    if (quote) throw new MCPPasteError('unclosedQuote', quote);
    if (inWord) words.push(word);
    return words;
}

const URL_RE = /^https?:\/\/\S+$/i;
const ASSIGNMENT_RE = /^([A-Za-z_][A-Za-z0-9_]*)=([\s\S]*)$/;
const HEADER_RE = /^([A-Za-z0-9-]+):\s*([\s\S]*)$/;

const remoteTransport = (url: string, declared?: string): 'http' | 'sse' =>
    declared === 'sse' || (!declared && /\/sse\/?(\?.*)?$/i.test(url)) ? 'sse' : 'http';

const hostName = (url: string) => {
    try {
        return new URL(url).hostname;
    } catch {
        return undefined;
    }
};

// A name suggestion for `npx -y @scope/pkg@1 …`-style launchers; the user can
// still edit it. Other commands get none rather than a guess like "docker".
const RUNNERS = new Set(['npx', 'uvx', 'bunx', 'pipx', 'dlx', 'pnpx']);
function launcherName(command: string, args: string[]): string | undefined {
    if (!RUNNERS.has(command.split('/').pop() || '')) return undefined;
    const pkg = args.find((arg) => !arg.startsWith('-') && arg !== 'run' && arg !== 'dlx');
    if (!pkg) return undefined;
    const bare = pkg.replace(/^@[^/]+\//, '').replace(/@[^@]*$/, '');
    return bare || undefined;
}

function commandServer(words: string[]): ParsedMCPServer {
    const env: Record<string, string> = {};
    let i = 0;
    if (words[0] === 'env') i++;
    for (; i < words.length; i++) {
        const m = ASSIGNMENT_RE.exec(words[i]);
        if (!m) break;
        env[m[1]] = m[2];
    }
    const [command, ...args] = words.slice(i);
    if (!command) throw new MCPPasteError('noCommand');
    return {
        name: launcherName(command, args),
        transport: 'stdio',
        command,
        args,
        ...(Object.keys(env).length ? { env } : {}),
    };
}

// `claude|gemini mcp add [opts] <name> <command-or-url> [args…]` and
// `codex mcp add <name> [--env K=V] [--url U] -- <command…>`.
const VALUE_FLAGS = new Set([
    '-s',
    '--scope',
    '--callback-port',
    '--client-id',
    '--client-secret',
    '--bearer-token-env-var',
    '--timeout',
    '--trust',
    '--description',
    '--include-tools',
    '--exclude-tools',
]);

function registrationServer(words: string[]): ParsedMCPServer {
    const env: Record<string, string> = {};
    const headers: Record<string, string> = {};
    const positional: string[] = [];
    let transport: string | undefined;
    let url: string | undefined;
    let rest: string[] | undefined;
    for (let i = 0; i < words.length; i++) {
        const word = words[i];
        // After a command, every word is its argument; after a URL the CLI's
        // own options (headers, scope) may still follow.
        if (positional.length >= 2 && !URL_RE.test(positional[1])) {
            rest = [...(rest || []), ...(word === '--' ? [] : [word])];
            continue;
        }
        if (word === '--') {
            rest = words.slice(i + 1);
            break;
        }
        if (!word.startsWith('-')) {
            positional.push(word);
            continue;
        }
        const eq = word.indexOf('=');
        const flag = eq > 0 && word.startsWith('--') ? word.slice(0, eq) : word;
        const inline = eq > 0 && word.startsWith('--') ? word.slice(eq + 1) : undefined;
        const take = () => inline ?? words[++i] ?? '';
        if (flag === '-t' || flag === '--transport') transport = take();
        else if (flag === '--url') url = take();
        else if (flag === '-e' || flag === '--env') {
            // The flag is variadic in these CLIs: keep reading KEY=VALUE words.
            for (let value = take(); ; value = words[++i]) {
                const m = ASSIGNMENT_RE.exec(value || '');
                if (m) env[m[1]] = m[2];
                if (!ASSIGNMENT_RE.test(words[i + 1] || '')) break;
            }
        } else if (flag === '-H' || flag === '--header') {
            for (let value = take(); ; value = words[++i]) {
                const m = HEADER_RE.exec(value || '');
                if (m) headers[m[1]] = m[2];
                if (!/^[A-Za-z0-9-]+:\s/.test(words[i + 1] || '')) break;
            }
        } else if (VALUE_FLAGS.has(flag) && inline === undefined) i++;
    }
    const [name, target] = positional;
    const tail = [...(target ? [target] : []), ...(rest || [])];
    const extras = {
        ...(Object.keys(env).length ? { env } : {}),
        ...(Object.keys(headers).length ? { headers } : {}),
    };
    const endpoint = url ?? (tail.length === 1 && URL_RE.test(tail[0]) ? tail[0] : undefined);
    if (endpoint && transport !== 'stdio') {
        return { name, transport: remoteTransport(endpoint, transport), endpoint, ...extras };
    }
    if (!tail.length) throw new MCPPasteError('noCommand');
    const [command, ...args] = tail;
    return { name, transport: 'stdio', command, args, ...extras };
}

const stringMap = (value: unknown): Record<string, string> | undefined => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, String(v)]));
};

const isServerShape = (value: unknown): value is Record<string, unknown> =>
    !!value &&
    typeof value === 'object' &&
    !Array.isArray(value) &&
    ['command', 'url', 'serverUrl', 'httpUrl', 'args'].some((key) => key in value);

// One entry of mcpServers (Claude Desktop, Cursor, Windsurf, Gemini), servers
// (VS Code), mcp (opencode: command array, environment) or context_servers (Zed).
function jsonServer(raw: Record<string, unknown>, name?: string): ParsedMCPServer {
    const type = typeof (raw.type ?? raw.transport) === 'string' ? String(raw.type ?? raw.transport).toLowerCase() : '';
    const url = [raw.url, raw.serverUrl, raw.httpUrl].find((v): v is string => typeof v === 'string' && !!v);
    const env = stringMap(raw.env ?? raw.environment);
    const headers = stringMap(raw.headers ?? raw.http_headers);
    const extras = { ...(env ? { env } : {}), ...(headers ? { headers } : {}) };
    if (url && type !== 'stdio' && type !== 'local') {
        const declared = type === 'sse' ? 'sse' : type ? 'http' : undefined;
        return { name, transport: remoteTransport(url, declared), endpoint: url, ...extras };
    }
    let command: string | undefined;
    let args = Array.isArray(raw.args) ? raw.args.map(String) : undefined;
    if (Array.isArray(raw.command)) {
        [command, ...args] = [...raw.command.map(String), ...(args || [])];
    } else if (typeof raw.command === 'string') {
        command = raw.command;
        // `"command": "npx -y pkg"` with no args: split it like a command line.
        if (!args && /\s/.test(command.trim())) [command, ...args] = splitShellWords(command);
    }
    if (!command) throw new MCPPasteError('noCommand', name);
    return {
        name,
        transport: 'stdio',
        command,
        args: args || [],
        ...(typeof raw.cwd === 'string' && raw.cwd ? { cwd: raw.cwd } : {}),
        ...extras,
    };
}

const CONTAINERS = ['mcpServers', 'servers', 'mcp', 'context_servers', 'mcp_servers'];

function jsonServers(value: unknown): ParsedMCPServer[] {
    if (isServerShape(value)) return [jsonServer(value)];
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new MCPPasteError('noServer');
    const record = value as Record<string, unknown>;
    for (const key of CONTAINERS) {
        const inner = record[key];
        if (inner && typeof inner === 'object' && !isServerShape(inner)) return jsonServers(inner);
    }
    // A bare name → server map, e.g. `{"filesystem": {...}}` copied from inside mcpServers.
    const entries = Object.entries(record).filter(([, v]) => isServerShape(v));
    if (!entries.length) throw new MCPPasteError('noServer');
    return entries.map(([name, v]) => jsonServer(v as Record<string, unknown>, name));
}

function parseJson(text: string): unknown {
    const attempts = [text, `{${text}}`];
    let first: unknown;
    for (const attempt of attempts) {
        for (const candidate of [attempt, attempt.replace(/,(\s*[}\]])/g, '$1')]) {
            try {
                return JSON.parse(candidate);
            } catch (e) {
                first ??= e;
            }
        }
    }
    throw new MCPPasteError('invalidJson', first instanceof Error ? first.message : '');
}

// The slice of TOML a Codex `config.toml` entry uses: `[mcp_servers.<name>]`
// (and `.env` / `.http_headers` sub-tables), `key = value` with basic or literal
// strings, string arrays and one-line inline tables.
function tomlValue(raw: string): unknown {
    const value = raw.trim();
    if (/^'[^']*'$/.test(value)) return value.slice(1, -1);
    if (value.startsWith('{') && value.endsWith('}')) {
        const json = value
            .slice(1, -1)
            .split(/,(?=(?:[^"']*["'][^"']*["'])*[^"']*$)/)
            .filter((part) => part.trim())
            .map((part) => {
                const eq = part.indexOf('=');
                if (eq < 0) throw new MCPPasteError('invalidToml', part.trim());
                return [part.slice(0, eq).trim().replace(/^"|"$/g, ''), tomlValue(part.slice(eq + 1))];
            });
        return Object.fromEntries(json);
    }
    try {
        return JSON.parse(value.replace(/'([^']*)'/g, (_, s: string) => JSON.stringify(s)).replace(/,(\s*\])/g, '$1'));
    } catch {
        throw new MCPPasteError('invalidToml', value);
    }
}

function parseToml(text: string): Record<string, unknown> {
    const root: Record<string, unknown> = {};
    let table = root;
    const lines = text.split('\n');
    for (let i = 0; i < lines.length; i++) {
        let trimmed = lines[i].trim();
        if (!trimmed || trimmed.startsWith('#')) continue;
        // A multi-line array: `args = [` … `]`.
        if (/=\s*\[[^\]]*$/.test(trimmed)) {
            while (i + 1 < lines.length && !trimmed.endsWith(']')) trimmed += ' ' + lines[++i].trim();
        }
        const header = /^\[([^[\]]+)\]$/.exec(trimmed);
        if (header) {
            table = root;
            for (const key of header[1].split('.').map((k) => k.trim().replace(/^"|"$/g, ''))) {
                table = (table[key] ??= {}) as Record<string, unknown>;
            }
            continue;
        }
        const eq = trimmed.indexOf('=');
        if (eq < 0) throw new MCPPasteError('invalidToml', trimmed);
        table[trimmed.slice(0, eq).trim().replace(/^"|"$/g, '')] = tomlValue(trimmed.slice(eq + 1));
    }
    return root;
}

export function parseMCPPaste(input: string): MCPPasteResult {
    const text = input.replace(/\r\n?/g, '\n').trim();
    if (!text) throw new MCPPasteError('empty');
    // Codex config.toml: the first line is a table header such as [mcp_servers.fs].
    if (/^\[[A-Za-z_][\w.\-"]*\]$/.test(text.split('\n')[0].trim())) {
        return { format: 'toml', servers: jsonServers(parseToml(text)) };
    }
    if (text.startsWith('{') || text.startsWith('[') || /^"[^"\n]+"\s*:/.test(text)) {
        const value = parseJson(text);
        if (Array.isArray(value)) {
            if (!value.every((v) => typeof v === 'string' || typeof v === 'number'))
                throw new MCPPasteError('notStrings');
            return { format: 'args', servers: [{ transport: 'stdio', args: value.map(String) }] };
        }
        return { format: 'json', servers: jsonServers(value) };
    }
    if (!/\s/.test(text) && URL_RE.test(text)) {
        return { format: 'url', servers: [{ name: hostName(text), transport: remoteTransport(text), endpoint: text }] };
    }
    // A copied terminal line may keep its prompt.
    const words = splitShellWords(text.replace(/^[$>]\s+/, ''));
    if (['claude', 'gemini', 'codex'].includes(words[0]) && words[1] === 'mcp') {
        if (words[2] === 'add-json' && words[3] && words[4]) {
            const [server] = jsonServers(parseJson(words[4]));
            return { format: 'registration', servers: [{ ...server, name: words[3] }] };
        }
        if (words[2] === 'add') return { format: 'registration', servers: [registrationServer(words.slice(3))] };
    }
    return { format: 'command', servers: [commandServer(words)] };
}

const pairs = (map: Record<string, string>): MCPKVPair[] => Object.entries(map).map(([key, value]) => ({ key, value }));

// Fills the form from one parsed server. Only what the paste states is
// replaced; a name the user already typed is kept.
export function applyParsedMCPServer(form: MCPSourceFormValue, server: ParsedMCPServer): MCPSourceFormValue {
    const next: MCPSourceFormValue = { ...form };
    if (server.transport) next.transport = server.transport;
    if (server.command !== undefined) next.command = server.command;
    if (server.args !== undefined) next.args = server.args;
    if (server.cwd !== undefined) next.cwd = server.cwd;
    if (server.endpoint !== undefined) next.endpoint = server.endpoint;
    if (server.headers) next.headers = pairs(server.headers);
    if (server.env) {
        // `${KEY}` is the form's "inherit from the server" convention.
        const entries = Object.entries(server.env);
        next.env = pairs(Object.fromEntries(entries.filter(([k, v]) => v !== `\${${k}}`)));
        next.envPassthrough = entries.filter(([k, v]) => v === `\${${k}}`).map(([k]) => k);
    }
    if (server.name && !form.name.trim()) next.name = server.name;
    return next;
}
