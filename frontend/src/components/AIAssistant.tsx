import { createSignal, createEffect, Show, For } from 'solid-js';
import { aiApi } from '../services/api';

interface AIAssistantProps {
  onClose: () => void;
}

export default function AIAssistant(props: AIAssistantProps) {
  const [prompt, setPrompt] = createSignal('');
  const [systemPrompt, setSystemPrompt] = createSignal('You are a creative fiction writer assistant.');
  const [response, setResponse] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [history, setHistory] = createSignal<Array<{role: string; content: string}>>([]);

  const presets = [
    {
      name: 'Story Opening',
      system: 'You are a creative fiction writer specializing in compelling story openings.',
      prompt: 'Write an engaging opening paragraph for a science fiction novel about:',
    },
    {
      name: 'Character Description',
      system: 'You are a character development expert.',
      prompt: 'Create a detailed character description for:',
    },
    {
      name: 'Scene Setting',
      system: 'You are an expert at vivid scene descriptions.',
      prompt: 'Describe the following scene in vivid detail:',
    },
    {
      name: 'Dialogue',
      system: 'You are a dialogue specialist for fiction writing.',
      prompt: 'Write a natural dialogue between two characters about:',
    },
    {
      name: 'Plot Twist',
      system: 'You are a master of plot twists and surprises.',
      prompt: 'Suggest a surprising plot twist for a story about:',
    },
  ];

  const handleGenerate = async () => {
    if (!prompt().trim()) return;

    setLoading(true);
    setResponse('');

    try {
      const result = await aiApi.complete(prompt(), systemPrompt());
      setResponse(result.text);

      // Add to history
      setHistory([
        ...history(),
        { role: 'user', content: prompt() },
        { role: 'assistant', content: result.text },
      ]);
    } catch (err) {
      setResponse('Error: Failed to generate response. Please check your API configuration.');
    } finally {
      setLoading(false);
    }
  };

  const handleContinue = async () => {
    if (!response()) return;

    setLoading(true);

    try {
      const result = await aiApi.continue(response());
      const continued = response() + '\n\n' + result.continuation;
      setResponse(continued);

      // Update history
      const newHistory = [...history()];
      newHistory[newHistory.length - 1] = {
        role: 'assistant',
        content: continued,
      };
      setHistory(newHistory);
    } catch (err) {
      alert('Failed to continue writing');
    } finally {
      setLoading(false);
    }
  };

  const handlePreset = (preset: typeof presets[0]) => {
    setSystemPrompt(preset.system);
    setPrompt(preset.prompt);
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(response());

    // Show feedback
    const btn = document.getElementById('copy-btn');
    if (btn) {
      const originalText = btn.textContent;
      btn.textContent = '✓ Copied';
      setTimeout(() => {
        btn.textContent = originalText;
      }, 2000);
    }
  };

  const handleClear = () => {
    setPrompt('');
    setResponse('');
    setHistory([]);
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
        width: '90vw',
        'max-width': '1200px',
        height: '90vh',
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
              🤖 AI Writing Assistant
            </h2>
            <p style={{ color: '#94A3B8', 'font-size': '0.875rem' }}>
              Generate creative content with AI
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
          {/* Left Panel - Input */}
          <div style={{
            width: '45%',
            'border-right': '1px solid #334155',
            display: 'flex',
            'flex-direction': 'column',
            padding: '1.5rem',
            'overflow-y': 'auto',
          }}>
            <h3 style={{
              color: '#F8FAFC',
              'font-size': '1rem',
              'font-weight': '600',
              'margin-bottom': '1rem',
            }}>
              Quick Presets
            </h3>

            <div style={{
              display: 'flex',
              'flex-wrap': 'wrap',
              gap: '0.5rem',
              'margin-bottom': '1.5rem',
            }}>
              <For each={presets}>
                {(preset) => (
                  <button
                    onClick={() => handlePreset(preset)}
                    style={{
                      padding: '0.5rem 1rem',
                      background: '#1E293B',
                      border: '1px solid #334155',
                      'border-radius': '8px',
                      color: '#CBD5E1',
                      'font-size': '0.875rem',
                      cursor: 'pointer',
                      transition: 'all 0.2s',
                    }}
                    onMouseOver={(e) => {
                      e.currentTarget.style.borderColor = '#38BDF8';
                      e.currentTarget.style.color = '#38BDF8';
                    }}
                    onMouseOut={(e) => {
                      e.currentTarget.style.borderColor = '#334155';
                      e.currentTarget.style.color = '#CBD5E1';
                    }}
                  >
                    {preset.name}
                  </button>
                )}
              </For>
            </div>

            <label style={{
              color: '#CBD5E1',
              'font-size': '0.875rem',
              'font-weight': '600',
              'margin-bottom': '0.5rem',
              display: 'block',
            }}>
              System Prompt
            </label>
            <textarea
              value={systemPrompt()}
              onInput={(e) => setSystemPrompt(e.currentTarget.value)}
              placeholder="Set the AI's role and behavior..."
              style={{
                padding: '0.75rem',
                background: '#1E293B',
                border: '1px solid #334155',
                'border-radius': '8px',
                color: '#F8FAFC',
                'font-size': '0.875rem',
                'margin-bottom': '1rem',
                'min-height': '80px',
                resize: 'vertical',
              }}
            />

            <label style={{
              color: '#CBD5E1',
              'font-size': '0.875rem',
              'font-weight': '600',
              'margin-bottom': '0.5rem',
              display: 'block',
            }}>
              Your Prompt
            </label>
            <textarea
              value={prompt()}
              onInput={(e) => setPrompt(e.currentTarget.value)}
              placeholder="Describe what you want the AI to write..."
              style={{
                padding: '0.75rem',
                background: '#1E293B',
                border: '1px solid #334155',
                'border-radius': '8px',
                color: '#F8FAFC',
                'font-size': '0.875rem',
                flex: '1',
                resize: 'none',
                'margin-bottom': '1rem',
              }}
            />

            <button
              onClick={handleGenerate}
              disabled={loading() || !prompt().trim()}
              style={{
                padding: '0.75rem',
                background: loading() || !prompt().trim() ? '#334155' : '#38BDF8',
                color: loading() || !prompt().trim() ? '#94A3B8' : '#0F172A',
                border: 'none',
                'border-radius': '8px',
                'font-weight': '600',
                cursor: loading() || !prompt().trim() ? 'not-allowed' : 'pointer',
              }}
            >
              {loading() ? '⏳ Generating...' : '✨ Generate'}
            </button>
          </div>

          {/* Right Panel - Output */}
          <div style={{
            flex: '1',
            display: 'flex',
            'flex-direction': 'column',
            padding: '1.5rem',
          }}>
            <div style={{
              display: 'flex',
              'justify-content': 'space-between',
              'align-items': 'center',
              'margin-bottom': '1rem',
            }}>
              <h3 style={{
                color: '#F8FAFC',
                'font-size': '1rem',
                'font-weight': '600',
              }}>
                Generated Content
              </h3>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <button
                  onClick={handleContinue}
                  disabled={!response() || loading()}
                  style={{
                    padding: '0.5rem 1rem',
                    background: 'transparent',
                    border: '1px solid #F97316',
                    'border-radius': '8px',
                    color: '#F97316',
                    'font-size': '0.875rem',
                    cursor: !response() || loading() ? 'not-allowed' : 'pointer',
                    opacity: !response() || loading() ? '0.5' : '1',
                  }}
                >
                  Continue Writing
                </button>
                <button
                  onClick={handleCopy}
                  id="copy-btn"
                  disabled={!response()}
                  style={{
                    padding: '0.5rem 1rem',
                    background: 'transparent',
                    border: '1px solid #38BDF8',
                    'border-radius': '8px',
                    color: '#38BDF8',
                    'font-size': '0.875rem',
                    cursor: !response() ? 'not-allowed' : 'pointer',
                    opacity: !response() ? '0.5' : '1',
                  }}
                >
                  Copy
                </button>
                <button
                  onClick={handleClear}
                  disabled={!response()}
                  style={{
                    padding: '0.5rem 1rem',
                    background: 'transparent',
                    border: '1px solid #475569',
                    'border-radius': '8px',
                    color: '#94A3B8',
                    'font-size': '0.875rem',
                    cursor: !response() ? 'not-allowed' : 'pointer',
                    opacity: !response() ? '0.5' : '1',
                  }}
                >
                  Clear
                </button>
              </div>
            </div>

            <div style={{
              flex: '1',
              padding: '1.5rem',
              background: '#1E293B',
              border: '1px solid #334155',
              'border-radius': '12px',
              'overflow-y': 'auto',
            }}>
              <Show when={loading()}>
                <div style={{
                  display: 'flex',
                  'align-items': 'center',
                  'justify-content': 'center',
                  height: '100%',
                  color: '#94A3B8',
                }}>
                  <div style={{ 'text-align': 'center' }}>
                    <div style={{ 'font-size': '2rem', 'margin-bottom': '1rem' }}>⏳</div>
                    <div>AI is thinking...</div>
                  </div>
                </div>
              </Show>

              <Show when={!loading() && !response()}>
                <div style={{
                  display: 'flex',
                  'align-items': 'center',
                  'justify-content': 'center',
                  height: '100%',
                  color: '#94A3B8',
                  'text-align': 'center',
                }}>
                  <div>
                    <div style={{ 'font-size': '3rem', 'margin-bottom': '1rem' }}>✨</div>
                    <div>Enter a prompt and click Generate</div>
                  </div>
                </div>
              </Show>

              <Show when={response()}>
                <pre style={{
                  color: '#F8FAFC',
                  'line-height': '1.8',
                  'white-space': 'pre-wrap',
                  'font-family': 'Georgia, serif',
                  'font-size': '1rem',
                  margin: '0',
                }}>
                  {response()}
                </pre>
              </Show>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
