import React, { useEffect, useState } from 'react';
import {
    Dialog, DialogTitle, DialogContent, DialogActions,
    Button, Box, Typography, CircularProgress, Chip, Stack,
    List, ListItem, ListItemText, ListItemSecondaryAction,
} from '@mui/material';
import { CheckCircle, RadioButtonUnchecked, Computer } from '@mui/icons-material';
import { api } from '@/services/api';

interface LocalProvider {
    id: string;
    name: string;
    url: string;
    detected: boolean;
}

interface LocalModelDialogProps {
    open: boolean;
    onClose: () => void;
    onConnect: (provider: { id: string; name: string; url: string }) => void;
}

const LocalModelDialog: React.FC<LocalModelDialogProps> = ({ open, onClose, onConnect }) => {
    const [providers, setProviders] = useState<LocalProvider[]>([]);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        if (!open) return;
        setLoading(true);
        api.probeLocalModels().then(({ results }) => {
            setProviders(results);
            setLoading(false);
        });
    }, [open]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                <Stack direction="row" alignItems="center" spacing={1}>
                    <Computer fontSize="small" />
                    <span>Connect Local Model</span>
                </Stack>
            </DialogTitle>
            <DialogContent dividers>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    Tingly Box detected the following local inference servers.
                    The base URL and token are pre-filled but editable — change the port if you
                    customised it.
                </Typography>

                {loading ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
                        <CircularProgress size={28} />
                    </Box>
                ) : (
                    <List disablePadding>
                        {providers.map((p) => (
                            <ListItem
                                key={p.id}
                                divider
                                sx={{ py: 1.5 }}
                            >
                                <ListItemText
                                    primary={
                                        <Stack direction="row" alignItems="center" spacing={1}>
                                            <Typography fontWeight={500}>{p.name}</Typography>
                                            {p.detected ? (
                                                <Chip
                                                    icon={<CheckCircle />}
                                                    label="Running"
                                                    size="small"
                                                    color="success"
                                                    variant="outlined"
                                                />
                                            ) : (
                                                <Chip
                                                    icon={<RadioButtonUnchecked />}
                                                    label="Not detected"
                                                    size="small"
                                                    variant="outlined"
                                                />
                                            )}
                                        </Stack>
                                    }
                                    secondary={
                                        <Typography variant="caption" color="text.secondary">
                                            {p.url}
                                        </Typography>
                                    }
                                />
                                <ListItemSecondaryAction>
                                    <Button
                                        size="small"
                                        variant={p.detected ? 'contained' : 'outlined'}
                                        onClick={() => { onConnect(p); onClose(); }}
                                    >
                                        Connect
                                    </Button>
                                </ListItemSecondaryAction>
                            </ListItem>
                        ))}
                    </List>
                )}
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose}>Cancel</Button>
            </DialogActions>
        </Dialog>
    );
};

export default LocalModelDialog;
