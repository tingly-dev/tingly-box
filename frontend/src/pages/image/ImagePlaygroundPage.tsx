import { useCallback, useEffect } from 'react';
import CardGrid from '@/components/CardGrid.tsx';
import PageLayout from '@/components/PageLayout';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement';
import { useNotify } from '@/hooks/useNotify';
import ImageGenPlaygroundCard from './components/ImageGenPlaygroundCard';

const scenario = 'imagegen';

// The image work surface. It has no rules of its own: it reads the imagegen
// scenario's rules (edited on the sibling Image API page) and every run goes
// through /tingly/imagegen, the same path a client would take. The page
// never blocks on a skeleton — the card renders its own "no model yet"
// state with a way to the rules. See .design/image-layout.md.
const ImagePlaygroundPage: React.FC = () => {
    const { rules, loadingRule, loadRules } = useRuleManagement();
    const { notify } = useNotify();

    useEffect(() => {
        void loadRules(scenario);
    }, [loadRules]);

    const showNotification = useCallback(
        (message: string, severity: 'success' | 'info' | 'warning' | 'error') => { notify(severity, message); },
        [notify],
    );

    return (
        <PageLayout loading={false}>
            <CardGrid>
                <ImageGenPlaygroundCard
                    rules={rules}
                    loadingRules={loadingRule}
                    showNotification={showNotification}
                />
            </CardGrid>
        </PageLayout>
    );
};

export default ImagePlaygroundPage;
