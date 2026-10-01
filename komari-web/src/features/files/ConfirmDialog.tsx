import FileDialog, { type FileDialogProps } from "./FileDialog";

export function ConfirmDialog({ onConfirm, ...props }: Omit<FileDialogProps, "fields" | "onSubmit"> & {
  onConfirm: FileDialogProps["onSubmit"];
}) {
  return <FileDialog {...props} onSubmit={onConfirm} />;
}

export default ConfirmDialog;
