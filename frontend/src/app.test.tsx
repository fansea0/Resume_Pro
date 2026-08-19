import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './app';
import { createSampleResumeDocument } from './types/resume';

const getCurrentUser = vi.fn();
const logout = vi.fn();
const listResumes = vi.fn();
const updateResume = vi.fn();
const getTemplateStatus = vi.fn();
const listAdminTemplates = vi.fn();
const disableTemplate = vi.fn();
const login = vi.fn();
const listTargetJobs = vi.fn();
let unauthorizedListener: (() => void) | null = null;

vi.mock('./api/auth', () => ({
  getCurrentUser: (...args: unknown[]) => getCurrentUser(...args),
  logout: (...args: unknown[]) => logout(...args),
  login: (...args: unknown[]) => login(...args),
  register: vi.fn(),
}));

vi.mock('./api/resumes', () => ({
  listResumes: (...args: unknown[]) => listResumes(...args),
  createResume: vi.fn(),
  updateResume: (...args: unknown[]) => updateResume(...args),
  deleteResume: vi.fn(),
  documentFromStored: (resume: { content: string; title: string; templateId: string }) => ({
    ...JSON.parse(resume.content),
    title: resume.title,
    templateId: resume.templateId,
  }),
}));

vi.mock('./api/templates', () => ({
  getTemplateStatus: (...args: unknown[]) => getTemplateStatus(...args),
  listAdminTemplates: (...args: unknown[]) => listAdminTemplates(...args),
  disableTemplate: (...args: unknown[]) => disableTemplate(...args),
}));

vi.mock('./api/targetJobs', () => ({
  listTargetJobs: (...args: unknown[]) => listTargetJobs(...args),
  createTargetJob: vi.fn(),
  updateTargetJob: vi.fn(),
  deleteTargetJob: vi.fn(),
}));

vi.mock('./api/client', () => ({
  onUnauthorized: (listener: () => void) => {
    unauthorizedListener = listener;
    return () => {
      if (unauthorizedListener === listener) unauthorizedListener = null;
    };
  },
}));

afterEach(() => {
  cleanup();
  window.history.replaceState({}, '', '/');
});

describe('application authentication gate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getCurrentUser.mockReset();
    logout.mockReset();
    login.mockReset();
    getTemplateStatus.mockReset();
    listAdminTemplates.mockReset();
    disableTemplate.mockReset();
    listTargetJobs.mockReset();
    unauthorizedListener = null;
    listResumes.mockResolvedValue([]);
    getTemplateStatus.mockResolvedValue({ disabledTemplateIds: [] });
    listTargetJobs.mockResolvedValue([]);
  });

  it('shows login when startup has no session', async () => {
    getCurrentUser.mockRejectedValueOnce(new Error('authentication required'));
    render(<App />);

    expect(await screen.findByRole('heading', { name: '欢迎回来' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: '选择简历模板' })).not.toBeInTheDocument();
  });

  it('shows the gallery for a session and returns to auth after logout', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    logout.mockResolvedValueOnce(undefined);
    render(<App />);

    expect(await screen.findByRole('heading', { name: '选择简历模板' })).toBeInTheDocument();
    expect(screen.getByText('ada@example.com')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '退出登录' }));

    await waitFor(() => expect(logout).toHaveBeenCalledOnce());
    expect(await screen.findByRole('heading', { name: '欢迎回来' })).toBeInTheDocument();
  });

  it('purges user A resume state before user B signs in', async () => {
    const userAResume = {
      ...createSampleResumeDocument(),
      title: 'A 的机密简历',
      profile: { ...createSampleResumeDocument().profile, fullName: '用户 A' },
    };
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'a@example.com', tenantId: 11 });
    listResumes.mockResolvedValueOnce([{
      id: 71,
      title: userAResume.title,
      templateId: userAResume.templateId,
      content: JSON.stringify(userAResume),
      createdAt: '2026-08-18T01:00:00Z',
      updatedAt: '2026-08-18T02:00:00Z',
    }]);
    logout.mockResolvedValueOnce(undefined);
    login.mockResolvedValueOnce({ id: 2, email: 'b@example.com', tenantId: 22, isAdmin: false });
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    fireEvent.click(screen.getByRole('button', { name: '我的草稿' }));
    fireEvent.click(await screen.findByRole('button', { name: '继续编辑：A 的机密简历' }));
    expect(screen.getByLabelText('姓名')).toHaveValue('用户 A');

    fireEvent.click(screen.getByRole('button', { name: '退出登录' }));
    await screen.findByRole('heading', { name: '欢迎回来' });
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'b@example.com' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'password1234' } });
    fireEvent.click(screen.getByRole('button', { name: '登录' }));

    await screen.findByRole('heading', { name: '选择简历模板' });
    expect(window.location.pathname).toBe('/');
    fireEvent.click(screen.getByRole('button', { name: '选择模板：现代商务' }));
    expect(screen.getByLabelText('姓名')).not.toHaveValue('用户 A');
    expect(screen.queryByText('A 的机密简历')).not.toBeInTheDocument();
  });

  it('purges the active editor when authorization expires', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'a@example.com', tenantId: 11 });
    login.mockResolvedValueOnce({ id: 2, email: 'b@example.com', tenantId: 22, isAdmin: false });
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    fireEvent.click(screen.getByRole('button', { name: '选择模板：现代商务' }));
    fireEvent.change(screen.getByLabelText('姓名'), { target: { value: '过期的 A 用户' } });
    act(() => unauthorizedListener?.());

    await screen.findByRole('heading', { name: '欢迎回来' });
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'b@example.com' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'password1234' } });
    fireEvent.click(screen.getByRole('button', { name: '登录' }));
    await screen.findByRole('heading', { name: '选择简历模板' });
    fireEvent.click(screen.getByRole('button', { name: '选择模板：现代商务' }));

    expect(screen.getByLabelText('姓名')).not.toHaveValue('过期的 A 用户');
  });

  it('uses the template home as the primary authenticated starting point', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    render(<App />);

    expect(await screen.findByRole('heading', { name: '选择简历模板' })).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: '主要导航' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '模板主页' })).toHaveAttribute('aria-current', 'page');
  });

  it('waits for template status before showing the gallery during session restoration', async () => {
    let resolveUser!: (user: { id: number; email: string; tenantId: number; isAdmin: boolean }) => void;
    let resolveStatus!: (status: { disabledTemplateIds: string[] }) => void;
    getCurrentUser.mockImplementationOnce(() => new Promise((resolve) => { resolveUser = resolve; }));
    getTemplateStatus.mockImplementationOnce(() => new Promise((resolve) => { resolveStatus = resolve; }));
    render(<App />);

    resolveUser({ id: 1, email: 'ada@example.com', tenantId: 3, isAdmin: false });
    await waitFor(() => expect(getTemplateStatus).toHaveBeenCalledOnce());
    expect(screen.queryByRole('heading', { name: '选择简历模板' })).not.toBeInTheDocument();

    resolveStatus({ disabledTemplateIds: ['minimal'] });
    await screen.findByRole('heading', { name: '选择简历模板' });
    expect(screen.queryByRole('img', { name: '极简专业 模板预览' })).not.toBeInTheDocument();
  });

  it('does not show template administration to a regular user', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3, isAdmin: false });
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    expect(screen.queryByRole('button', { name: '后台管理' })).not.toBeInTheDocument();
  });

  it('allows an administrator to open template management', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'admin@example.com', tenantId: 3, isAdmin: true });
    listAdminTemplates.mockResolvedValueOnce({ disabledTemplateIds: [] });
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    fireEvent.click(screen.getByRole('button', { name: '后台管理' }));

    expect(await screen.findByRole('heading', { name: '模板管理' })).toBeInTheDocument();
  });

  it('waits for template status before showing the gallery after signing in', async () => {
    getCurrentUser.mockRejectedValueOnce(new Error('authentication required'));
    login.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3, isAdmin: false });
    let resolveStatus!: (status: { disabledTemplateIds: string[] }) => void;
    getTemplateStatus.mockImplementationOnce(() => new Promise((resolve) => { resolveStatus = resolve; }));
    render(<App />);

    await screen.findByRole('heading', { name: '欢迎回来' });
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'ada@example.com' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'password1234' } });
    fireEvent.click(screen.getByRole('button', { name: '登录' }));

    await waitFor(() => expect(getTemplateStatus).toHaveBeenCalledOnce());
    expect(screen.queryByRole('heading', { name: '选择简历模板' })).not.toBeInTheDocument();

    resolveStatus({ disabledTemplateIds: ['minimal'] });
    await screen.findByRole('heading', { name: '选择简历模板' });
    await waitFor(() => expect(screen.queryByRole('img', { name: '极简专业 模板预览' })).not.toBeInTheDocument());
  });

  it('keeps every template usable when template status cannot be loaded', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3, isAdmin: false });
    getTemplateStatus.mockRejectedValueOnce(new Error('status unavailable'));
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    expect(screen.getByRole('img', { name: '极简专业 模板预览' })).toBeInTheDocument();
  });

  it('navigates from personal drafts into the selected draft editor', async () => {
    const sample = createSampleResumeDocument();
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    listResumes.mockResolvedValueOnce([{
      id: 7,
      title: '产品经理简历',
      templateId: 'business',
      content: JSON.stringify({ ...sample, templateId: 'business', title: '产品经理简历' }),
      createdAt: '2026-08-14T01:00:00Z',
      updatedAt: '2026-08-14T02:00:00Z',
    }]);
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    expect(screen.getByRole('button', { name: '模板主页' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '我的草稿' }));

    expect(await screen.findByRole('heading', { name: '我的草稿' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '继续编辑：产品经理简历' }));

    expect(await screen.findByTestId('resume-preview')).toBeInTheDocument();
    expect(screen.getByText('当前模板：现代商务')).toBeInTheDocument();

    updateResume.mockResolvedValueOnce({
      id: 7,
      title: '产品经理简历',
      templateId: 'business',
      content: JSON.stringify({ ...sample, templateId: 'business', title: '产品经理简历' }),
      createdAt: '2026-08-14T01:00:00Z',
      updatedAt: '2026-08-14T03:00:00Z',
    });
    fireEvent.click(screen.getByRole('button', { name: '保存更改' }));

    await waitFor(() => expect(updateResume).toHaveBeenCalledWith(7, expect.objectContaining({ title: '产品经理简历' })));
  });

  it('keeps malformed drafts open and explains why they cannot be read', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    listResumes.mockResolvedValueOnce([{
      id: 9,
      title: '损坏的简历',
      templateId: 'business',
      content: '{bad json',
      createdAt: '2026-08-14T01:00:00Z',
      updatedAt: '2026-08-14T02:00:00Z',
    }]);
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    fireEvent.click(screen.getByRole('button', { name: '我的草稿' }));
    fireEvent.click(await screen.findByRole('button', { name: '继续编辑：损坏的简历' }));

    expect(screen.getByRole('heading', { name: '我的草稿' })).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('无法读取该简历内容。');
  });

  it('returns an empty drafts state to the template home', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    listResumes.mockResolvedValueOnce([]);
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    fireEvent.click(screen.getByRole('button', { name: '我的草稿' }));
    fireEvent.click(await screen.findByRole('button', { name: '去选择模板' }));

    expect(await screen.findByRole('heading', { name: '选择简历模板' })).toBeInTheDocument();
  });

  it('registers the target jobs page at /target-jobs', async () => {
    window.history.replaceState({}, '', '/target-jobs');
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });

    render(<App />);

    expect(await screen.findByRole('heading', { name: '目标岗位' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '目标岗位' })).toHaveAttribute('aria-current', 'page');
    expect(listTargetJobs).toHaveBeenCalledOnce();
  });

  it('isolates the whole authenticated app while the target-job dialog is open', async () => {
    window.history.replaceState({}, '', '/target-jobs');
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    const { container } = render(<App />);

    await screen.findByRole('heading', { name: '目标岗位' });
    const appRoot = container.querySelector('main.app-shell');
    const headerNavigation = screen.getByRole('navigation', { name: '主要导航' });
    fireEvent.click(screen.getByRole('button', { name: '+ 新建' }));

    const dialog = screen.getByRole('dialog', { name: '新建目标岗位' });
    expect(appRoot).toHaveAttribute('aria-hidden', 'true');
    expect(appRoot).toHaveAttribute('inert');
    expect(appRoot).not.toContainElement(dialog);
    headerNavigation.focus();
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(appRoot).not.toHaveAttribute('aria-hidden');
    expect(appRoot).not.toHaveAttribute('inert');
  });

  it('returns to a target-jobs deep link after signing in', async () => {
    window.history.replaceState({}, '', '/target-jobs');
    getCurrentUser.mockRejectedValueOnce(new Error('authentication required'));
    login.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3, isAdmin: false });
    render(<App />);

    await screen.findByRole('heading', { name: '欢迎回来' });
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'ada@example.com' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'password1234' } });
    fireEvent.click(screen.getByRole('button', { name: '登录' }));

    expect(await screen.findByRole('heading', { name: '目标岗位' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/target-jobs');
  });

  it('updates the URL when navigating out of target jobs', async () => {
    window.history.replaceState({}, '', '/target-jobs');
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    render(<App />);

    await screen.findByRole('heading', { name: '目标岗位' });
    fireEvent.click(screen.getByRole('button', { name: '我的草稿' }));

    expect(await screen.findByRole('heading', { name: '我的草稿' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/');
  });

  it('restores the target-jobs route with browser history', async () => {
    getCurrentUser.mockResolvedValueOnce({ id: 1, email: 'ada@example.com', tenantId: 3 });
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });
    fireEvent.click(screen.getByRole('button', { name: '目标岗位' }));
    expect(await screen.findByRole('heading', { name: '目标岗位' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '我的草稿' }));
    expect(await screen.findByRole('heading', { name: '我的草稿' })).toBeInTheDocument();

    window.history.back();

    await waitFor(() => expect(window.location.pathname).toBe('/target-jobs'));
    expect(await screen.findByRole('heading', { name: '目标岗位' })).toBeInTheDocument();

    window.history.forward();

    await waitFor(() => expect(window.location.pathname).toBe('/'));
    expect(await screen.findByRole('heading', { name: '我的草稿' })).toBeInTheDocument();
  });
});
