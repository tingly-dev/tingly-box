import {
    Chip,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import { Close as CloseIcon } from '@/components/icons';
import LogExplorer from '@/components/LogExplorer';

interface ScenarioLogDialogProps {
    open: boolean;
    onClose: () => void;
    scenario: string;
}

const ScenarioLogDialog = ({ open, onClose, scenario }: ScenarioLogDialogProps) => {
    return (
        <Dialog open={open} onClose={onClose} maxWidth="xl" fullWidth PaperProps={{ sx: { height: '80vh' } }}>
            <DialogTitle sx={{ pb: 1 }}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Stack direction="row" alignItems="center" spacing={1}>
                        <Typography variant="h6">Logs</Typography>
                        <Chip label={scenario} size="small" variant="outlined" sx={{ fontSize: '0.72rem', height: 22 }} />
                    </Stack>
                    <IconButton size="small" onClick={onClose}>
                        <CloseIcon />
                    </IconButton>
                </Stack>
            </DialogTitle>

            <DialogContent sx={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', pt: 1.5 }}>
                <LogExplorer lockedScenario={scenario} />
            </DialogContent>
        </Dialog>
    );
};

export default ScenarioLogDialog;
