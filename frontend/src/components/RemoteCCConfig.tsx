import React, { useState, useEffect } from 'react';
import {
    Box,
    Switch,
    FormControlLabel,
    Typography,
    CircularProgress,
    Alert,
    Button,
    Link,
} from '@mui/material';
import { CheckCircle, Cancel, OpenInNew } from '@mui/icons-material';
import api from '@/services/api';

const RemoteCCConfig: React.FC = () => {
    const [enabled, setEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [remoteCCAvailable, setRemoteCCAvailable] = useState(false);

    useEffect(() => {
        loadRemoteCCStatus();
    }, []);

    const loadRemoteCCStatus = async () => {
        try {
            setLoading(true);
            // Check if remote-cc service is available
            const isAvailable = await api.checkRemoteCCAvailable();
            setRemoteCCAvailable(isAvailable);

            // Get the current flag status
            try {
                const flagResult = await api.getScenarioFlag('_global', 'skill_remote_cc');
                setEnabled(flagResult?.data?.value || false);
            } catch {
                setEnabled(false);
            }
        } catch (err) {
            console.error('Failed to load remote-cc status:', err);
        } finally {
            setLoading(false);
        }
    };

    const handleToggle = async (event: React.ChangeEvent<HTMLInputElement>) => {
        const newValue = event.target.checked;
        setEnabled(newValue);
        setSaving(true);
        setError(null);
        setSuccess(null);

        try {
            const result = await api.setScenarioFlag('_global', 'skill_remote_cc', newValue);
            if (result.success) {
                setSuccess(newValue ? 'Remote CC enabled!' : 'Remote CC disabled.');
                setTimeout(() => setSuccess(null), 3000);
            } else {
                setError(result.error || 'Failed to update setting');
                // Revert on error
                setEnabled(!newValue);
            }
        } catch (err) {
            setError('Failed to update setting');
            // Revert on error
            setEnabled(!newValue);
            console.error(err);
        } finally {
            setSaving(false);
        }
    };

    if (loading) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
                <CircularProgress size={24} />
            </Box>
        );
    }

    if (!remoteCCAvailable) {
        return (
            <Alert severity="warning" icon={<Cancel />}>
                <Typography variant="body2">
                    Remote CC service is not available. Make sure the remote-cc service is running on port 18080.
                </Typography>
            </Alert>
        );
    }

    return (
        <Box>
            <FormControlLabel
                control={
                    <Switch
                        checked={enabled}
                        onChange={handleToggle}
                        disabled={saving}
                        color="primary"
                    />
                }
                label={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography variant="body1">
                            Enable Remote CC
                        </Typography>
                        {enabled ? (
                            <CheckCircle color="success" fontSize="small" />
                        ) : (
                            <Cancel color="disabled" fontSize="small" />
                        )}
                    </Box>
                }
            />

            {success && (
                <Alert severity="success" sx={{ mt: 1 }}>
                    {success}
                </Alert>
            )}

            {error && (
                <Alert severity="error" sx={{ mt: 1 }} onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            {enabled && (
                <Alert severity="info" icon={<OpenInNew />} sx={{ mt: 2 }}>
                    <Typography variant="body2">
                        Remote CC is now enabled! A "Remote CC" menu item has appeared in the sidebar.
                        Click it to access the remote Claude Code interface.
                    </Typography>
                </Alert>
            )}

            {!enabled && (
                <Alert severity="warning" icon={<Cancel />} sx={{ mt: 2 }}>
                    <Typography variant="body2">
                        Remote CC is disabled. Toggle the switch above to enable it and access the Remote CC interface.
                    </Typography>
                </Alert>
            )}
        </Box>
    );
};

export default RemoteCCConfig;
