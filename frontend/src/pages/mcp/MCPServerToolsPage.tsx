import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { IconButton, Stack } from '@mui/material';
import { Refresh } from '@/components/icons';
import { PageLayout } from '@/components/PageLayout';
import MCPSourceDrawer from './MCPSourceDrawer';
import MCPToolsPanel from './MCPToolsPanel';
import MCPWorkspaceAlerts from './MCPWorkspaceAlerts';
import { isServerToolSource } from './toolPresentation';
import { useMCPWorkspace } from './useMCPWorkspace';

// Tools Tingly Box runs itself: configure, then enable, without an MCP profile.
export default function MCPServerToolsPage() {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [search, setSearch] = useSearchParams();
    const ws = useMCPWorkspace();
    const { config, routing, loading, checking, busy } = ws;
    const sources = (config.sources || []).filter(isServerToolSource);
    const selectedId = search.get('source');
    const closeSheet = () => {
        const next = new URLSearchParams(search);
        next.delete('source');
        setSearch(next, { replace: true });
    };
    return (
        <PageLayout
            loading={loading}
            title={label('serverToolsTitle', 'Server Tools')}
            subtitle={label(
                'serverToolsHint',
                'Provided and run by Tingly Box on the server. Configure each service and enable its tools; no MCP connection or profile is needed.'
            )}
            rightAction={
                <IconButton
                    aria-label={label('refresh', 'Refresh connections')}
                    disabled={busy || checking}
                    onClick={() => void ws.run(ws.reload)}
                >
                    <Refresh />
                </IconButton>
            }
        >
            <Stack spacing={2}>
                <MCPWorkspaceAlerts ws={ws} serverTools />
                <MCPToolsPanel
                    sources={sources}
                    routes={(routing?.sources || []).filter((route) => sources.some((source) => source.id === route.id))}
                    loading={checking}
                    enabled
                    saveSource={ws.saveSource}
                    onConfigureSource={(id) => setSearch({ source: id })}
                />
            </Stack>
            <MCPSourceDrawer ws={ws} selectedId={selectedId} onClose={closeSheet} />
        </PageLayout>
    );
}
