import type { NextAuthOptions } from "next-auth";
import Credentials from "next-auth/providers/credentials";
import { login } from "@/lib/api/auth";

export const authOptions: NextAuthOptions = {
  providers: [
    Credentials({
      credentials: {
        email: { label: "Email", type: "email" },
        password: { label: "Password", type: "password" },
      },
      async authorize(creds) {
        if (!creds?.email || !creds?.password) return null;
        try {
          const { token, user } = await login(creds.email, creds.password);
          return { id: String(user.id), email: user.email, apiToken: token } as any;
        } catch {
          return null;
        }
      },
    }),
  ],
  pages: { signIn: "/login" },
  callbacks: {
    async jwt({ token, user }) {
      if (user) (token as any).apiToken = (user as any).apiToken;
      return token;
    },
    async session({ session, token }) {
      (session as any).token = (token as any).apiToken;
      return session;
    },
  },
  session: { strategy: "jwt" },
};
