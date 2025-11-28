import { Cancel, CheckCircle } from '@mui/icons-material';
import { Button, Stack, Typography } from '@mui/material';
import UnifiedCard from './UnifiedCard';
import { api } from '../services/api';

interface ServerStatusCardProps {
    serverStatus: any;
    onLoadServerStatus: () => Promise<void>;
}

const ServerStatusCard = ({ serverStatus, onLoadServerStatus }: ServerStatusCardProps) => {
    const handleStartServer = async () => {
        const port = prompt('Enter port (8080):', '8080');
        if (port) {
            const result = await api.startServer(parseInt(port));
            if (result.success) {
                alert(result.message);
                setTimeout(onLoadServerStatus, 1000);
            } else {
                alert(result.error);
            }
        }
    };

    const handleStopServer = async () => {
        if (confirm('Are you sure you want to stop the server?')) {
            const result = await api.stopServer();
            if (result.success) {
                alert(result.message);
                setTimeout(onLoadServerStatus, 1000);
            } else {
                alert(result.error);
            }
        }
    };

    const handleRestartServer = async () => {
        const port = prompt('Enter port (8080):', '8080');
        if (port) {
            const result = await api.restartServer(parseInt(port));
            if (result.success) {
                alert(result.message);
                setTimeout(onLoadServerStatus, 1000);
            } else {
                alert(result.error);
            }
        }
    };

    return (
        <UnifiedCard
            title="Server Status"
            subtitle="Monitor and control server operations"
            size="medium"
        >
            {serverStatus ? (
                <Stack spacing={2}>
                    <Stack direction="row" alignItems="center" spacing={1}>
                        {serverStatus.server_running ? (
                            <CheckCircle color="success" />
                        ) : (
                            <Cancel color="error" />
                        )}
                        <Typography variant="body1">
                            <strong>Status:</strong> {serverStatus.server_running ? 'Running' : 'Stopped'}
                        </Typography>
                    </Stack>
                    <Typography variant="body2">
                        <strong>Port:</strong> {serverStatus.port}
                    </Typography>
                    <Typography variant="body2">
                        <strong>Providers:</strong> {serverStatus.providers_enabled}/{serverStatus.providers_total}
                    </Typography>
                    {serverStatus.uptime && (
                        <Typography variant="body2">
                            <strong>Uptime:</strong> {serverStatus.uptime}
                        </Typography>
                    )}
                    <Stack direction="row" spacing={2}>
                        <Button
                            variant="contained"
                            color="success"
                            onClick={handleStartServer}
                            disabled={serverStatus.server_running}
                        >
                            Start
                        </Button>
                        <Button
                            variant="contained"
                            color="error"
                            onClick={handleStopServer}
                            disabled={!serverStatus.server_running}
                        >
                            Stop
                        </Button>
                        <Button variant="contained" onClick={handleRestartServer}>
                            Restart
                        </Button>
                    </Stack>
                </Stack>
            ) : (
                <Typography color="text.secondary">Loading...</Typography>
            )}
        </UnifiedCard>
    );
};

export default ServerStatusCard;
