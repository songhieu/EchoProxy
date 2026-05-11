"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function CopyButton({
  value,
  label,
  className,
  size = "sm",
}: {
  value: string;
  label?: string;
  className?: string;
  size?: "default" | "sm" | "icon";
}) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      type="button"
      variant="outline"
      size={size}
      className={cn(className)}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value);
          setCopied(true);
          toast.success(label ? `${label} copied to clipboard` : "Copied to clipboard");
          setTimeout(() => setCopied(false), 1500);
        } catch {
          toast.error("Failed to copy");
        }
      }}
    >
      {copied ? <Check className="text-green-600" /> : <Copy />}
      {size !== "icon" && <span>{copied ? "Copied" : "Copy"}</span>}
    </Button>
  );
}
