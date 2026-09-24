import { useCallback, useEffect } from 'react';
import { Box } from '@mui/material';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement';
import { useNotify } from '@/hooks/useNotify';
import ImageGenPlaygroundCard from './components/ImageGenPlaygroundCard';

const scenario = 'imagegen';

// The image work surface. It has no rules of its own: it reads the imagegen
// scenario's rules (edited on the sibling Image API page) and every run goes
// through /tingly/imagegen, the same path a client would take. The page
// never blocks on a skeleton — the card renders its own "no model yet"
// state with a way to the rules.
//
// On lg it is a workbench, not a scrolling page: it takes exactly the
// content area's height, and the panels inside divide it. Below the minimum
// height it stops shrinking and the content area scrolls instead of squashing
// the prompt. Smaller breakpoints keep the stacked, scrolling layout.
// See .design/image-layout.md §6.
const WORKBENCH_MIN_HEIGHT = 600;
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
        <Box sx={{ height: { lg: '100%' }, minHeight: { lg: WORKBENCH_MIN_HEIGHT } }}>
            <ImageGenPlaygroundCard
                rules={rules}
                loadingRules={loadingRule}
                showNotification={showNotification}
            />
        </Box>
    );
};

export default ImagePlaygroundPage;
