import { createSignal } from 'solid-js';
import { Novel, novelApi } from '../services/api';

const [novels, setNovels] = createSignal<Novel[]>([]);
const [currentNovel, setCurrentNovel] = createSignal<Novel | null>(null);
const [loading, setLoading] = createSignal(false);
const [error, setError] = createSignal<string | null>(null);

export const novelStore = {
  novels,
  currentNovel,
  loading,
  error,

  async loadNovels() {
    setLoading(true);
    setError(null);
    try {
      const data = await novelApi.getAll();
      setNovels(data.novels);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load novels');
    } finally {
      setLoading(false);
    }
  },

  async loadNovel(id: number) {
    setLoading(true);
    setError(null);
    try {
      const novel = await novelApi.getOne(id);
      setCurrentNovel(novel);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load novel');
    } finally {
      setLoading(false);
    }
  },

  async createNovel(title: string, author: string, description?: string) {
    setLoading(true);
    setError(null);
    try {
      const novel = await novelApi.create({ title, author, description });
      setNovels([...novels(), novel]);
      return novel;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create novel');
      throw err;
    } finally {
      setLoading(false);
    }
  },

  async deleteNovel(id: number) {
    setLoading(true);
    setError(null);
    try {
      await novelApi.delete(id);
      setNovels(novels().filter(n => n.id !== id));
      if (currentNovel()?.id === id) {
        setCurrentNovel(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete novel');
      throw err;
    } finally {
      setLoading(false);
    }
  },
};
