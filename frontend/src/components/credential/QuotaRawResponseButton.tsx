import { useState } from 'react';
import { Button, Tooltip } from '@mui/material';
import { Code as CodeIcon } from '@/components/icons';
import { QuotaRawResponseDialog } from './QuotaRawResponseDialog';

interface QuotaRawResponseButtonProps {
  providerName?: string;
  /** The stored upstream payload; the button renders nothing without one. */
  response: unknown;
}

/**
 * "Details" entry to the raw upstream quota response, shared by every surface
 * that lists quota windows — the payload is often the only explanation for a
 * missing or odd-looking figure (e.g. a refused upstream keeps its body here).
 */
export function QuotaRawResponseButton({ providerName, response }: QuotaRawResponseButtonProps) {
  const [open, setOpen] = useState(false);
  if (response === undefined || response === null) return null;

  return (
    <>
      <Tooltip title="View raw quota response" arrow>
        <Button
          aria-label="View raw quota response"
          size="small"
          variant="text"
          startIcon={<CodeIcon sx={{ fontSize: 16 }} />}
          onClick={() => setOpen(true)}
          sx={{
            flexShrink: 0,
            minWidth: 0,
            px: 0.75,
            color: 'text.secondary',
            fontSize: '0.7rem',
            fontWeight: 400,
            textTransform: 'none',
            whiteSpace: 'nowrap',
            '& .MuiButton-startIcon': { mr: 0.5 },
            '&:hover': {
              bgcolor: 'action.hover',
              color: 'text.primary',
            },
          }}
        >
          Details
        </Button>
      </Tooltip>
      <QuotaRawResponseDialog
        open={open}
        onClose={() => setOpen(false)}
        providerName={providerName}
        response={response}
      />
    </>
  );
}
