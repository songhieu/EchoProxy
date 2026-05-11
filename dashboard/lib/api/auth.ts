import { redirect } from "next/navigation";
import type { Project, APIKey } from "./types";

const AUTH_API = process.env.AUTH_API_URL ?? "http://localhost:8083";

async function request<T>(path: string, opts: RequestInit & { token?: string } = {}): Promise<T> {
  const headers = new Headers(opts.headers);
  headers.set("Content-Type", "application/json");
  if (opts.token) headers.set("Authorization", `Bearer ${opts.token}`);
  const res = await fetch(AUTH_API + path, { ...opts, headers, cache: "no-store" });
  // Token cookie is stale (user deleted, DB recreated, JWT secret rotated, etc.):
  // clear NextAuth session and bounce back to login instead of crashing.
  if (res.status === 401 && opts.token) {
    redirect("/api/auth/signout?callbackUrl=/login");
  }
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`auth-api ${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

export async function login(email: string, password: string) {
  return request<{ token: string; user: { id: number; email: string } }>(
    "/v1/login",
    { method: "POST", body: JSON.stringify({ email, password }) },
  );
}

export async function signup(email: string, password: string) {
  return request<{ token: string; user: { id: number; email: string } }>(
    "/v1/signup",
    { method: "POST", body: JSON.stringify({ email, password }) },
  );
}

export async function listProjects(token: string) {
  return (await request<Project[] | null>("/v1/projects", { token })) ?? [];
}

export async function createProject(token: string, name: string) {
  return request<Project>("/v1/projects", {
    method: "POST",
    body: JSON.stringify({ name }),
    token,
  });
}

export async function getProject(token: string, projectID: number | string) {
  return request<Project>(`/v1/projects/${projectID}`, { token });
}

export async function updateProjectRetention(
  token: string,
  projectID: number | string,
  retentionDays: number,
) {
  return request<Project>(`/v1/projects/${projectID}`, {
    method: "PATCH",
    body: JSON.stringify({ retention_days: retentionDays }),
    token,
  });
}

export async function deleteProject(token: string, projectID: number | string) {
  // Bypass request<T> because the 204 No Content body breaks JSON parsing.
  const res = await fetch(`${AUTH_API}/v1/projects/${projectID}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (res.status === 401) redirect("/api/auth/signout?callbackUrl=/login");
  if (!res.ok && res.status !== 204) {
    throw new Error(`auth-api ${res.status}: ${await res.text()}`);
  }
}

export async function listAPIKeys(token: string, projectID: number | string) {
  return (await request<APIKey[] | null>(`/v1/projects/${projectID}/keys`, { token })) ?? [];
}

export type APIKeyInput = {
  allowlist: string[];
  body_cap?: number;
  rate_limit_rps?: number;
  redact_rules?: import("./types").RedactRules;
  description?: string;
};

export async function createAPIKey(token: string, projectID: number | string, body: APIKeyInput) {
  return request<APIKey>(`/v1/projects/${projectID}/keys`, {
    method: "POST",
    body: JSON.stringify(body),
    token,
  });
}

export async function getAPIKey(token: string, projectID: number | string, keyID: number | string) {
  return request<APIKey>(`/v1/projects/${projectID}/keys/${keyID}`, { token });
}

export async function updateAPIKey(
  token: string,
  projectID: number | string,
  keyID: number | string,
  body: APIKeyInput,
) {
  return request<APIKey>(`/v1/projects/${projectID}/keys/${keyID}`, {
    method: "PATCH",
    body: JSON.stringify(body),
    token,
  });
}

export async function revokeAPIKey(token: string, projectID: number | string, keyID: number) {
  const res = await fetch(`${AUTH_API}/v1/projects/${projectID}/keys/${keyID}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error(`revoke failed: ${res.status}`);
}
