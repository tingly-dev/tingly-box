import { useTranslation } from 'react-i18next';
import { Alert, Button } from '@mui/material';
import type { MCPWorkspace } from './useMCPWorkspace';

// Load, status and execution-off notices shared by the Tools and Profiles pages.
export default function MCPWorkspaceAlerts({ ws, serverTools = false }: { ws: MCPWorkspace; serverTools?: boolean }) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    return (
        <>
            {ws.error && <Alert severity="error">{ws.error}</Alert>}
            {ws.routeError && (
                <Alert
                    severity="warning"
                    action={
                        <Button disabled={ws.checking} onClick={() => void ws.refreshRoutes()}>
                            {label('retry', 'Retry connection')}
                        </Button>
                    }
                >
                    {label(
                        'statusUnavailable',
                        'Connection status could not be refreshed. Saved settings remain available.'
                    )}{' '}
                    {ws.routeError}
                </Alert>
            )}
            {!ws.enabled && !serverTools && (
                <Alert
                    severity="info"
                    action={
                        <Button disabled={ws.busy} onClick={() => void ws.enableExecution()}>
                            {label('enableMCP', 'Enable MCP')}
                        </Button>
                    }
                >
                    {label(
                        'executionOff',
                        'MCP execution is off. Your configuration is saved; enable execution to use it.'
                    )}
                </Alert>
            )}
        </>
    );
}
