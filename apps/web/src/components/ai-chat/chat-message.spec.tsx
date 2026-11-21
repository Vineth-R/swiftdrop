import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';

import { ChatMessage } from './chat-message';

function render(text: string, role: 'user' | 'assistant' = 'assistant') {
  return renderToStaticMarkup(<ChatMessage role={role} text={text} />);
}

describe('ChatMessage Markdown rendering', () => {
  it('renders bold, italic, lists and inline code', () => {
    const html = render(
      [
        'The delivery fee is **700 LKR**.',
        '',
        '- **Base fee:** 450 LKR',
        '- **Weight charge:** 100 LKR',
        '',
        '1. First step',
        '2. Second step',
        '',
        'Use `SDK7M2Q9XP4` for tracking.',
      ].join('\n'),
    );

    expect(html).toMatch(/<strong[^>]*>700 LKR<\/strong>/);
    expect(html).toMatch(/<strong[^>]*>Base fee:<\/strong>/);
    expect(html).toMatch(/<ul[^>]*>/);
    expect(html).toMatch(/<ol[^>]*>/);
    expect(html).toMatch(/<li[^>]*>/);
    expect(html).toMatch(/<code[^>]*>SDK7M2Q9XP4<\/code>/);
    expect(html).not.toContain('**');
  });

  it('renders italic and line breaks', () => {
    const html = render('This is *important*.\nA new line.');
    expect(html).toMatch(/<em[^>]*>important<\/em>/);
    expect(html).toContain('A new line.');
  });

  it('keeps plain text readable', () => {
    const html = render('SwiftDrop delivers parcels.');
    expect(html).toContain('SwiftDrop delivers parcels.');
    expect(html).toMatch(/<p[^>]*>/);
  });

  it('rejects protocol-relative and javascript links', () => {
    const html = render(
      '[protocol relative](//evil.example/page) [bad](javascript:alert(1))',
    );
    expect(html).not.toContain('href="//evil.example/page"');
    expect(html).not.toContain('href="javascript:');
    expect(html).toContain('href=""');
  });

  it('does not render raw HTML or unsafe links', () => {
    const html = render(
      '<img src=x onerror="window.__xss=1"> [bad](javascript:alert(1))',
    );
    expect(html).not.toContain('<img');
    expect(html).not.toMatch(/<[^>]*onerror/i);
    expect(html).not.toContain('javascript:');
  });

  it('renders safe external links with safe rel attributes', () => {
    const html = render('[SwiftDrop](https://example.com/page)');
    expect(html).toContain('href="https://example.com/page"');
    expect(html).toContain('target="_blank"');
    expect(html).toContain('rel="noopener noreferrer nofollow"');
  });

  it('handles incomplete Markdown without throwing', () => {
    expect(() => render('An **unfinished bold and `code')).not.toThrow();
  });

  it('keeps user messages as plain text', () => {
    const html = render('**not bold**', 'user');
    expect(html).not.toMatch(/<strong/);
    expect(html).toContain('**not bold**');
  });
});
