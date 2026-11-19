import { useCallback, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
} from '@mui/material';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import { ProviderListContent } from '@/components/ConnectProviderDialog';
import { useProviderDialog } from '@/hooks/useProviderDialog';
import { useNotify } from '@/hooks/useNotify.ts';

/**
 * ProvidersCard — browse the provider catalog and connect one. This is what
 * the old standalone Onboarding page used to be: the lightbulb Help entry is
 * the new onboarding front door, and this is the one piece of Onboarding's
 * content that still needed a home (the shared Connect AI *dialog* —
 * form/OAuth/paste/import — already exists everywhere else; this is
 * specifically the *browsable list* view of it). Rendered inside a
 * CollapsibleCard on HelpPage; title/description live in that shared
 * accordion header, not here.
 *
 * Unlike CredentialPage's Connect AI (a picker hidden behind a button, so
 * empty-state entry points need a `?dialog=add` deep link to open it), the
 * list here is always visible as soon as the card is expanded — first-run
 * and empty-state entry points just need to land on this page (HelpPage
 * expands this section by default), no query param needed.
 */
export const ProvidersCard = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();
    const [browseQuery, setBrowseQuery] = useState('');
    const [successDialogOpen, setSuccessDialogOpen] = useState(false);

    const showNotification = useCallback((message: string, severity: 'success' | 'error') => {
        notify[severity](message);
    }, [notify]);

    // The same Connect AI flow every other surface uses: the provider list is
    // rendered inline (card content instead of a picker dialog), and every
    // card — key / custom / self-hosted / OAuth / import / paste & detect —
    // routes through the shared hook + dialog stack.
    const connectAI = useProviderDialog(showNotification, {
        onProviderAdded: () => setSuccessDialogOpen(true),
    });

    const handleGoToAgents = () => {
        setSuccessDialogOpen(false);
        navigate('/agent');
    };

    const handleStayHere = () => {
        setSuccessDialogOpen(false);
        notify.success(t('onboarding.success', { defaultValue: 'Provider added successfully! You can now create scenarios.' }));
    };

    return (
        <>
            <ProviderListContent
                onSelect={connectAI.handleConnectSelect}
                query={browseQuery}
                onQueryChange={setBrowseQuery}
                hideOfficialInfo={true}
                showDetails={true}
                wide={true}
            />

            {/* Shared Connect AI dialog stack (form / OAuth / paste / import /
                cloud). inline: the provider list above is the picker, so no
                picker dialog and no "← Back to picker" in the form. */}
            <ConnectAIDialogs flow={connectAI} inline isFirstProvider />

            <Dialog
                open={successDialogOpen}
                onClose={() => setSuccessDialogOpen(false)}
                aria-labelledby="providers-success-dialog-title"
                aria-describedby="providers-success-dialog-description"
            >
                <DialogTitle id="providers-success-dialog-title">
                    {t('onboarding.dialog.title', { defaultValue: 'Provider Added' })}
                </DialogTitle>
                <DialogContent>
                    <DialogContentText id="providers-success-dialog-description">
                        {t('onboarding.dialog.message', { defaultValue: 'Your AI provider has been added successfully. Would you like to go to the agents page to start using it?' })}
                    </DialogContentText>
                </DialogContent>
                <DialogActions>
                    <Button onClick={handleStayHere}>
                        {t('onboarding.dialog.stay', { defaultValue: 'Stay Here' })}
                    </Button>
                    <Button onClick={handleGoToAgents} variant="contained" autoFocus>
                        {t('onboarding.dialog.goToAgents', { defaultValue: 'Go to Agents' })}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default ProvidersCard;
