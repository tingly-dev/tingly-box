import React from 'react';
import PageLayout from '../../components/PageLayout';
import UseAnthropicPage from '../UseAnthropicPage';
import { useFunctionPanelData } from '../../hooks/useFunctionPanelData';

const UseAnthropicPageWrapper: React.FC = () => {
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
            <UseAnthropicPage
                showTokenModal={showTokenModal}
                setShowTokenModal={setShowTokenModal}
                token={token}
                showNotification={showNotification}
                providers={providers}
            />
        </PageLayout>
    );
};

export default UseAnthropicPageWrapper;
