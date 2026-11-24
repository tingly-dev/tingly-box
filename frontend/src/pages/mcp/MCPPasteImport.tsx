import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Button, Stack, TextField, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import { ContentPaste } from '@/components/icons';
import { fontMono } from '@tingly/ui/theme/fonts';
import type { MCPSourceFormValue } from '../../features/mcp/types';
import {
    applyParsedMCPServer,
    MCPPasteError,
    parseMCPPaste,
    type MCPPasteResult,
    type ParsedMCPServer,
} from '../../features/mcp/pasteImport';

const PLACEHOLDER = [
    'npx -y @modelcontextprotocol/server-filesystem ~/Documents',
    'claude mcp add --transport http notion https://mcp.notion.com/mcp',
    '{"mcpServers": {"memory": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-memory"]}}}',
].join('\n');

// "Paste a command or config": reads what an MCP README hands out into the
// connection form below. The form is the preview; nothing is saved here.
export default function MCPPasteImport({
    value,
    onChange,
}: {
    value: MCPSourceFormValue;
    onChange: (next: MCPSourceFormValue) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string, options?: Record<string, unknown>) =>
        t(`mcp.workspace.paste.${key}`, { defaultValue: fallback, ...options });
    const [text, setText] = useState('');
    const [result, setResult] = useState<MCPPasteResult | null>(null);
    const [picked, setPicked] = useState(0);
    const [error, setError] = useState('');
    const pasting = useRef(false);

    // The name is the user's unless it is empty or came from a previous fill.
    const filledName = useRef<string | undefined>(undefined);
    const fill = (server: ParsedMCPServer) => {
        const base = value.name === filledName.current ? { ...value, name: '' } : value;
        const next = applyParsedMCPServer(base, server);
        filledName.current = next.name;
        onChange(next);
    };

    const read = (input: string) => {
        try {
            const parsed = parseMCPPaste(input);
            setResult(parsed);
            setPicked(0);
            setError('');
            fill(parsed.servers[0]);
        } catch (e) {
            setResult(null);
            const code = e instanceof MCPPasteError ? e.code : 'invalidJson';
            const detail = e instanceof MCPPasteError ? e.detail : String(e);
            setError(
                {
                    empty: label('errorEmpty', 'Paste a command, a JSON config or a URL first.'),
                    unclosedQuote: label('errorQuote', 'A quote ({{quote}}) is not closed.', { quote: detail }),
                    invalidJson: label('errorJson', 'This looks like JSON but cannot be read: {{detail}}', { detail }),
                    invalidToml: label('errorToml', 'This looks like a Codex config but this line cannot be read: {{detail}}', {
                        detail,
                    }),
                    noServer: label(
                        'errorNoServer',
                        'No MCP server found in this JSON. Expected "mcpServers", "servers", or an object with "command" or "url".'
                    ),
                    notStrings: label('errorNotStrings', 'An argument list must contain only strings.'),
                    noCommand: label('errorNoCommand', 'No command to run was found.'),
                }[code]
            );
        }
    };

    const summary = (server: ParsedMCPServer) => {
        const parts: string[] = [];
        if (server.transport === 'stdio') {
            if (server.command) parts.push(server.command);
            if (server.args) parts.push(label('summaryArgs', '{{count}} arguments', { count: server.args.length }));
        } else if (server.endpoint) {
            parts.push(server.transport === 'sse' ? 'SSE' : 'Streamable HTTP');
        }
        const envCount = Object.keys(server.env || {}).length;
        if (envCount) parts.push(label('summaryEnv', '{{count}} environment variables', { count: envCount }));
        const headerCount = Object.keys(server.headers || {}).length;
        if (headerCount) parts.push(label('summaryHeaders', '{{count}} headers', { count: headerCount }));
        return parts.join(' · ');
    };

    const formatLabel = result
        ? {
              command: label('formatCommand', 'command line'),
              registration: label('formatRegistration', 'registration command'),
              json: label('formatJson', 'JSON config'),
              toml: label('formatToml', 'Codex config'),
              args: label('formatArgs', 'argument list'),
              url: label('formatUrl', 'URL'),
          }[result.format]
        : '';

    return (
        <Stack spacing={1}>
            <TextField
                multiline
                minRows={2}
                maxRows={8}
                fullWidth
                label={label('label', 'Paste a command or config')}
                placeholder={PLACEHOLDER}
                value={text}
                onPaste={() => {
                    pasting.current = true;
                }}
                onChange={(e) => {
                    setText(e.target.value);
                    setResult(null);
                    setError('');
                    if (pasting.current) read(e.target.value);
                    pasting.current = false;
                }}
                error={!!error}
                helperText={
                    error ||
                    label(
                        'hint',
                        'A command line, claude / codex / gemini mcp add, mcpServers JSON, a Codex config.toml entry, a JSON argument list or a URL. Fills in the fields below.'
                    )
                }
                slotProps={{ htmlInput: { spellCheck: false, 'aria-label': label('label', 'Paste a command or config') } }}
                sx={{ '& textarea': { fontFamily: fontMono } }}
            />
            {!result && (
                <Box>
                    <Button size="small" startIcon={<ContentPaste />} disabled={!text.trim()} onClick={() => read(text)}>
                        {label('fill', 'Fill in fields')}
                    </Button>
                </Box>
            )}
            {result && (
                <Alert severity="success">
                    <Stack spacing={1}>
                        <Typography variant="body2">
                            {result.servers.length > 1
                                ? label('filledMany', 'Found {{count}} servers in this {{format}}. Pick the one to connect:', {
                                      count: result.servers.length,
                                      format: formatLabel,
                                  })
                                : label('filled', 'Read as {{format}}: {{summary}}. Review the fields below, then connect.', {
                                      format: formatLabel,
                                      summary: summary(result.servers[0]),
                                  })}
                        </Typography>
                        {result.servers.length > 1 && (
                            <>
                                <ToggleButtonGroup
                                    exclusive
                                    size="small"
                                    value={picked}
                                    onChange={(_, index: number | null) => {
                                        if (index === null) return;
                                        setPicked(index);
                                        fill(result.servers[index]);
                                    }}
                                    sx={{ flexWrap: 'wrap' }}
                                >
                                    {result.servers.map((server, index) => (
                                        <ToggleButton key={index} value={index}>
                                            {server.name || `#${index + 1}`}
                                        </ToggleButton>
                                    ))}
                                </ToggleButtonGroup>
                                <Typography variant="body2" color="text.secondary">
                                    {summary(result.servers[picked])}
                                </Typography>
                            </>
                        )}
                    </Stack>
                </Alert>
            )}
        </Stack>
    );
}
