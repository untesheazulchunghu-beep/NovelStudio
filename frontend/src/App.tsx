import { Component } from 'solid-js';
import NovelList from './components/NovelList';

const App: Component = () => {
  return (
    <div>
      <nav style={{
        background: '#1E293B',
        'border-bottom': '1px solid #334155',
        padding: '1rem 2rem',
      }}>
        <h1 style={{
          'font-size': '1.5rem',
          'font-weight': '700',
          color: '#38BDF8',
        }}>
          NovelStudio
        </h1>
      </nav>
      <main>
        <NovelList />
      </main>
    </div>
  );
};

export default App;
