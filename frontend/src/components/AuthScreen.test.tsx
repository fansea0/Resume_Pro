import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AuthScreen } from './AuthScreen';

const login = vi.fn();
const register = vi.fn();

vi.mock('../api/auth', () => ({
  login: (...args: unknown[]) => login(...args),
  register: (...args: unknown[]) => register(...args),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('AuthScreen', () => {
  it('logs in with email and password', async () => {
    const onAuthenticated = vi.fn();
    login.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 2 });
    render(<AuthScreen onAuthenticated={onAuthenticated} />);

    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'ada@example.com' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'correct horse battery' } });
    fireEvent.click(screen.getByRole('button', { name: '登录' }));

    await waitFor(() => expect(login).toHaveBeenCalledWith({ email: 'ada@example.com', password: 'correct horse battery' }));
    expect(onAuthenticated).toHaveBeenCalledWith({ id: 1, email: 'ada@example.com', tenantId: 2 });
  });

  it('switches to registration and validates password length before requesting', () => {
    render(<AuthScreen onAuthenticated={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: '注册账号' }));
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'ada@example.com' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'short' } });
    fireEvent.click(screen.getByRole('button', { name: '创建账号' }));

    expect(register).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('密码至少需要 12 个字符。');
  });

  it('shows a request error without exposing a session token', async () => {
    login.mockRejectedValueOnce(new Error('invalid email or password'));
    render(<AuthScreen onAuthenticated={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'ada@example.com' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'correct horse battery' } });
    fireEvent.click(screen.getByRole('button', { name: '登录' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid email or password');
    expect(document.documentElement.innerHTML).not.toContain('eyJ');
  });
});
