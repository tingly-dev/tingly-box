import { Dialog, DialogContent } from '@mui/material';
import { useTranslation } from 'react-i18next';
import DialogHeader from '@/components/DialogHeader';
import LogExplorer from '@/components/LogExplorer';

interface ScenarioLogDialogProps {
    open: boolean;
    onClose: () => void;
    scenario: string;
    /** Dialog title; defaults to "Log". */
    title?: string;
}

/**
 * One agent's Log: its requests (each opens into the request's journey),
 * today's usage, and the system log. The agent's page opens it from its one
 * Log button; every request across agents lives in System → Logs.
 */
const ScenarioLogDialog = ({ open, onClose, scenario, title }: ScenarioLogDialogProps) => {
    const { t } = useTranslation();
    const titleId = 'agent-log-title';
    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="lg"
            fullWidth
            aria-labelledby={titleId}
            slotProps={{ paper: { sx: { height: '80vh' } } }}
        >
            <DialogHeader
                title={title ?? t('agentStatus.log')}
                titleId={titleId}
                closeLabel={t('common.close')}
                onClose={onClose}
            />
            <DialogContent sx={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', pt: 0 }}>
                <LogExplorer scenario={scenario} />
            </DialogContent>
        </Dialog>
    );
};

export default ScenarioLogDialog;
