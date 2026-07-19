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
import { useTranslation } from 'react-i18next';

interface ScenarioLogDialogProps {
    open: boolean;
    onClose: () => void;
    scenario: string;
}

const ScenarioLogDialog = ({ open, onClose, scenario, initialScenario }: ScenarioLogDialogProps) => {
    const { t } = useTranslation();
    return (
        <Dialog open={open} onClose={onClose} maxWidth="xl" fullWidth slotProps={{
            paper: { sx: { height: '80vh' } }
        }}>
            <DialogTitle sx={{ pb: 1 }}>
                <Stack
                    direction="row"
                    sx={{
                        alignItems: "center",
                        justifyContent: "space-between"
                    }}>
                    <Stack direction="row" spacing={1} sx={{
                        alignItems: "center"
                    }}>
                        <Typography variant="h6">{t('templateActions.troubleshoot')}</Typography>
                        <Chip label={scenario} size="small" variant="outlined" sx={{ fontSize: '0.72rem', height: 22 }} />
                    </Stack>
                    <IconButton size="small" onClick={onClose}>
                        <CloseIcon />
                    </IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent sx={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', pt: 1.5 }}>
                <LogExplorer initialScenario={scenario} />
            </DialogContent>
        </Dialog>
    );
};

export default ScenarioLogDialog;
