import { apiFetch } from './client';

export interface AuthUser {
  id: number;
  email: string;
  tenantId: number;
}

export interface Credentials {
  email: string;
  password: string;
}

interface AuthResponse {
  user: AuthUser;
}

export async function getCurrentUser(): Promise<AuthUser> {
  return (await apiFetch<AuthResponse>('/api/auth/me')).user;
}

export async function login(credentials: Credentials): Promise<AuthUser> {
  return (await apiFetch<AuthResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify(credentials),
  })).user;
}

export async function register(credentials: Credentials): Promise<AuthUser> {
  return (await apiFetch<AuthResponse>('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify(credentials),
  })).user;
}

export function logout(): Promise<void> {
  return apiFetch<void>('/api/auth/logout', { method: 'POST' });
}
