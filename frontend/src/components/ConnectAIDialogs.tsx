import ConnectProviderDialog from '@/components/ConnectProviderDialog';
import ImportModal from '@/components/ImportModal';
import OAuthDialog from '@/components/OAuthDialog';
import PasteDetectDialog from '@/components/paste-detect/PasteDetectDialog';
import ProviderFormDialog from '@/components/ProviderFormDialog';
import type { UseProviderDialogReturn } from '@/hooks/useProviderDialog';

// The complete "Connect AI" dialog stack, driven by useProviderDialog. Every
// surface that offers the picker renders this once, so all picker cards —
// key / custom / self-hosted / OAuth / import / paste — behave identically
// everywhere instead of each page wiring (and forgetting) its own subset.

interface ConnectAIDialogsProps {
    flow: UseProviderDialogReturn;
    /**
     * Inline surfaces (onboarding) render ProviderListContent themselves:
     * skip the picker dialog and don't offer "← Back to picker" in the form.
     */
    inline?: boolean;
    /** Forwarded to ProviderFormDialog (onboarding first-run copy). */
    isFirstProvider?: boolean;
}

const ConnectAIDialogs: React.FC<ConnectAIDialogsProps> = ({ flow, inline = false, isFirstProvider = false }) => {
    return (
        <>
            {!inline && (
                <ConnectProviderDialog
                    open={flow.connectDialogOpen}
                    onClose={flow.handleCloseConnect}
                    onSelect={flow.handleConnectSelect}
                />
            )}
            <ProviderFormDialog
                open={flow.providerDialogOpen}
                onClose={flow.handleCloseDialog}
                onBack={!inline && flow.fromConnectPicker
                    ? () => { flow.handleCloseDialog(); flow.handleConnectAIClick(); }
                    : undefined}
                onSubmit={flow.handleProviderSubmit}
                onForceAdd={flow.handleProviderForceAdd}
                data={flow.providerFormData}
                onChange={flow.handleFieldChange}
                mode="add"
                isFirstProvider={isFirstProvider}
                optionalEditableToken={flow.optionalEditableToken}
            />
            <OAuthDialog
                open={flow.oauthDialogOpen}
                autoStartProviderId={flow.oauthAutoStartId}
                onClose={flow.handleCloseOAuth}
                onSuccess={flow.handleOAuthSuccess}
            />
            <PasteDetectDialog
                open={flow.pasteDialogOpen}
                onClose={flow.handleClosePasteDialog}
                onPick={flow.handlePastePick}
            />
            <ImportModal
                open={flow.importModalOpen}
                onClose={flow.handleCloseImport}
                onImport={flow.handleImportData}
                loading={flow.importing}
            />
        </>
    );
};

export default ConnectAIDialogs;
