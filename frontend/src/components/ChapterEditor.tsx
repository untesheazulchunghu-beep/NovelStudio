import { createSignal, createEffect, Show, For } from 'solid-js';
import { chapterApi, Chapter } from '../services/api';

interface ChapterEditorProps {
  novelId: number;
  novelTitle: string;
  onClose: () => void;
}

export default function ChapterEditor(props: ChapterEditorProps) {
  const [chapters, setChapters] = createSignal<Chapter[]>([]);
  const [selectedChapter, setSelectedChapter] = createSignal<Chapter | null>(null);
  const [content, setContent] = createSignal('');
  const [title, setTitle] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [saving, setSaving] = createSignal(false);

  createEffect(() => {
    loadChapters();
  });

  const loadChapters = async () => {
    setLoading(true);
    try {
      const data = await chapterApi.getByNovel(props.novelId);
      setChapters(data.chapters);
      if (data.chapters.length > 0) {
        selectChapter(data.chapters[0]);
      }
    } catch (err) {
      console.error('Failed to load chapters:', err);
    } finally {
      setLoading(false);
    }
  };

  const selectChapter = (chapter: Chapter) => {
    setSelectedChapter(chapter);
    setTitle(chapter.title);
    setContent(chapter.content);
  };

  const handleSave = async () => {
    const current = selectedChapter();
    if (!current) return;

    setSaving(true);
    try {
      const updated = await chapterApi.update(current.id, {
        title: title(),
        content: content(),
      });

      setChapters(chapters().map(c => c.id === updated.id ? updated : c));
      setSelectedChapter(updated);

      // Show success feedback
      const btn = document.getElementById('save-btn');
      if (btn) {
        const originalText = btn.textContent;
        btn.textContent = '✓ Saved';
        setTimeout(() => {
          btn.textContent = originalText;
        }, 2000);
      }
    } catch (err) {
      alert('Failed to save chapter');
    } finally {
      setSaving(false);
    }
  };

  const handleCreateNew = async () => {
    const newTitle = prompt('Chapter title:');
    if (!newTitle) return;

    setLoading(true);
    try {
      const newChapter = await chapterApi.create(props.novelId, {
        title: newTitle,
        content: '',
        chapter_number: chapters().length + 1,
      });
      setChapters([...chapters(), newChapter]);
      selectChapter(newChapter);
    } catch (err) {
      alert('Failed to create chapter');
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    const current = selectedChapter();
    if (!current) return;

    if (!confirm(`Delete "${current.title}"?`)) return;

    setLoading(true);
    try {
      await chapterApi.delete(current.id);
      const remaining = chapters().filter(c => c.id !== current.id);
      setChapters(remaining);
      setSelectedChapter(remaining[0] || null);
      if (remaining[0]) {
        setTitle(remaining[0].title);
        setContent(remaining[0].content);
      } else {
        setTitle('');
        setContent('');
      }
    } catch (err) {
      alert('Failed to delete chapter');
    } finally {
      setLoading(false);
    }
  };

  const wordCount = () => {
    return content().trim().split(/\s+/).filter(w => w.length > 0).length;
  };

  return (
    <div style={{
      position: 'fixed',
      inset: '0',
      background: 'rgba(0, 0, 0, 0.8)',
      display: 'flex',
      'align-items': 'center',
      'justify-content': 'center',
      'z-index': '1000',
    }}>
      <div style={{
        background: '#0F172A',
        width: '95vw',
        height: '95vh',
        'border-radius': '16px',
        border: '1px solid #334155',
        display: 'flex',
        'flex-direction': 'column',
        overflow: 'hidden',
      }}>
        {/* Header */}
        <div style={{
          background: '#1E293B',
          padding: '1.5rem 2rem',
          'border-bottom': '1px solid #334155',
          display: 'flex',
          'justify-content': 'space-between',
          'align-items': 'center',
        }}>
          <div>
            <h2 style={{
              'font-size': '1.5rem',
              'font-weight': '700',
              color: '#F8FAFC',
              'margin-bottom': '0.25rem',
            }}>
              {props.novelTitle}
            </h2>
            <p style={{ color: '#94A3B8', 'font-size': '0.875rem' }}>
              {chapters().length} chapters
            </p>
          </div>
          <button
            onClick={props.onClose}
            style={{
              padding: '0.5rem 1rem',
              background: 'transparent',
              border: '1px solid #475569',
              'border-radius': '8px',
              color: '#F8FAFC',
              cursor: 'pointer',
              'font-size': '0.875rem',
            }}
          >
            Close
          </button>
        </div>

        {/* Main Content */}
        <div style={{
          display: 'flex',
          flex: '1',
          overflow: 'hidden',
        }}>
          {/* Chapter List Sidebar */}
          <div style={{
            width: '280px',
            background: '#1E293B',
            'border-right': '1px solid #334155',
            display: 'flex',
            'flex-direction': 'column',
          }}>
            <div style={{ padding: '1rem' }}>
              <button
                onClick={handleCreateNew}
                disabled={loading()}
                style={{
                  width: '100%',
                  padding: '0.75rem',
                  background: '#38BDF8',
                  color: '#0F172A',
                  border: 'none',
                  'border-radius': '8px',
                  'font-weight': '600',
                  cursor: loading() ? 'not-allowed' : 'pointer',
                  opacity: loading() ? '0.6' : '1',
                }}
              >
                + New Chapter
              </button>
            </div>

            <div style={{
              flex: '1',
              'overflow-y': 'auto',
              padding: '0 1rem 1rem',
            }}>
              <Show when={loading()}>
                <p style={{ color: '#94A3B8', 'font-size': '0.875rem', 'text-align': 'center' }}>
                  Loading...
                </p>
              </Show>

              <For each={chapters()}>
                {(chapter) => (
                  <div
                    onClick={() => selectChapter(chapter)}
                    style={{
                      padding: '1rem',
                      background: selectedChapter()?.id === chapter.id ? '#334155' : 'transparent',
                      border: `1px solid ${selectedChapter()?.id === chapter.id ? '#38BDF8' : '#334155'}`,
                      'border-radius': '8px',
                      'margin-bottom': '0.5rem',
                      cursor: 'pointer',
                      transition: 'all 0.2s',
                    }}
                  >
                    <div style={{
                      'font-weight': '600',
                      color: '#F8FAFC',
                      'margin-bottom': '0.25rem',
                      'font-size': '0.875rem',
                    }}>
                      Ch.{chapter.chapter_number}
                    </div>
                    <div style={{
                      color: '#CBD5E1',
                      'font-size': '0.875rem',
                      'margin-bottom': '0.5rem',
                      overflow: 'hidden',
                      'text-overflow': 'ellipsis',
                      'white-space': 'nowrap',
                    }}>
                      {chapter.title}
                    </div>
                    <div style={{ color: '#94A3B8', 'font-size': '0.75rem' }}>
                      {chapter.word_count} words
                    </div>
                  </div>
                )}
              </For>
            </div>
          </div>

          {/* Editor Area */}
          <div style={{
            flex: '1',
            display: 'flex',
            'flex-direction': 'column',
          }}>
            <Show when={selectedChapter()} fallback={
              <div style={{
                display: 'flex',
                'align-items': 'center',
                'justify-content': 'center',
                height: '100%',
                color: '#94A3B8',
              }}>
                Select or create a chapter to start writing
              </div>
            }>
              {/* Editor Toolbar */}
              <div style={{
                padding: '1rem 2rem',
                'border-bottom': '1px solid #334155',
                display: 'flex',
                'justify-content': 'space-between',
                'align-items': 'center',
                background: '#1E293B',
              }}>
                <input
                  type="text"
                  value={title()}
                  onInput={(e) => setTitle(e.currentTarget.value)}
                  placeholder="Chapter title..."
                  style={{
                    flex: '1',
                    padding: '0.5rem 1rem',
                    background: '#0F172A',
                    border: '1px solid #334155',
                    'border-radius': '8px',
                    color: '#F8FAFC',
                    'font-size': '1.125rem',
                    'font-weight': '600',
                    'margin-right': '1rem',
                  }}
                />
                <div style={{ display: 'flex', gap: '0.5rem', 'align-items': 'center' }}>
                  <span style={{ color: '#94A3B8', 'font-size': '0.875rem', 'margin-right': '1rem' }}>
                    {wordCount()} words
                  </span>
                  <button
                    onClick={handleSave}
                    id="save-btn"
                    disabled={saving()}
                    style={{
                      padding: '0.5rem 1.5rem',
                      background: '#38BDF8',
                      color: '#0F172A',
                      border: 'none',
                      'border-radius': '8px',
                      'font-weight': '600',
                      cursor: saving() ? 'not-allowed' : 'pointer',
                      opacity: saving() ? '0.6' : '1',
                      'font-size': '0.875rem',
                    }}
                  >
                    {saving() ? 'Saving...' : 'Save'}
                  </button>
                  <button
                    onClick={handleDelete}
                    disabled={loading()}
                    style={{
                      padding: '0.5rem 1rem',
                      background: 'transparent',
                      border: '1px solid #F87171',
                      'border-radius': '8px',
                      color: '#F87171',
                      cursor: loading() ? 'not-allowed' : 'pointer',
                      'font-size': '0.875rem',
                    }}
                  >
                    Delete
                  </button>
                </div>
              </div>

              {/* Text Editor */}
              <textarea
                value={content()}
                onInput={(e) => setContent(e.currentTarget.value)}
                placeholder="Start writing your story..."
                style={{
                  flex: '1',
                  padding: '2rem',
                  background: '#0F172A',
                  border: 'none',
                  color: '#F8FAFC',
                  'font-size': '1rem',
                  'line-height': '1.8',
                  'font-family': 'Georgia, serif',
                  resize: 'none',
                  outline: 'none',
                }}
              />
            </Show>
          </div>
        </div>
      </div>
    </div>
  );
}
