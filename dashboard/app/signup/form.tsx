"use client";

import { useState } from "react";
import { signIn } from "next-auth/react";
import { useRouter } from "next/navigation";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { signupAction } from "./actions";

export function SignupForm() {
  const router = useRouter();
  const [pending, setPending] = useState(false);

  return (
    <form
      className="space-y-4"
      onSubmit={async (e) => {
        e.preventDefault();
        setPending(true);
        const data = new FormData(e.currentTarget);
        const email = String(data.get("email"));
        const password = String(data.get("password"));
        const res = await signupAction(email, password);
        if (!res.ok) {
          toast.error(res.error);
          setPending(false);
          return;
        }
        const signed = await signIn("credentials", { email, password, redirect: false });
        setPending(false);
        if (signed?.error) {
          toast.error("Account created — please sign in.");
          router.push("/login");
          return;
        }
        router.push("/projects");
        router.refresh();
      }}
    >
      <div className="space-y-2">
        <Label htmlFor="email">Email</Label>
        <Input id="email" name="email" type="email" required autoComplete="email" />
      </div>
      <div className="space-y-2">
        <Label htmlFor="password">Password</Label>
        <Input id="password" name="password" type="password" required minLength={8} autoComplete="new-password" />
        <p className="text-xs text-muted-foreground">8 characters minimum.</p>
      </div>
      <Button type="submit" className="w-full" disabled={pending}>
        {pending && <Loader2 className="animate-spin" />}
        Create account
      </Button>
    </form>
  );
}
