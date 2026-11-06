import GuideDrawer from '@/components/GuideDrawer';
import { HelpOutline } from '@/components/icons';
import { Button } from '@mui/material';
import type { ReactNode } from 'react';
import { useState } from 'react';

interface GuideActionProps {
    label: string;
    title: string;
    description?: string;
    children: ReactNode;
    width?: number;
    primaryAction?: {
        label: string;
        onClick: () => void;
    };
}

/**
 * Standard guide entry for a primary work surface. It owns both the compact
 * card action and the responsive drawer so pages only supply guide content.
 */
const GuideAction = ({
    label,
    title,
    description,
    children,
    width,
    primaryAction,
}: GuideActionProps) => {
    const [open, setOpen] = useState(false);

    return (
        <>
            <Button
                variant="outlined"
                size="small"
                startIcon={<HelpOutline />}
                onClick={() => setOpen(true)}
                sx={{ whiteSpace: 'nowrap' }}
            >
                {label}
            </Button>
            <GuideDrawer
                open={open}
                title={title}
                description={description}
                width={width}
                onClose={() => setOpen(false)}
                actionLabel={primaryAction?.label}
                onAction={primaryAction?.onClick}
            >
                {children}
            </GuideDrawer>
        </>
    );
};

export default GuideAction;
