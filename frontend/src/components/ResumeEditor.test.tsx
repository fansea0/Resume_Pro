import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResumeEditor } from './ResumeEditor';
import { createEmptyResumeDocument, createWorkSection } from '../types/resume';

afterEach(cleanup);

function latestDocument(onChange: ReturnType<typeof vi.fn>) {
  return onChange.mock.calls[onChange.mock.calls.length - 1]?.[0];
}

function StatefulEditor({ document, onChange }: { document: ReturnType<typeof createEmptyResumeDocument>; onChange: ReturnType<typeof vi.fn> }) {
  const [current, setCurrent] = useState(document);
  return <ResumeEditor document={current} onChange={(next) => { setCurrent(next); onChange(next); }} />;
}

describe('ResumeEditor', () => {
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

  it('edits a work location, makes it visible, and groups display settings clearly', () => {
    const document = { ...createEmptyResumeDocument(), sections: [createWorkSection('work-1')] };
    const onChange = vi.fn();
    render(<StatefulEditor document={document} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '添加工作条目' }));
    const added = latestDocument(onChange).sections[0].content.items[0];
    expect(added).toMatchObject({ location: '', achievements: '', highlightsMarkdown: '', showLocation: true, showAchievements: true });

    fireEvent.change(screen.getByLabelText('公司 1'), { target: { value: 'Acme' } });
    expect(latestDocument(onChange).sections[0].content.items[0].company).toBe('Acme');
    fireEvent.click(screen.getByLabelText('显示地点 1'));
    expect(latestDocument(onChange).sections[0].content.items[0].showLocation).toBe(false);
    fireEvent.change(screen.getByLabelText('地点 1'), { target: { value: '武汉 · 光谷' } });
    expect(latestDocument(onChange).sections[0].content.items[0]).toMatchObject({ location: '武汉 · 光谷', showLocation: true });
    expect(screen.getByLabelText('地点 1')).toHaveAttribute('placeholder', '例如：武汉 · 光谷');
    expect(screen.getByRole('group', { name: '展示设置 1' })).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText('显示地点 1'));
    fireEvent.click(screen.getByLabelText('显示业绩贡献 1'));
    expect(latestDocument(onChange).sections[0].content.items[0]).toMatchObject({ showLocation: false, showAchievements: false });
  });

  it('initializes and edits all project optional field switches', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加项目经历区块' }));
    fireEvent.click(screen.getByRole('button', { name: '添加项目条目' }));

    const project = latestDocument(onChange).sections[0].content.items[0];
    expect(project).toMatchObject({ showLocation: true, showLink: true, showIntroduction: true, showAchievements: true });
    ['显示地点 1', '显示项目链接 1', '显示项目介绍 1', '显示项目成绩 1'].forEach((label) => fireEvent.click(screen.getByLabelText(label)));
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
    fireEvent.change(screen.getByLabelText('备注 1'), { target: { value: '熟悉' } });
    expect(latestDocument(onChange).sections[1].content.items[0]).toMatchObject({ name: '', note: '熟悉' });
  });

  it('updates summary markdown', () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '添加个人总结区块' }));
    fireEvent.change(screen.getByLabelText('个人总结 Markdown'), { target: { value: '**重点**\n\n- 条目' } });
    expect(latestDocument(onChange).sections[0]).toMatchObject({ type: 'summary', content: { markdown: '**重点**\n\n- 条目' } });
  });
});
