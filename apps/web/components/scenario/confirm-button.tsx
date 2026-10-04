"use client";

import { Trash2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";

/** Two-step destructive button: the first click arms it, the second confirms. */
export function ConfirmButton({
  label,
  confirmLabel = "Confirm delete",
  onConfirm,
  disabled,
  iconOnly,
}: {
  label: string;
  confirmLabel?: string;
  onConfirm: () => void;
  disabled?: boolean;
  iconOnly?: boolean;
}) {
  const [armed, setArmed] = useState(false);

  if (armed) {
    return (
      <span className="inline-flex items-center gap-1">
        <Button
          type="button"
          variant="destructive"
          size="sm"
          autoFocus
          onClick={() => {
            setArmed(false);
            onConfirm();
          }}
        >
          {confirmLabel}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setArmed(false)}>
          Cancel
        </Button>
      </span>
    );
  }
  return (
    <Button
      type="button"
      variant="ghost"
      size={iconOnly ? "icon-sm" : "sm"}
      className="text-muted-foreground hover:text-destructive"
      disabled={disabled}
      onClick={() => setArmed(true)}
    >
      <Trash2 aria-hidden />
      {iconOnly ? <span className="sr-only">{label}</span> : label}
    </Button>
  );
}
