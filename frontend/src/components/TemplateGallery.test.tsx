import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import App from '../app';
import { templateRegistry } from '../templates/registry';

afterEach(cleanup);

describe('template gallery workflow', () => {
  it('shows static cards first, enters the selected editor without resetting content, and returns to the gallery', () => {
    render(<App />);

    expect(screen.queryByTestId('resume-preview')).not.toBeInTheDocument();
    for (const template of templateRegistry) {
      expect(screen.getByRole('img', { name: `${template.name} 模板预览` })).toHaveAttribute('src', template.previewImage);
      expect(screen.getByRole('button', { name: `使用此模板：${template.name}` })).toBeInTheDocument();
    }

    fireEvent.click(screen.getByRole('button', { name: '使用此模板：现代商务' }));

    expect(screen.getByTestId('resume-preview')).toHaveTextContent('林知夏');
    expect(screen.getByTestId('active-template')).toHaveTextContent('business');

    fireEvent.click(screen.getByRole('button', { name: '返回模板库' }));

    expect(screen.queryByTestId('resume-preview')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '选择简历模板' })).toBeInTheDocument();
  });
});
