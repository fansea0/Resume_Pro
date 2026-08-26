import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ResumeEditor } from './ResumeEditor';
import {
  createEmptyResumeDocument,
  createProjectSection,
  createSampleResumeDocument,
  createWorkSection,
} from '../types/resume';
import { uploadResumeAvatar } from '../api/uploads';
import type { SSEEvent } from '../types/ai';
import type { TargetJob } from '../types/targetJob';

const { createTargetJobMock, getTargetJobMock, listTargetJobsMock, streamRewriteMock } = vi.hoisted(() => ({
  createTargetJobMock: vi.fn(),
  getTargetJobMock: vi.fn(),
  listTargetJobsMock: vi.fn(),
  streamRewriteMock: vi.fn(),
}));

vi.mock('../api/uploads', () => ({ uploadResumeAvatar: vi.fn() }));
vi.mock('../api/ai', () => ({ streamRewrite: streamRewriteMock }));
vi.mock('../api/targetJobs', () => ({
  createTargetJob: createTargetJobMock,
  getTargetJob: getTargetJobMock,
  listTargetJobs: listTargetJobsMock,
}));

afterEach(() => { cleanup(); vi.clearAllMocks(); });

const targetJob: TargetJob = {
  id: 7,
  company: '字节跳动',
  position: '高级后端工程师',
  requirements: '负责后端架构设计与 AI 能力落地，关注稳定性、研发效能和业务结果。',
  createdAt: '2026-08-18T01:00:00Z',
  updatedAt: '2026-08-19T02:00:00Z',
};
const emptyOptimizationAnalysis = { jdAlignment: { status: 'skipped' as const }, matchedIssues: [], evidenceRequests: [], warnings: [] };

beforeEach(() => {
  getTargetJobMock.mockResolvedValue(targetJob);
  listTargetJobsMock.mockResolvedValue([targetJob]);
  streamRewriteMock.mockResolvedValue(undefined);
});

function latestDocument(onChange: ReturnType<typeof vi.fn>) {
  return onChange.mock.calls[onChange.mock.calls.length - 1]?.[0];
}

function StatefulEditor({ document, onBeforeAiRewrite = async () => true, onChange, resumeId }: {
  document: ReturnType<typeof createEmptyResumeDocument>;
  onBeforeAiRewrite?: (snapshot: ReturnType<typeof createEmptyResumeDocument>) => Promise<boolean>;
  onChange: ReturnType<typeof vi.fn>;
  resumeId?: number;
}) {
  const [current, setCurrent] = useState(document);
  return <ResumeEditor document={current} onBeforeAiRewrite={onBeforeAiRewrite} onChange={(next) => { setCurrent(next); onChange(next); }} resumeId={resumeId} />;
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

  it('writes a selected target job id through the document onChange path', async () => {
    const onChange = vi.fn();
    render(<StatefulEditor document={createEmptyResumeDocument()} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '+ 选择目标岗位' }));
    fireEvent.click(await screen.findByRole('button', { name: '选择：字节跳动 · 高级后端工程师' }));

    expect(latestDocument(onChange)).toEqual(expect.objectContaining({ targetJobId: 7 }));
    expect(await screen.findByText('当前：字节跳动 · 高级后端工程师')).toBeInTheDocument();
    expect(getTargetJobMock).toHaveBeenCalledWith(7);
  });

  it('shows AI polish actions only for rewriteable section headers', () => {
    render(<ResumeEditor document={createSampleResumeDocument()} onChange={vi.fn()} resumeId={42} />);

    ['个人总结', '工作经历', '实习经历', '项目经历'].forEach((label) => {
      expect(screen.getByRole('button', { name: `AI 润色 ${label}` })).toBeInTheDocument();
    });
    ['教育经历', '专业技能', '校园经历', '荣誉奖项'].forEach((label) => {
      expect(screen.queryByRole('button', { name: `AI 润色 ${label}` })).not.toBeInTheDocument();
    });
  });

  it('opens the AI suggestion panel from a supported section header', async () => {
    render(<ResumeEditor document={createSampleResumeDocument()} onBeforeAiRewrite={async () => true} onChange={vi.fn()} resumeId={42} />);

    fireEvent.click(screen.getByRole('button', { name: 'AI 润色 个人总结' }));

    expect(await screen.findByRole('dialog', { name: 'AI 润色建议' })).toBeInTheDocument();
    expect(streamRewriteMock).toHaveBeenCalledWith(
      42,
      { sectionId: 'sample-summary', sectionType: 'summary' },
      expect.any(Function),
      expect.any(AbortSignal),
    );
  });

  it('waits for the exact current document to save before starting AI rewrite', async () => {
    const current = createSampleResumeDocument();
    current.targetJobId = 7;
    current.sections[0] = {
      ...current.sections[0],
      content: { markdown: '刚刚编辑、尚未自动保存的总结' },
    } as typeof current.sections[0];
    let finishSave!: (saved: boolean) => void;
    const onBeforeAiRewrite = vi.fn(() => new Promise<boolean>((resolve) => { finishSave = resolve; }));
    render(<ResumeEditor
      document={current}
      onBeforeAiRewrite={onBeforeAiRewrite}
      onChange={vi.fn()}
      resumeId={42}
    />);

    fireEvent.click(screen.getByRole('button', { name: 'AI 润色 个人总结' }));

    expect(onBeforeAiRewrite).toHaveBeenCalledWith(current);
    expect(screen.getByRole('button', { name: 'AI 润色 个人总结' })).toBeDisabled();
    expect(screen.getByText('正在保存最新简历内容…')).toHaveAttribute('role', 'status');
    expect(streamRewriteMock).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog', { name: 'AI 润色建议' })).not.toBeInTheDocument();

    await act(async () => { finishSave(true); });

    expect(await screen.findByRole('dialog', { name: 'AI 润色建议' })).toBeInTheDocument();
    expect(streamRewriteMock).toHaveBeenCalledTimes(1);
  });

  it('keeps AI rewrite closed and usable after the latest snapshot fails to save', async () => {
    const onBeforeAiRewrite = vi.fn().mockResolvedValue(false);
    render(<ResumeEditor
      document={createSampleResumeDocument()}
      onBeforeAiRewrite={onBeforeAiRewrite}
      onChange={vi.fn()}
      resumeId={42}
    />);

    fireEvent.click(screen.getByRole('button', { name: 'AI 润色 个人总结' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('保存最新内容后才能使用 AI 润色');
    expect(screen.getByRole('button', { name: 'AI 润色 个人总结' })).toBeEnabled();
    expect(streamRewriteMock).not.toHaveBeenCalled();
  });

  it('retains a failed target binding with retry and clear controls', async () => {
    getTargetJobMock.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(targetJob);
    const onChange = vi.fn();
    render(<StatefulEditor
      document={{ ...createEmptyResumeDocument(), targetJobId: 7 }}
      onChange={onChange}
      resumeId={42}
    />);

    expect(await screen.findByText('当前：目标岗位 #7')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('无法加载目标岗位详情');
    expect(screen.getByRole('button', { name: '更换目标岗位' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '清除目标岗位绑定' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '重新加载目标岗位详情' }));
    expect(await screen.findByText('当前：字节跳动 · 高级后端工程师')).toBeInTheDocument();
    expect(getTargetJobMock).toHaveBeenCalledTimes(2);
    expect(getTargetJobMock).toHaveBeenNthCalledWith(2, 7);

    fireEvent.click(screen.getByRole('button', { name: '清除目标岗位绑定' }));
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '清除绑定' }));
    expect(latestDocument(onChange).targetJobId).toBeUndefined();
  });

  it('rejects ambiguous duplicate section ids before saving or starting AI', async () => {
    const first = createProjectSection('duplicate-project');
    first.title = '重复项目甲';
    first.content.items = [{
      id: 'first', name: '甲', role: '', startDate: '', endDate: '', location: '', link: '',
      introduction: '甲介绍', achievements: '', highlightsMarkdown: '', showLocation: true,
      showLink: true, showIntroduction: true, showAchievements: true,
    }];
    const second = createProjectSection('duplicate-project');
    second.title = '重复项目乙';
    second.content.items = [{
      id: 'second', name: '乙', role: '', startDate: '', endDate: '', location: '', link: '',
      introduction: '乙介绍', achievements: '', highlightsMarkdown: '', showLocation: true,
      showLink: true, showIntroduction: true, showAchievements: true,
    }];
    const onBeforeAiRewrite = vi.fn().mockResolvedValue(true);
    render(<ResumeEditor
      document={{ ...createEmptyResumeDocument(), sections: [first, second] }}
      onBeforeAiRewrite={onBeforeAiRewrite}
      onChange={vi.fn()}
      resumeId={42}
    />);

    fireEvent.click(screen.getByRole('button', { name: 'AI 润色 重复项目乙' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('区块标识重复，无法安全应用 AI 建议');
    expect(onBeforeAiRewrite).not.toHaveBeenCalled();
    expect(streamRewriteMock).not.toHaveBeenCalled();
  });

  it('does not accept a rewrite after the selected section changes type', async () => {
    const initial = createSampleResumeDocument();
    const replacement = createProjectSection(initial.sections[0].id);
    replacement.content.items = [{
      id: 'replacement', name: '替换后的项目', role: '', startDate: '', endDate: '', location: '', link: '',
      introduction: '项目内容', achievements: '', highlightsMarkdown: '', showLocation: true,
      showLink: true, showIntroduction: true, showAchievements: true,
    }];
    let emit!: (event: SSEEvent) => void;
    streamRewriteMock.mockImplementation((_resumeId, _request, onEvent) => {
      emit = onEvent;
      return new Promise<void>(() => undefined);
    });
    const onChange = vi.fn();
    const beforeRewrite = vi.fn().mockResolvedValue(true);
    const { rerender } = render(<ResumeEditor
      document={initial}
      onBeforeAiRewrite={beforeRewrite}
      onChange={onChange}
      resumeId={42}
    />);
    fireEvent.click(screen.getByRole('button', { name: 'AI 润色 个人总结' }));
    await screen.findByRole('dialog', { name: 'AI 润色建议' });

    rerender(<ResumeEditor
      document={{ ...initial, sections: [replacement, ...initial.sections.slice(1)] }}
      onBeforeAiRewrite={beforeRewrite}
      onChange={onChange}
      resumeId={42}
    />);
    act(() => {
      emit({
        kind: 'suggestion',
        sectionId: initial.sections[0].id,
        sectionType: 'summary',
        newContent: { rewrite: { markdown: '不应应用' }, analysis: emptyOptimizationAnalysis },
      });
      emit({ kind: 'done' });
    });
    fireEvent.click(await screen.findByRole('button', { name: '采纳' }));

    expect(onChange).not.toHaveBeenCalled();
    expect(await screen.findByRole('alert')).toHaveTextContent('当前区块已变化，未应用 AI 建议');
  });

  it('merges accepted rewrites by section and item id without changing frozen fields', async () => {
    const first = createProjectSection('project-first');
    first.title = '项目甲';
    first.content.items = [{
      id: 'first-item',
      name: '冻结的甲项目名',
      role: '甲负责人',
      startDate: '2024.01',
      endDate: '2024.02',
      location: '上海',
      link: 'https://first.example.com',
      introduction: '甲原介绍',
      achievements: '甲原成果',
      highlightsMarkdown: '甲原亮点',
      showLocation: true,
      showLink: true,
      showIntroduction: true,
      showAchievements: true,
    }];
    const second = createProjectSection('project-second');
    second.title = '项目乙';
    second.visible = false;
    second.content.items = [
      {
        id: 'second-one',
        name: '冻结的乙项目名',
        role: '乙负责人',
        startDate: '2025.01',
        endDate: '2025.02',
        location: '北京',
        link: 'https://second.example.com',
        introduction: '乙原介绍',
        achievements: '乙原成果',
        highlightsMarkdown: '乙原亮点',
        showLocation: false,
        showLink: true,
        showIntroduction: true,
        showAchievements: false,
      },
      {
        id: 'second-two',
        name: '冻结的第三项目名',
        role: '开发者',
        startDate: '2025.03',
        endDate: '2025.04',
        location: '深圳',
        link: '',
        introduction: '第三原介绍',
        achievements: '第三原成果',
        highlightsMarkdown: '第三原亮点',
        showLocation: true,
        showLink: false,
        showIntroduction: true,
        showAchievements: true,
      },
    ];
    streamRewriteMock.mockImplementation(async (
      _resumeId: number,
      _request: unknown,
      onEvent: (event: SSEEvent) => void,
    ) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'project-second',
        sectionType: 'project',
        newContent: {
          rewrite: { items: [
            { id: 'second-two', highlightsMarkdown: '第三新亮点' },
            {
              id: 'second-one',
              introduction: '乙新介绍',
              achievements: '',
              name: '不能覆盖的名称',
            },
            { id: 'unknown-item', introduction: '不应加入文档' },
          ] },
          analysis: emptyOptimizationAnalysis,
        },
      } as SSEEvent);
      onEvent({ kind: 'done' });
    });
    const onChange = vi.fn();
    const initialDocument = { ...createEmptyResumeDocument(), sections: [first, second] };
    render(<StatefulEditor document={initialDocument} onChange={onChange} resumeId={42} />);

    fireEvent.click(screen.getByRole('button', { name: 'AI 润色 项目乙' }));
    fireEvent.click(await screen.findByRole('button', { name: '采纳' }));

    const changed = latestDocument(onChange);
    expect(changed.sections[0]).toEqual(first);
    expect(changed.sections[1]).toEqual({
      ...second,
      content: {
        items: [
          { ...second.content.items[0], introduction: '乙新介绍', achievements: '' },
          { ...second.content.items[1], highlightsMarkdown: '第三新亮点' },
        ],
      },
    });
    expect(changed.sections[1].content.items).toHaveLength(2);
  });
});
