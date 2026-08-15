import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ResumeMarkdown } from './ResumeMarkdown';

describe('ResumeMarkdown', () => {
  it('renders the supported inline Markdown and line breaks', () => {
    const { container } = render(
      <ResumeMarkdown markdown={'**加粗**、*斜体*、[链接](https://example.com)\n下一行'} />,
    );

    expect(screen.getByText('加粗').tagName).toBe('STRONG');
    expect(screen.getByText('斜体').tagName).toBe('EM');
    expect(screen.getByRole('link', { name: '链接' })).toHaveAttribute('href', 'https://example.com');
    const paragraph = container.querySelector('.resume-markdown-paragraph');
    expect(paragraph?.querySelector('br')).toBeInTheDocument();
    expect(paragraph).toHaveTextContent('下一行');
  });

  it('renders unordered and ordered lists', () => {
    const { container } = render(<ResumeMarkdown markdown={'- 第一项\n- 第二项\n\n1. 第三项\n2. 第四项'} />);

    expect(container.querySelector('.resume-markdown-unordered-list')).toBeInTheDocument();
    expect(container.querySelector('.resume-markdown-ordered-list')).toBeInTheDocument();
    expect(screen.getAllByRole('listitem')).toHaveLength(4);
  });

  it('treats raw HTML as text instead of rendering it', () => {
    render(<ResumeMarkdown markdown={'<img src=x onerror="window.__unsafe = true">安全文本'} />);

    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    expect(screen.getByText(/<img src=x/)).toBeInTheDocument();
    expect(window).not.toHaveProperty('__unsafe');
  });

  it('does not render unsupported rich Markdown elements', () => {
    const { container } = render(
      <ResumeMarkdown markdown={'# 标题\n\n![图片](https://example.com/image.png)\n\n| 名称 | 内容 |\n| --- | --- |\n| A | B |\n\n~~删除线~~\n\n`代码`\n\n- [x] 任务'} />,
    );

    expect(container.querySelector('h1, img, table, del, code, input')).not.toBeInTheDocument();
  });

  it('does not expose javascript URLs in links', () => {
    const { container } = render(<ResumeMarkdown markdown={'[危险链接](javascript:alert(1))'} />);

    const link = container.querySelector('a');
    expect(link).toHaveTextContent('危险链接');
    expect(link?.getAttribute('href')).not.toMatch(/^javascript:/i);
  });
});
