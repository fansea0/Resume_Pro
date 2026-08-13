import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import App from '../app';
import { ResumeEditor } from './ResumeEditor';
import { createEmptyResumeDocument } from '../types/resume';

afterEach(cleanup);

describe('ResumeEditor', () => {
  it('updates the preview when the profile name changes', () => {
    render(<App />);

    fireEvent.change(screen.getByLabelText('姓名'), { target: { value: '林然' } });

    expect(screen.getByTestId('resume-preview')).toHaveTextContent('林然');
  });

  it('selects the business template', () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: /现代商务/ }));

    expect(screen.getByTestId('active-template')).toHaveTextContent('business');
  });

  it('adds and removes a work experience', () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: '添加工作经历' }));
    expect(screen.getByLabelText('公司 1')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '删除工作经历 1' }));
    expect(screen.queryByLabelText('公司 1')).not.toBeInTheDocument();
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
});
