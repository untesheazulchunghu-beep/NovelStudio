import { createSignal, createEffect, Show, For } from 'solid-js';
import { characterApi, Character } from '../services/api';

interface CharacterPanelProps {
  novelId: number;
  onClose: () => void;
}

export default function CharacterPanel(props: CharacterPanelProps) {
  const [characters, setCharacters] = createSignal<Character[]>([]);
  const [selectedCharacter, setSelectedCharacter] = createSignal<Character | null>(null);
  const [loading, setLoading] = createSignal(false);
  const [isCreating, setIsCreating] = createSignal(false);

  // Form fields
  const [name, setName] = createSignal('');
  const [role, setRole] = createSignal('Main Character');
  const [description, setDescription] = createSignal('');

  const roleOptions = [
    'Main Character',
    'Supporting Character',
    'Antagonist',
    'Side Character',
    'Mentor',
    'Love Interest',
  ];

  createEffect(() => {
    loadCharacters();
  });

  const loadCharacters = async () => {
    setLoading(true);
    try {
      const data = await characterApi.getByNovel(props.novelId);
      setCharacters(data.characters);
    } catch (err) {
      console.error('Failed to load characters:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = async () => {
    if (!name().trim()) {
      alert('Character name is required');
      return;
    }

    setLoading(true);
    try {
      const newChar = await characterApi.create(props.novelId, {
        name: name(),
        role: role(),
        description: description(),
      });
      setCharacters([...characters(), newChar]);
      setIsCreating(false);
      resetForm();
    } catch (err) {
      alert('Failed to create character');
    } finally {
      setLoading(false);
    }
  };

  const resetForm = () => {
    setName('');
    setRole('Main Character');
    setDescription('');
  };

  const handleSelectCharacter = (char: Character) => {
    setSelectedCharacter(char);
    setIsCreating(false);
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
        'max-width': '1000px',
        height: '80vh',
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
              👥 Character Management
            </h2>
            <p style={{ color: '#94A3B8', 'font-size': '0.875rem' }}>
              {characters().length} characters
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
          {/* Character List */}
          <div style={{
            width: '320px',
            background: '#1E293B',
            'border-right': '1px solid #334155',
            display: 'flex',
            'flex-direction': 'column',
          }}>
            <div style={{ padding: '1rem' }}>
              <button
                onClick={() => {
                  setIsCreating(true);
                  setSelectedCharacter(null);
                  resetForm();
                }}
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
                }}
              >
                + Add Character
              </button>
            </div>

            <div style={{
              flex: '1',
              'overflow-y': 'auto',
              padding: '0 1rem 1rem',
            }}>
              <Show when={loading() && characters().length === 0}>
                <p style={{ color: '#94A3B8', 'font-size': '0.875rem', 'text-align': 'center' }}>
                  Loading...
                </p>
              </Show>

              <For each={characters()}>
                {(character) => (
                  <div
                    onClick={() => handleSelectCharacter(character)}
                    style={{
                      padding: '1rem',
                      background: selectedCharacter()?.id === character.id ? '#334155' : 'transparent',
                      border: `1px solid ${selectedCharacter()?.id === character.id ? '#38BDF8' : '#334155'}`,
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
                    }}>
                      {character.name}
                    </div>
                    <div style={{
                      color: '#94A3B8',
                      'font-size': '0.75rem',
                      padding: '0.25rem 0.5rem',
                      background: '#0F172A',
                      'border-radius': '4px',
                      display: 'inline-block',
                    }}>
                      {character.role}
                    </div>
                  </div>
                )}
              </For>
            </div>
          </div>

          {/* Detail Panel */}
          <div style={{
            flex: '1',
            padding: '2rem',
            'overflow-y': 'auto',
          }}>
            <Show when={isCreating()}>
              <div>
                <h3 style={{
                  color: '#F8FAFC',
                  'font-size': '1.25rem',
                  'font-weight': '600',
                  'margin-bottom': '1.5rem',
                }}>
                  Create New Character
                </h3>

                <div style={{ 'margin-bottom': '1.5rem' }}>
                  <label style={{
                    color: '#CBD5E1',
                    'font-size': '0.875rem',
                    'font-weight': '600',
                    'margin-bottom': '0.5rem',
                    display: 'block',
                  }}>
                    Character Name *
                  </label>
                  <input
                    type="text"
                    value={name()}
                    onInput={(e) => setName(e.currentTarget.value)}
                    placeholder="Enter character name..."
                    style={{
                      width: '100%',
                      padding: '0.75rem',
                      background: '#1E293B',
                      border: '1px solid #334155',
                      'border-radius': '8px',
                      color: '#F8FAFC',
                      'font-size': '1rem',
                    }}
                  />
                </div>

                <div style={{ 'margin-bottom': '1.5rem' }}>
                  <label style={{
                    color: '#CBD5E1',
                    'font-size': '0.875rem',
                    'font-weight': '600',
                    'margin-bottom': '0.5rem',
                    display: 'block',
                  }}>
                    Role
                  </label>
                  <select
                    value={role()}
                    onChange={(e) => setRole(e.currentTarget.value)}
                    style={{
                      width: '100%',
                      padding: '0.75rem',
                      background: '#1E293B',
                      border: '1px solid #334155',
                      'border-radius': '8px',
                      color: '#F8FAFC',
                      'font-size': '1rem',
                    }}
                  >
                    <For each={roleOptions}>
                      {(option) => <option value={option}>{option}</option>}
                    </For>
                  </select>
                </div>

                <div style={{ 'margin-bottom': '1.5rem' }}>
                  <label style={{
                    color: '#CBD5E1',
                    'font-size': '0.875rem',
                    'font-weight': '600',
                    'margin-bottom': '0.5rem',
                    display: 'block',
                  }}>
                    Description
                  </label>
                  <textarea
                    value={description()}
                    onInput={(e) => setDescription(e.currentTarget.value)}
                    placeholder="Describe the character's appearance, personality, background..."
                    style={{
                      width: '100%',
                      padding: '0.75rem',
                      background: '#1E293B',
                      border: '1px solid #334155',
                      'border-radius': '8px',
                      color: '#F8FAFC',
                      'font-size': '1rem',
                      'min-height': '200px',
                      resize: 'vertical',
                    }}
                  />
                </div>

                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <button
                    onClick={handleCreate}
                    disabled={loading()}
                    style={{
                      padding: '0.75rem 1.5rem',
                      background: '#38BDF8',
                      color: '#0F172A',
                      border: 'none',
                      'border-radius': '8px',
                      'font-weight': '600',
                      cursor: loading() ? 'not-allowed' : 'pointer',
                    }}
                  >
                    {loading() ? 'Creating...' : 'Create Character'}
                  </button>
                  <button
                    onClick={() => {
                      setIsCreating(false);
                      resetForm();
                    }}
                    style={{
                      padding: '0.75rem 1.5rem',
                      background: 'transparent',
                      border: '1px solid #475569',
                      'border-radius': '8px',
                      color: '#CBD5E1',
                      cursor: 'pointer',
                    }}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            </Show>

            <Show when={selectedCharacter()}>
              {(char) => (
                <div>
                  <h3 style={{
                    color: '#F8FAFC',
                    'font-size': '1.5rem',
                    'font-weight': '700',
                    'margin-bottom': '0.5rem',
                  }}>
                    {char().name}
                  </h3>
                  <div style={{
                    color: '#38BDF8',
                    'font-size': '0.875rem',
                    padding: '0.5rem 1rem',
                    background: 'rgba(56, 189, 248, 0.1)',
                    'border-radius': '8px',
                    display: 'inline-block',
                    'margin-bottom': '1.5rem',
                  }}>
                    {char().role}
                  </div>

                  <div style={{
                    background: '#1E293B',
                    padding: '1.5rem',
                    'border-radius': '12px',
                    border: '1px solid #334155',
                  }}>
                    <h4 style={{
                      color: '#CBD5E1',
                      'font-size': '0.875rem',
                      'font-weight': '600',
                      'margin-bottom': '1rem',
                      'text-transform': 'uppercase',
                      'letter-spacing': '0.05em',
                    }}>
                      Description
                    </h4>
                    <p style={{
                      color: '#F8FAFC',
                      'line-height': '1.8',
                      'white-space': 'pre-wrap',
                    }}>
                      {char().description || 'No description provided.'}
                    </p>
                  </div>
                </div>
              )}
            </Show>

            <Show when={!isCreating() && !selectedCharacter()}>
              <div style={{
                display: 'flex',
                'align-items': 'center',
                'justify-content': 'center',
                height: '100%',
                color: '#94A3B8',
                'text-align': 'center',
              }}>
                <div>
                  <div style={{ 'font-size': '3rem', 'margin-bottom': '1rem' }}>👥</div>
                  <div>Select a character or create a new one</div>
                </div>
              </div>
            </Show>
          </div>
        </div>
      </div>
    </div>
  );
}
