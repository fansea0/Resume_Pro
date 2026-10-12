import type { ReactNode } from 'react';
import { safeUrl } from './ResumeMarkdown';

interface InlineToken {
  type: 'text' | 'strong' | 'em' | 'link';
  value: string;
  href?: string;
}

// Supports the same inline subset as ResumeMarkdown: bold, italic and safe links.
const INLINE_PATTERN = /\*\*(.+?)\*\*|\*(.+?)\*|\[([^\]\n]+)\]\(([^)\s]+)\)/g;

function tokenize(text: string): InlineToken[] {
  const tokens: InlineToken[] = [];
  let lastIndex = 0;

  for (const match of text.matchAll(INLINE_PATTERN)) {
    const start = match.index ?? 0;
    if (start > lastIndex) tokens.push({ type: 'text', value: text.slice(lastIndex, start) });

    const [raw, strong, emphasis, label, href] = match;
    if (strong !== undefined) {
      tokens.push({ type: 'strong', value: strong });
    } else if (emphasis !== undefined) {
      tokens.push({ type: 'em', value: emphasis });
    } else {
      const safeHref = safeUrl(href ?? '');
      tokens.push(safeHref
        ? { type: 'link', value: label ?? '', href: safeHref }
        // An unsafe target degrades to literal text instead of a dead link.
        : { type: 'text', value: raw });
    }
    lastIndex = start + raw.length;
  }

  if (lastIndex < text.length) tokens.push({ type: 'text', value: text.slice(lastIndex) });
  return tokens;
}

/**
 * Renders inline Markdown for single-line fields (项目介绍 / 项目成绩) that
 * templates print as plain text, so `**加粗**` never leaks literal asterisks.
 */
export function renderInlineMarkdown(text: string): ReactNode[] {
  return tokenize(text).map((token, index) => {
    switch (token.type) {
      case 'strong':
        return <strong key={index}>{token.value}</strong>;
      case 'em':
        return <em key={index}>{token.value}</em>;
      case 'link':
        return <a key={index} href={token.href} rel="noreferrer noopener" target="_blank">{token.value}</a>;
      default:
        return token.value;
    }
  });
}