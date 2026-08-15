import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

interface ResumeMarkdownProps {
  markdown: string;
}

function preserveLineBreaks(markdown: string): string {
  return markdown.replace(/([^\n])\n(?!\n|[-*+]\s|\d+\.\s)/g, '$1  \n');
}

const allowedElements = ['p', 'br', 'strong', 'em', 'a', 'ul', 'ol', 'li'];

function safeUrl(url: string): string {
  return /^(https?:|mailto:|tel:)/i.test(url) ? url : '';
}

/** Renders the limited Markdown supported by resume content without raw HTML. */
export function ResumeMarkdown({ markdown }: ResumeMarkdownProps) {
  return (
    <ReactMarkdown
      allowedElements={allowedElements}
      remarkPlugins={[remarkGfm]}
      urlTransform={safeUrl}
      components={{
        p: ({ children }) => <p className="resume-markdown-paragraph">{children}</p>,
        ul: ({ children }) => <ul className="resume-markdown-unordered-list">{children}</ul>,
        ol: ({ children }) => <ol className="resume-markdown-ordered-list">{children}</ol>,
        li: ({ children }) => <li className="resume-markdown-list-item">{children}</li>,
      }}
    >
      {preserveLineBreaks(markdown)}
    </ReactMarkdown>
  );
}
