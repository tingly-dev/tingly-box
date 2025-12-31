import React from 'react';
import PageLayout from '../../components/PageLayout';
import UseClaudeCodePage from '../UseClaudeCodePage';
import { useFunctionPanelData } from '../../hooks/useFunctionPanelData';

const UseClaudeCodePageWrapper: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        loading,
        notification,
    } = useFunctionPanelData();

    return (
        <PageLayout notification={notification} loading={loading}>
            <UseClaudeCodePage
                showTokenModal={showTokenModal}
                setShowTokenModal={setShowTokenModal}
                token={token}
                showNotification={showNotification}
                providers={providers}
            />
        </PageLayout>
    );
};

export default UseClaudeCodePageWrapper;
