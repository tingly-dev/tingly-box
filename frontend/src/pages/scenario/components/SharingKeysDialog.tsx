import { Key as IconKey } from '@/components/icons';
import {
    Dialog,
    DialogContent,
    DialogTitle,
    Stack,
} from '@mui/material';
import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import SharingKeysTable, { type SharingKey } from '@/components/SharingKeysTable';
import CreateSharingKeyButton from '@/components/CreateSharingKeyButton';
import { useSharingKeyActions } from '@/hooks/useSharingKeyActions';
import type { Team } from '@/types/team';
import TeamKeyScopeAlert from './TeamKeyScopeAlert';

interface SharingKeysDialogProps {
    open: boolean;
    onClose: () => void;
    team: Team;
    teams: Team[];
}

const SharingKeysDialog: React.FC<SharingKeysDialogProps> = ({ open, onClose, team, teams }) => {
    const { t } = useTranslation();

    const [sharingKeys, setSharingKeys] = useState<SharingKey[]>([]);
    const [keysLoading, setKeysLoading] = useState(true);

    // Guards against a stale response for a previously selected team
    // overwriting the keys of the team the user has since switched to.
    const requestedTeamIdRef = useRef<string | null>(null);

    const loadSharingKeys = async () => {
        const requestedTeamId = team.id;
        requestedTeamIdRef.current = requestedTeamId;
        setKeysLoading(true);
        const result = await api.listAPITokens({team_id: requestedTeamId});
        if (requestedTeamIdRef.current !== requestedTeamId) {
            // A newer request for a different team has since been issued; discard this response.
            return;
        }
        if (result.success && result.data) {
            setSharingKeys(result.data.tokens || []);
        }
        setKeysLoading(false);
    };

    useEffect(() => {
        if (open) {
            loadSharingKeys();
        }
    }, [open, team.id]);

    const { tableProps, openCreate, dialogs } = useSharingKeyActions({ teams, onChanged: loadSharingKeys });

    return (
        <>
            <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <Stack direction="row" spacing={1} sx={{
                        alignItems: "center"
                    }}>
                        <IconKey />
                        <span>{t('sharingKeys.titleForTeam', {team: team.name})}</span>
                    </Stack>
                    <CreateSharingKeyButton team={team} onClick={() => openCreate(team)} />
                </DialogTitle>
                <DialogContent>
                    <Stack spacing={2}>
                        <TeamKeyScopeAlert team={team} />
                        <SharingKeysTable
                            tokens={sharingKeys}
                            loading={keysLoading}
                            {...tableProps}
                        />
                    </Stack>
                </DialogContent>
            </Dialog>
            {dialogs}
        </>
    );
};

export default SharingKeysDialog;
