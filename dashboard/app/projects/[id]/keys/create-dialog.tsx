"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangle, KeyRound, Loader2, Plus } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { CopyButton } from "@/components/copy-button";
import { createKeyAction } from "./actions";

export function CreateKeyDialog({ projectId }: { projectId: string }) {
  const [open, setOpen] = useState(false);
  const [pending, start] = useTransition();
  const [created, setCreated] = useState<string | null>(null);
  const router = useRouter();

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (!v) {
          setCreated(null);
          if (created) router.refresh();
        }
      }}
    >
      <DialogTrigger asChild>
        <Button>
          <Plus /> New API key
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <KeyRound className="h-5 w-5" /> Save your API key
              </DialogTitle>
              <DialogDescription>
                This is the only time you&apos;ll see the full key. Copy it now and store it somewhere safe.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-md border border-warning/40 bg-warning/5 p-4">
              <div className="mb-2 flex items-center gap-2 text-xs font-medium text-warning">
                <AlertTriangle className="h-4 w-4" />
                One-time view — not stored, not recoverable
              </div>
              {/* Click the code itself selects all so users without clipboard
                  permission can still ⌘C. */}
              <code
                className="block cursor-text select-all break-all rounded bg-background px-3 py-2 font-mono text-xs"
                onClick={(e) => {
                  const r = document.createRange();
                  r.selectNodeContents(e.currentTarget);
                  const sel = window.getSelection();
                  sel?.removeAllRanges();
                  sel?.addRange(r);
                }}
              >
                {created}
              </code>
              <div className="mt-3 flex items-center justify-end">
                <CopyButton value={created} label="API key" />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={() => setOpen(false)}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              const fd = new FormData(e.currentTarget);
              start(async () => {
                const res = await createKeyAction(projectId, null, fd);
                if (!res.ok) {
                  toast.error(res.error || "Failed to create key");
                  return;
                }
                setCreated(res.raw ?? null);
                toast.success("API key created");
              });
            }}
          >
            <DialogHeader>
              <DialogTitle>New API key</DialogTitle>
              <DialogDescription>
                Configure the upstream allowlist and body capture limit. Both can be left empty to use sane defaults.
              </DialogDescription>
            </DialogHeader>
            <div className="my-4 space-y-4">
              <div className="space-y-2">
                <Label htmlFor="description">Description</Label>
                <Input
                  id="description"
                  name="description"
                  placeholder="Production checkout API"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="allowlist">Allowlist (comma-separated hostnames)</Label>
                <Textarea
                  id="allowlist"
                  name="allowlist"
                  placeholder="api.example.com, api.payments.example.com"
                  rows={2}
                />
                <p className="text-xs text-muted-foreground">
                  Empty = allow all hosts. Recommended for production: list every upstream the key may forward to.
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="body_cap">Body capture cap (bytes)</Label>
                <Input id="body_cap" name="body_cap" type="number" min={0} placeholder="65536" />
                <p className="text-xs text-muted-foreground">
                  0 (or empty) = use the proxy default of 64 KB.
                </p>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={pending}>
                {pending && <Loader2 className="animate-spin" />}
                Create key
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
