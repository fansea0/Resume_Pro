import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

interface ResumeMarkdownProps {
  markdown: string;
}

function preserveLineBreaks(markdown: string): string {
  return markdown.replace(/([^\n])\n(?!\n|[-*+]\s|\d+\.\s)/g, '$1  \n');
}

/** Renders the limited Markdown supported by resume content without raw HTML. */
export function ResumeMarkdown({ markdown }: ResumeMarkdownProps) {
  return (
    <div className="resume-markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          p: ({ children }) => <p className="resume-markdown-paragraph">{children}</p>,
          ul: ({ children }) => <ul className="resume-markdown-unordered-list">{children}</ul>,
          ol: ({ children }) => <ol className="resume-markdown-ordered-list">{children}</ol>,
          li: ({ children }) => <li className="resume-markdown-list-item">{children}</li>,
        }}
      >
        {preserveLineBreaks(markdown)}
      </ReactMarkdown>
    </div>
  );
}
