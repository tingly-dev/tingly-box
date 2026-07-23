import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  Typography,
} from '@mui/material';
import { Close as CloseIcon } from '@/components/icons';
import CodeBlock from '@/components/CodeBlock';

interface QuotaRawResponseDialogProps {
  open: boolean;
  onClose: () => void;
  providerName?: string;
  response: unknown;
}

function formatRawResponse(response: unknown): string {
  if (typeof response === 'string') {
    try {
      return JSON.stringify(JSON.parse(response), null, 2);
    } catch {
      return response;
    }
  }

  return JSON.stringify(response, null, 2) ?? String(response);
}

export function QuotaRawResponseDialog({
  open,
  onClose,
  providerName,
  response,
}: QuotaRawResponseDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle component="div">
        <Stack direction="row" alignItems="flex-start" justifyContent="space-between" spacing={2}>
          <Stack spacing={0.25}>
            <Typography variant="h6">Raw quota response</Typography>
            <Typography variant="body2" color="text.secondary">
              {providerName
                ? `Complete successful response returned for ${providerName}.`
                : 'Complete successful response returned by the quota endpoint.'}
            </Typography>
          </Stack>
          <IconButton aria-label="Close raw quota response" onClick={onClose} edge="end">
            <CloseIcon />
          </IconButton>
        </Stack>
      </DialogTitle>
      <DialogContent>
        <CodeBlock
          code={formatRawResponse(response)}
          language="json"
          filename="quota-response.json"
          maxHeight="60vh"
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}
