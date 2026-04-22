/**
 * Shared hook for dialog state management.
 * Standardizes dialog open/close patterns across 20+ files.
 */
import { useCallback, useState } from 'react';

export interface DialogStateResult {
  open: boolean;
  handleOpen: () => void;
  handleClose: () => void;
  toggleOpen: () => void;
  setOpen: (open: boolean) => void;
}

/**
 * Standard dialog state management hook.
 * Handles open/close state with optional blur effect support.
 */
export function useDialogState(initialState = false): DialogStateResult {
  const [open, setOpen] = useState(initialState);

  const handleOpen = useCallback(() => {
    setOpen(true);
  }, []);

  const handleClose = useCallback(() => {
    setOpen(false);
  }, []);

  const toggleOpen = useCallback(() => {
    setOpen(prev => !prev);
  }, []);

  return {
    open,
    handleOpen,
    handleClose,
    toggleOpen,
    setOpen,
  };
}

/**
 * Dialog state with blur effect hook.
 * Adds a blur class to the document body when dialog is open.
 */
export function useDialogStateWithBlur(initialState = false): DialogStateResult {
  const [open, setOpen] = useState(initialState);

  const handleOpen = useCallback(() => {
    setOpen(true);
    document.body.classList.add('dialog-blur');
  }, []);

  const handleClose = useCallback(() => {
    setOpen(false);
    document.body.classList.remove('dialog-blur');
  }, []);

  const toggleOpen = useCallback(() => {
    setOpen(prev => {
      const newState = !prev;
      if (newState) {
        document.body.classList.add('dialog-blur');
      } else {
        document.body.classList.remove('dialog-blur');
      }
      return newState;
    });
  }, []);

  return {
    open,
    handleOpen,
    handleClose,
    toggleOpen,
    setOpen,
  };
}
