import { createEffect, For, Show } from 'solid-js';
import { novelStore } from '../stores/novelStore';

export default function NovelList() {
  createEffect(() => {
    novelStore.loadNovels();
  });

  const handleCreate = async () => {
    const title = prompt('Novel title:');
    if (!title) return;

    const author = prompt('Author name:') || 'Unknown';

    try {
      await novelStore.createNovel(title, author);
    } catch (err) {
      alert('Failed to create novel');
    }
  };

  const handleDelete = async (id: number, title: string) => {
    if (!confirm(`Delete "${title}"?`)) return;

    try {
      await novelStore.deleteNovel(id);
    } catch (err) {
      alert('Failed to delete novel');
    }
  };

  return (
    <div style={{
      padding: '2rem',
      'max-width': '1200px',
      margin: '0 auto',
    }}>
      <div style={{
        display: 'flex',
        'justify-content': 'space-between',
        'align-items': 'center',
        'margin-bottom': '2rem',
      }}>
        <h1 style={{
          'font-size': '2rem',
          'font-weight': '700',
          color: '#F8FAFC',
        }}>
          My Novels
        </h1>
        <button
          onClick={handleCreate}
          style={{
            padding: '0.75rem 1.5rem',
            background: '#38BDF8',
            color: '#0F172A',
            border: 'none',
            'border-radius': '8px',
            'font-weight': '600',
            cursor: 'pointer',
            transition: 'all 0.2s',
          }}
          onMouseOver={(e) => e.currentTarget.style.background = '#0EA5E9'}
          onMouseOut={(e) => e.currentTarget.style.background = '#38BDF8'}
        >
          + New Novel
        </button>
      </div>

      <Show when={novelStore.loading()}>
        <p style={{ color: '#94A3B8' }}>Loading...</p>
      </Show>

      <Show when={novelStore.error()}>
        <p style={{ color: '#F87171' }}>{novelStore.error()}</p>
      </Show>

      <div style={{
        display: 'grid',
        'grid-template-columns': 'repeat(auto-fill, minmax(300px, 1fr))',
        gap: '1.5rem',
      }}>
        <For each={novelStore.novels()}>
          {(novel) => (
            <div style={{
              background: '#1E293B',
              border: '1px solid #334155',
              'border-radius': '12px',
              padding: '1.5rem',
              transition: 'all 0.2s',
              cursor: 'pointer',
            }}
            onMouseOver={(e) => {
              e.currentTarget.style.borderColor = '#38BDF8';
              e.currentTarget.style.transform = 'translateY(-2px)';
            }}
            onMouseOut={(e) => {
              e.currentTarget.style.borderColor = '#334155';
              e.currentTarget.style.transform = 'translateY(0)';
            }}
            >
              <h3 style={{
                'font-size': '1.25rem',
                'font-weight': '600',
                color: '#F8FAFC',
                'margin-bottom': '0.5rem',
              }}>
                {novel.title}
              </h3>
              <p style={{
                color: '#94A3B8',
                'font-size': '0.875rem',
                'margin-bottom': '0.5rem',
              }}>
                by {novel.author}
              </p>
              <Show when={novel.description}>
                <p style={{
                  color: '#CBD5E1',
                  'font-size': '0.875rem',
                  'line-height': '1.5',
                  'margin-top': '1rem',
                }}>
                  {novel.description}
                </p>
              </Show>
              <div style={{
                display: 'flex',
                gap: '0.5rem',
                'margin-top': '1rem',
              }}>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    handleDelete(novel.id, novel.title);
                  }}
                  style={{
                    padding: '0.5rem 1rem',
                    background: 'transparent',
                    border: '1px solid #475569',
                    'border-radius': '6px',
                    color: '#F87171',
                    'font-size': '0.875rem',
                    cursor: 'pointer',
                  }}
                >
                  Delete
                </button>
              </div>
            </div>
          )}
        </For>
      </div>
    </div>
  );
}
