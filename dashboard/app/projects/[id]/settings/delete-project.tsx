"use client";

import { useState, useTransition } from "react";
import { AlertTriangle, Loader2, Trash2 } from "lucide-react";
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
import { deleteProjectAction } from "./actions";

// Deletes the project after the user types its exact name — mirrors GitHub /
// Stripe destructive-action UX. Cascades to api_keys (Postgres FK); ClickHouse
// events stay until retention TTL expires.
export function DeleteProjectButton({
  projectId,
  projectName,
}: {
  projectId: number;
  projectName: string;
}) {
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const [pending, start] = useTransition();
  const armed = typed === projectName;

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (!v) setTyped("");
      }}
    >
      <DialogTrigger asChild>
        <Button variant="destructive">
          <Trash2 /> Delete project
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle className="h-5 w-5" />
            Delete project — irreversible
          </DialogTitle>
          <DialogDescription>
            Drops the project and all API keys belonging to it. Captured
            events stay in ClickHouse until their retention window expires.
            This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="my-4 space-y-3">
          <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm">
            <p className="font-medium">
              To confirm, type the project name:{" "}
              <code className="font-mono">{projectName}</code>
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="confirm-name">Project name</Label>
            <Input
              id="confirm-name"
              autoFocus
              autoComplete="off"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={projectName}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={!armed || pending}
            onClick={() => {
              start(async () => {
                const res = await deleteProjectAction(projectId);
                // Action calls redirect("/projects") on success — code below
                // only runs when the action returned an error object.
                if (res && !res.ok) toast.error(res.error || "Delete failed");
              });
            }}
          >
            {pending && <Loader2 className="animate-spin" />}
            <Trash2 /> Delete forever
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
