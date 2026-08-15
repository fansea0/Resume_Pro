import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResumeEditor } from './ResumeEditor';
import { createEmptyResumeDocument, createWorkSection } from '../types/resume';
import { uploadResumeAvatar } from '../api/uploads';

vi.mock('../api/uploads', () => ({ uploadResumeAvatar: vi.fn() }));

afterEach(() => { cleanup(); vi.clearAllMocks(); });

function latestDocument(onChange: ReturnType<typeof vi.fn>) {
  return onChange.mock.calls[onChange.mock.calls.length - 1]?.[0];
}

function StatefulEditor({ document, onChange }: { document: ReturnType<typeof createEmptyResumeDocument>; onChange: ReturnType<typeof vi.fn> }) {
  const [current, setCurrent] = useState(document);
  return <ResumeEditor document={current} onChange={(next) => { setCurrent(next); onChange(next); }} />;
}

describe('ResumeEditor', () => {
  it('uploads a 5:7 resume photo, initializes its key, and reuses the key when replacing it', async () => {
    const onChange = vi.fn();
    const upload = vi.mocked(uploadResumeAvatar);
    upload.mockResolvedValueOnce({ url: '/uploads/avatars/first.webp' }).mockResolvedValueOnce({ url: '/uploads/avatars/second.webp' });
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    const input = screen.getByLabelText('上传简历证件照');

    fireEvent.change(input, { target: { files: [new File(['first'], 'first.webp', { type: 'image/webp' })] } });
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1));
    const stored = latestDocument(onChange).profile.avatarStorageKey;
    expect(stored).toMatch(/^[0-9a-f-]{36}$/);
    expect(latestDocument(onChange).profile.avatarUrl).toBe('/uploads/avatars/first.webp');
    expect(screen.getByAltText('简历证件照预览')).toHaveAttribute('src', '/uploads/avatars/first.webp');

    fireEvent.change(input, { target: { files: [new File(['second'], 'second.webp', { type: 'image/webp' })] } });
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(2));
    expect(upload.mock.calls[1][1]).toBe(stored);
  });

  it('keeps the profile unchanged and explains invalid or failed photo uploads', async () => {
    const onChange = vi.fn();
    const upload = vi.mocked(uploadResumeAvatar);
    upload.mockRejectedValueOnce(new Error('图片内容无法识别。'));
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    const input = screen.getByLabelText('上传简历证件照');

    fireEvent.change(input, { target: { files: [new File(['bad'], 'bad.gif', { type: 'image/gif' })] } });
    expect(screen.getByRole('alert')).toHaveTextContent('请选择 JPG、PNG 或 WebP 格式的证件照。');
    expect(upload).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { files: [new File(['bad'], 'bad.png', { type: 'image/png' })] } });
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('图片内容无法识别。'));
    expect(onChange).not.toHaveBeenCalled();
  });
  it('adds fully initialized work and internship sections', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '添加工作经历区块' }));
    expect(latestDocument(onChange).sections).toEqual([expect.objectContaining({
      type: 'work', title: '工作经历', visible: true, content: { items: [] },
    })]);

    fireEvent.click(screen.getByRole('button', { name: '添加实习经历区块' }));
    expect(latestDocument(onChange).sections).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'internship', title: '实习经历', visible: true, content: { items: [] } }),
    ]));
  });

  it('moves sections and toggles a section visibility', () => {
    const work = createWorkSection('work-1');
    const document = { ...createEmptyResumeDocument(), sections: [
      { ...work, title: '第一段' },
      { ...createWorkSection('work-2'), title: '第二段' },
    ] };
    const onChange = vi.fn();
    render(<StatefulEditor document={document} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '下移 第一段' }));
    expect(latestDocument(onChange).sections.map((section: { id: string }) => section.id)).toEqual(['work-2', 'work-1']);

    fireEvent.click(screen.getByRole('button', { name: '隐藏 第一段' }));
    expect(latestDocument(onChange).sections.find((section: { id: string }) => section.id === 'work-1')).toEqual(expect.objectContaining({ visible: false }));
  });

  it('edits a work location without changing its display choice and groups display settings clearly', () => {
    const document = { ...createEmptyResumeDocument(), sections: [createWorkSection('work-1')] };
    const onChange = vi.fn();
    render(<StatefulEditor document={document} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '添加工作条目' }));
    const added = latestDocument(onChange).sections[0].content.items[0];
    expect(added).toMatchObject({ location: '', achievements: '', highlightsMarkdown: '', showLocation: true, showAchievements: true });

    fireEvent.change(screen.getByLabelText('公司 1'), { target: { value: 'Acme' } });
    expect(latestDocument(onChange).sections[0].content.items[0].company).toBe('Acme');
    fireEvent.click(screen.getByRole('button', { name: '隐藏地点 1' }));
    expect(latestDocument(onChange).sections[0].content.items[0].showLocation).toBe(false);
    fireEvent.change(screen.getByLabelText('地点 1'), { target: { value: '武汉 · 光谷' } });
    expect(latestDocument(onChange).sections[0].content.items[0]).toMatchObject({ location: '武汉 · 光谷', showLocation: false });
    expect(screen.getByLabelText('地点 1')).toHaveAttribute('placeholder', '例如：武汉 · 光谷');
    expect(screen.queryByText('填写城市、区域或办公地点；可在下方选择是否显示在简历中。')).not.toBeInTheDocument();
    expect(screen.queryByRole('group', { name: '展示设置 1' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '显示地点 1' }));
    expect(screen.getByRole('button', { name: '隐藏地点 1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '隐藏业绩贡献 1' }));
    expect(latestDocument(onChange).sections[0].content.items[0]).toMatchObject({ showLocation: true, showAchievements: false });
  });

  it('initializes and edits all project optional field switches', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加项目经历区块' }));
    fireEvent.click(screen.getByRole('button', { name: '添加项目条目' }));

    const project = latestDocument(onChange).sections[0].content.items[0];
    expect(project).toMatchObject({ showLocation: true, showLink: true, showIntroduction: true, showAchievements: true });
    ['隐藏地点 1', '隐藏项目链接 1', '隐藏项目介绍 1', '隐藏项目成绩 1'].forEach((label) => fireEvent.click(screen.getByRole('button', { name: label })));
    expect(latestDocument(onChange).sections[0].content.items[0]).toMatchObject({ showLocation: false, showLink: false, showIntroduction: false, showAchievements: false });
  });

  it('deletes a project item by its stable ID', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加项目经历区块' }));
    fireEvent.click(screen.getByRole('button', { name: '添加项目条目' }));
    fireEvent.click(screen.getByRole('button', { name: '添加项目条目' }));

    fireEvent.click(screen.getByRole('button', { name: '删除项目条目 1' }));
    const items = latestDocument(onChange).sections[0].content.items;
    expect(items).toHaveLength(1);
    expect(items[0].id).not.toBe('');
  });

  it('assigns unique input IDs to same-named fields in different sections', () => {
    const first = createWorkSection('work-first');
    first.content.items = [{ id: 'work-item', company: '', role: '', startDate: '', endDate: '', location: '', achievements: '', highlightsMarkdown: '', showLocation: true, showAchievements: true }];
    const second = createWorkSection('work-second');
    second.content.items = [{ id: 'second-item', company: '', role: '', startDate: '', endDate: '', location: '', achievements: '', highlightsMarkdown: '', showLocation: true, showAchievements: true }];
    render(<ResumeEditor document={{ ...createEmptyResumeDocument(), sections: [first, second] }} onChange={vi.fn()} />);

    const inputs = screen.getAllByLabelText('公司 1');
    expect(inputs).toHaveLength(2);
    expect(inputs[0].id).not.toBe(inputs[1].id);
    inputs.forEach((input) => expect(document.querySelector(`label[for="${input.id}"]`)).toBeTruthy());
  });

  it('adds and edits a campus item and a short honour item', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加校园经历区块' }));
    fireEvent.click(screen.getByRole('button', { name: '添加校园条目' }));
    fireEvent.change(screen.getByLabelText('活动或部门 1'), { target: { value: '学生会' } });
    expect(latestDocument(onChange).sections[0].content.items[0]).toMatchObject({ name: '学生会', role: '', highlightsMarkdown: '' });

    fireEvent.click(screen.getByRole('button', { name: '添加荣誉奖项区块' }));
    fireEvent.click(screen.getByRole('button', { name: '添加荣誉奖项' }));
    fireEvent.change(screen.getByLabelText('奖项名称 1'), { target: { value: '一等奖学金' } });
    expect(latestDocument(onChange).sections[1].content.items[0]).toMatchObject({ name: '一等奖学金' });
  });

  it('adds fully initialized education and skill items', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加教育经历区块' }));
    fireEvent.click(screen.getByRole('button', { name: '添加教育条目' }));
    expect(latestDocument(onChange).sections[0].content.items[0]).toMatchObject({ school: '', startDate: '', endDate: '', degree: '', college: '', major: '' });
    fireEvent.change(screen.getByLabelText('院校 1'), { target: { value: '复旦大学' } });
    expect(latestDocument(onChange).sections[0].content.items[0].school).toBe('复旦大学');

    fireEvent.click(screen.getByRole('button', { name: '添加专业技能区块' }));
    fireEvent.click(screen.getByRole('button', { name: '添加技能' }));
    expect(screen.getByRole('button', { name: '删除技能 1' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('备注 1'), { target: { value: '熟悉' } });
    expect(latestDocument(onChange).sections[1].content.items[0]).toMatchObject({ name: '', note: '熟悉' });
    expect(document.querySelector('.editor-item-grid--skills')).toBeInTheDocument();
  });

  it('updates summary markdown', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加个人总结区块' }));
    fireEvent.change(screen.getByLabelText('个人总结 Markdown'), { target: { value: '**重点**\n\n- 条目' } });
    expect(latestDocument(onChange).sections[0]).toMatchObject({ type: 'summary', content: { markdown: '**重点**\n\n- 条目' } });
  });
});
