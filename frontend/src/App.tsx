import { Component, createSignal, Show } from 'solid-js';
import NovelList from './components/NovelList';

const App: Component = () => {
  const [showSidebar, setShowSidebar] = createSignal(true);

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      {/* Sidebar */}
      <Show when={showSidebar()}>
        <div style={{
          width: '240px',
          background: '#1E293B',
          'border-right': '1px solid #334155',
          display: 'flex',
          'flex-direction': 'column',
        }}>
          <div style={{
            padding: '1.5rem',
            'border-bottom': '1px solid #334155',
          }}>
            <h1 style={{
              'font-size': '1.5rem',
              'font-weight': '700',
              color: '#38BDF8',
              'margin-bottom': '0.25rem',
            }}>
              NovelStudio
            </h1>
            <p style={{
              color: '#94A3B8',
              'font-size': '0.75rem',
            }}>
              AI-Powered Writing
            </p>
          </div>

          <nav style={{
            flex: '1',
            padding: '1rem',
            'overflow-y': 'auto',
          }}>
            <div style={{
              padding: '0.75rem 1rem',
              background: 'rgba(56, 189, 248, 0.1)',
              border: '1px solid #38BDF8',
              'border-radius': '8px',
              color: '#38BDF8',
              'font-weight': '600',
              'margin-bottom': '0.5rem',
              cursor: 'pointer',
            }}>
              📚 My Novels
            </div>

            <div style={{
              padding: '0.75rem 1rem',
              color: '#94A3B8',
              'border-radius': '8px',
              cursor: 'pointer',
              transition: 'all 0.2s',
            }}
            onMouseOver={(e) => {
              e.currentTarget.style.background = 'rgba(148, 163, 184, 0.1)';
              e.currentTarget.style.color = '#CBD5E1';
            }}
            onMouseOut={(e) => {
              e.currentTarget.style.background = 'transparent';
              e.currentTarget.style.color = '#94A3B8';
            }}
            >
              📊 Statistics
            </div>

            <div style={{
              padding: '0.75rem 1rem',
              color: '#94A3B8',
              'border-radius': '8px',
              cursor: 'pointer',
              transition: 'all 0.2s',
            }}
            onMouseOver={(e) => {
              e.currentTarget.style.background = 'rgba(148, 163, 184, 0.1)';
              e.currentTarget.style.color = '#CBD5E1';
            }}
            onMouseOut={(e) => {
              e.currentTarget.style.background = 'transparent';
              e.currentTarget.style.color = '#94A3B8';
            }}
            >
              ⚙️ Settings
            </div>
          </nav>

          <div style={{
            padding: '1rem',
            'border-top': '1px solid #334155',
          }}>
            <div style={{
              padding: '0.75rem',
              background: '#0F172A',
              'border-radius': '8px',
              'font-size': '0.75rem',
            }}>
              <div style={{
                color: '#94A3B8',
                'margin-bottom': '0.25rem',
              }}>
                Version
              </div>
              <div style={{
                color: '#38BDF8',
                'font-weight': '600',
              }}>
                v1.0.0-beta
              </div>
            </div>
          </div>
        </div>
      </Show>

      {/* Main Content */}
      <div style={{
        flex: '1',
        display: 'flex',
        'flex-direction': 'column',
        overflow: 'hidden',
      }}>
        {/* Top Bar */}
        <div style={{
          background: '#1E293B',
          'border-bottom': '1px solid #334155',
          padding: '0.75rem 1.5rem',
          display: 'flex',
          'align-items': 'center',
          'justify-content': 'space-between',
        }}>
          <button
            onClick={() => setShowSidebar(!showSidebar())}
            style={{
              padding: '0.5rem 0.75rem',
              background: 'transparent',
              border: '1px solid #334155',
              'border-radius': '6px',
              color: '#CBD5E1',
              cursor: 'pointer',
              'font-size': '1.25rem',
            }}
          >
            ☰
          </button>

          <div style={{
            display: 'flex',
            'align-items': 'center',
            gap: '1rem',
          }}>
            <div style={{
              display: 'flex',
              'align-items': 'center',
              gap: '0.5rem',
              padding: '0.5rem 1rem',
              background: '#0F172A',
              'border-radius': '8px',
            }}>
              <div style={{
                width: '8px',
                height: '8px',
                'border-radius': '50%',
                background: '#22C55E',
              }} />
              <span style={{
                color: '#94A3B8',
                'font-size': '0.875rem',
              }}>
                Connected
              </span>
            </div>
          </div>
        </div>

        {/* Page Content */}
        <main style={{
          flex: '1',
          'overflow-y': 'auto',
          background: '#0F172A',
        }}>
          <NovelList />
        </main>
      </div>
    </div>
  );
};

export default App;
