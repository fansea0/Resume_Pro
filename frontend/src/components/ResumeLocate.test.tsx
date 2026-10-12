import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  EDITOR_ITEM_ANCHOR_ATTR,
  EDITOR_SECTION_ANCHOR_ATTR,
  EntryLocateButton,
  ITEM_ANCHOR_ATTR,
  LOCATED_CLASS,
  SECTION_ANCHOR_ATTR,
  SectionLocateButton,
} from './ResumeLocate';

// vitest 未开启 globals，@testing-library/react 不会自动清理，需显式 cleanup。
afterEach(cleanup);

describe('ResumeLocate', () => {
  it('导出四个锚点属性名与高亮 class 常量', () => {
    expect(SECTION_ANCHOR_ATTR).toBe('data-resume-section-id');
    expect(ITEM_ANCHOR_ATTR).toBe('data-resume-item-id');
    expect(EDITOR_SECTION_ANCHOR_ATTR).toBe('data-editor-section-id');
    expect(EDITOR_ITEM_ANCHOR_ATTR).toBe('data-editor-item-id');
    expect(LOCATED_CLASS).toBe('is-located');
  });

  it('区块按钮点击后只发出区块级目标', () => {
    const onLocate = vi.fn();
    render(<h2><SectionLocateButton label="项目经历" onLocate={onLocate} sectionId="section-1">项目经历</SectionLocateButton></h2>);
    const button = screen.getByRole('button', { name: '定位到编辑器的「项目经历」' });
    expect(button.tagName).toBe('BUTTON');
    expect(button).toHaveAttribute('type', 'button');
    fireEvent.click(button);
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'section-1' });
  });

  it('条目按钮点击后同时发出区块与条目 id', () => {
    const onLocate = vi.fn();
    render(<h3><EntryLocateButton itemId="item-7" label="AI 创作副驾" sectionId="section-1" onLocate={onLocate}>AI 创作副驾</EntryLocateButton></h3>);
    fireEvent.click(screen.getByRole('button', { name: '定位到编辑器的「AI 创作副驾」' }));
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'section-1', itemId: 'item-7' });
  });

  it('缺少 onLocate 时不渲染按钮，标题保持纯文本', () => {
    render(<h2><SectionLocateButton label="项目经历" sectionId="section-1">项目经历</SectionLocateButton></h2>);
    expect(screen.queryByRole('button')).toBeNull();
    expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('项目经历');
  });

  it('保留标题语义层级：按钮在 h2 / h3 内部', () => {
    render(<h2><SectionLocateButton label="A" onLocate={vi.fn()} sectionId="section-1">A</SectionLocateButton></h2>);
    expect(screen.getByRole('heading', { level: 2 }).querySelector('button')).not.toBeNull();
  });
});
