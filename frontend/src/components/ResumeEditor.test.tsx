import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import App from '../app';
import { ResumeEditor } from './ResumeEditor';
import { createEmptyResumeDocument } from '../types/resume';

vi.mock('../api/auth', () => ({
  getCurrentUser: vi.fn().mockResolvedValue({ id: 1, email: 'ada@example.com', tenantId: 1 }),
  login: vi.fn(), register: vi.fn(), logout: vi.fn(),
}));

afterEach(cleanup);

describe('ResumeEditor', () => {
  it('updates the preview when the profile name changes', async () => {
    render(<App />);
    fireEvent.click(await screen.findByRole('button', { name: '选择模板：赤线单栏' }));

    fireEvent.change(screen.getByLabelText('姓名'), { target: { value: '林然' } });

    expect(screen.getByTestId('resume-preview')).toHaveTextContent('林然');
  });

  it('selects the business template', async () => {
    render(<App />);

    fireEvent.click(await screen.findByRole('button', { name: '选择模板：现代商务' }));

  });

  it('adds and removes a work experience', async () => {
    render(<App />);
    fireEvent.click(await screen.findByRole('button', { name: '选择模板：赤线单栏' }));

    const addExperience = screen.getByRole('button', { name: '添加工作经历' });
    fireEvent.click(addExperience.closest('details')?.querySelector('summary')!);
    fireEvent.click(addExperience);
    expect(screen.getByLabelText('公司 3')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '删除工作经历 3' }));
    expect(screen.queryByLabelText('公司 3')).not.toBeInTheDocument();
  });

  it('uses a UUID when adding an experience to a loaded resume', () => {
    const document = createEmptyResumeDocument();
    document.experiences = [{
      id: 'experience-1', company: '已保存公司', role: '', startDate: '', endDate: '', highlights: [],
    }];
    const onChange = vi.fn();
    const randomUUID = vi.fn(() => 'new-uuid');
    vi.stubGlobal('crypto', { randomUUID });

    render(<ResumeEditor document={document} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加工作经历' }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      experiences: expect.arrayContaining([expect.objectContaining({ id: 'new-uuid' })]),
    }));
    vi.unstubAllGlobals();
  });

  it('keeps basic profile open and repeatable sections collapsed by default', () => {
    render(<ResumeEditor document={createEmptyResumeDocument()} onChange={vi.fn()} />);

    const profile = screen.getByText('基本信息').closest('details');
    expect(profile).toHaveAttribute('open');
    expect(profile?.querySelector('summary')).toHaveTextContent('基本信息');
    expect(profile?.querySelector('summary')?.querySelector('h1, h2, h3, h4, h5, h6')).not.toBeInTheDocument();

    ['工作经历', '项目经历', '教育经历', '专业技能'].forEach((title) => {
      const section = screen.getByText(title).closest('details');
      expect(section).toBeInTheDocument();
      expect(section).not.toHaveAttribute('open');
      expect(section?.querySelector('summary')).toHaveTextContent(title);
      expect(section?.querySelector('summary')?.querySelector('h1, h2, h3, h4, h5, h6')).not.toBeInTheDocument();
    });
  });

  it('toggles each native section independently from its summary', () => {
    render(<ResumeEditor document={createEmptyResumeDocument()} onChange={vi.fn()} />);

    const work = screen.getByText('工作经历').closest('details');
    const projects = screen.getByText('项目经历').closest('details');
    fireEvent.click(screen.getByText('工作经历'));
    fireEvent.click(screen.getByText('项目经历'));

    expect(work).toHaveAttribute('open');
    expect(projects).toHaveAttribute('open');

    fireEvent.click(screen.getByText('工作经历'));
    expect(work).not.toHaveAttribute('open');
    expect(projects).toHaveAttribute('open');
  });

  it('updates document inputs after opening a collapsed section', () => {
    const document = createEmptyResumeDocument();
    document.experiences = [{
      id: 'experience-1', company: '', role: '', startDate: '', endDate: '', highlights: [],
    }];
    const onChange = vi.fn();
    render(<ResumeEditor document={document} onChange={onChange} />);

    fireEvent.click(screen.getByText('工作经历'));
    fireEvent.change(screen.getByLabelText('公司 1'), { target: { value: 'Apple' } });

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      experiences: [expect.objectContaining({ company: 'Apple' })],
    }));
  });
});
