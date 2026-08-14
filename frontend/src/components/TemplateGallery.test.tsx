import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import App from '../app';
import { templateRegistry } from '../templates/registry';

vi.mock('../api/auth', () => ({
  getCurrentUser: vi.fn().mockResolvedValue({ id: 1, email: 'ada@example.com', tenantId: 1 }),
  login: vi.fn(), register: vi.fn(), logout: vi.fn(),
}));

afterEach(cleanup);

describe('template gallery workflow', () => {
  it('shows static cards first, enters the selected editor without resetting content, and returns to the gallery', async () => {
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });

    expect(screen.queryByTestId('resume-preview')).not.toBeInTheDocument();
    for (const template of templateRegistry) {
      expect(screen.getByRole('img', { name: `${template.name} 模板预览` })).toHaveAttribute('src', template.previewImage);
      expect(screen.getByRole('button', { name: `使用此模板：${template.name}` })).toBeInTheDocument();
    }

    fireEvent.click(screen.getByRole('button', { name: '使用此模板：现代商务' }));

    expect(screen.getByTestId('resume-preview')).toHaveTextContent('林知夏');
    expect(screen.getByText('当前模板：现代商务')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '返回模板主页' }));

    expect(screen.queryByTestId('resume-preview')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '选择简历模板' })).toBeInTheDocument();
  });

  it('gives every static cover a descriptive alternative text', async () => {
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });

    for (const template of templateRegistry) {
      expect(screen.getByRole('img', { name: `${template.name} 模板预览` })).toHaveAttribute(
        'alt',
        `${template.name} 模板预览`,
      );
    }
  });
});
