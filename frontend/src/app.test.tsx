import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
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

vi.mock('./api/auth', () => ({
  getCurrentUser: (...args: unknown[]) => getCurrentUser(...args),
  logout: (...args: unknown[]) => logout(...args),
  login: vi.fn(),
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

afterEach(cleanup);

describe('application authentication gate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listResumes.mockResolvedValue([]);
    getTemplateStatus.mockResolvedValue({ disabledTemplateIds: [] });
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
});
