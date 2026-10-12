import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderInlineMarkdown } from './ResumeInline';

function renderInline(text: string) {
  const { container } = render(<p>{renderInlineMarkdown(text)}</p>);
  return container.querySelector('p') as HTMLElement;
}

describe('renderInlineMarkdown', () => {
  it('renders bold and italic instead of literal asterisks', () => {
    const node = renderInline('负责 **重点** 方向，*强调* 重点');
    expect(node.querySelector('strong')).toHaveTextContent('重点');
    expect(node.querySelector('em')).toHaveTextContent('强调');
    expect(node.textContent).toBe('负责 重点 方向，强调 重点');
  });

  it('keeps plain text untouched and supports mixed inline spans', () => {
    const node = renderInline('提升 **30%** 的转化');
    expect(node.textContent).toBe('提升 30% 的转化');
    expect(node.querySelectorAll('strong')).toHaveLength(1);
  });

  it('renders safe links and degrades unsafe ones to literal text', () => {
    expect(renderInline('[投递简历](https://example.com)').querySelector('a'))
      .toHaveAttribute('href', 'https://example.com');
    const unsafe = renderInline('[危险链接](javascript:alert(1))');
    expect(unsafe.querySelector('a')).toBeNull();
    expect(unsafe.textContent).toBe('[危险链接](javascript:alert(1))');
  });

  it('leaves an unclosed marker as plain text', () => {
    expect(renderInline('未闭合 **加粗').textContent).toBe('未闭合 **加粗');
  });
});